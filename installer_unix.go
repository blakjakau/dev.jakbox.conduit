//go:build linux
// +build linux

package main

import (
        "bytes"
        "runtime"
        "fmt"
        "io"
        "os"
        "os/exec"
        "path/filepath"
        "os/user" // Added import for os/user
        "strings"
        "text/template"
)

// Install path constants for Unix-like systems
const (
        userExecDirLinux         = ".local/bin"
        userDesktopFileDirLinux  = ".local/share/applications"
        desktopFileName          = "conduit.desktop"
        targetExecName           = "conduit"
        // System-wide paths for Linux service
        systemExecPathLinux     = "/usr/local/bin/conduit"
        serviceName             = "conduit.service" // Defined serviceName
        systemServiceFileLinux  = "/etc/systemd/system/conduit.service"
)

// Unix-specific templates
const desktopFileTemplate = `[Desktop Entry]
Type=Application
Name=Conduit Server
Comment=A PTY and file API server for web applications
Exec={{.ExecPath}} %u
Terminal=false
Categories=Utility;Development;
MimeType=x-scheme-handler/conduit;
NoDisplay=true
`

const serviceFileTemplate = `[Unit]
Description=Conduit PTY and file API server
After=network.target

[Service]
Type=simple
User={{.User}}
ExecStart={{.ExecPath}} --root={{.UserHome}} --no-idle-shutdown
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
        sourceFile, err := os.Open(src)
        if err != nil {
                return err
        }
        defer sourceFile.Close()

        destFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
        if err != nil {
                return err
        }
        defer destFile.Close()

        _, err = io.Copy(destFile, sourceFile)
        return err
}

// runCommand executes a command and returns its output or an error.
func runCommand(name string, arg ...string) (string, error) {
        cmd := exec.Command(name, arg...)
        var out bytes.Buffer
        cmd.Stdout = &out
        cmd.Stderr = &out
        err := cmd.Run()
        if err != nil {
                return out.String(), fmt.Errorf("command '%s %s' failed: %w, output: %s", name, strings.Join(arg, " "), err, out.String())
        }
        return out.String(), nil
}

// isRoot checks if the current user is root.
func isRoot() bool {
        return os.Geteuid() == 0
}

// checkSystemctl checks if systemctl is available.
func checkSystemctl() bool {
        _, err := exec.LookPath("systemctl")
        return err == nil
}

// checkIfInstalled checks if the executable is running from a known installation path.
func checkIfInstalled() bool {
        exePath, err := os.Executable()
        if err != nil {
                return false // Cannot determine path, assume not installed.
        }

        homeDir, err := os.UserHomeDir()

        if err != nil {
                return false // Can't check user install path without home dir.
        }
        installedPaths := []string{filepath.Join(homeDir, userExecDirLinux, targetExecName), systemExecPathLinux}

        for _, path := range installedPaths {
                if exePath == path {
                        return true
                }
        }
        return false
}

// InstallUser provides the user-level installation for Linux
func InstallUser() (string, error) {
        if !isCompiledBuild {
                return "Installation must be run from a compiled binary.", fmt.Errorf("not a compiled build")
        }

        var messages strings.Builder
        exePath, _ := os.Executable()
        homeDir, err := os.UserHomeDir()
        if err != nil {
                return fmt.Sprintf("Error getting user home directory: %v", err), err
        }

        messages.WriteString("Starting Linux user installation...\n")
        targetExecDir := filepath.Join(homeDir, userExecDirLinux)
        targetExecPath := filepath.Join(targetExecDir, targetExecName)
        targetDesktopDir := filepath.Join(homeDir, userDesktopFileDirLinux)
        targetDesktopPath := filepath.Join(targetDesktopDir, desktopFileName)
        for _, dir := range []string{targetExecDir, targetDesktopDir} {
                if err := os.MkdirAll(dir, 0755); err != nil {
                        return fmt.Sprintf("Error creating directory %s: %v", dir, err), err
                }
        }
        messages.WriteString(fmt.Sprintf("- Ensured directories exist: %s, %s\n", targetExecDir, targetDesktopDir))
        if err := copyFile(exePath, targetExecPath); err != nil {
                return fmt.Sprintf("Error copying executable to %s: %v", targetExecPath, err), err
        }
        messages.WriteString(fmt.Sprintf("- Copied executable to %s\n", targetExecPath))
        desktopTmpl := template.Must(template.New("desktop").Parse(desktopFileTemplate))
        desktopFile, err := os.Create(targetDesktopPath)
        if err != nil {
                return fmt.Sprintf("Error creating desktop file at %s: %v", targetDesktopPath, err), err
        }
        defer desktopFile.Close()
        if err := desktopTmpl.Execute(desktopFile, map[string]string{"ExecPath": targetExecPath}); err != nil {
                return "Error writing desktop file content.", err
        }
        messages.WriteString(fmt.Sprintf("- Created desktop file at %s\n", targetDesktopPath))
        if _, err := runCommand("update-desktop-database", targetDesktopDir); err != nil {
                messages.WriteString(fmt.Sprintf("! Warning: 'update-desktop-database' failed: %v\n", err))
        } else {
                messages.WriteString("- Updated desktop application database.\n")
        }
        if _, err := runCommand("xdg-mime", "default", desktopFileName, "x-scheme-handler/conduit"); err != nil {
                messages.WriteString(fmt.Sprintf("! Warning: 'xdg-mime' failed: %v\n", err))
        } else {
                messages.WriteString(fmt.Sprintf("- Set %s as default for conduit:// protocol.\n", desktopFileName))
        }
        messages.WriteString("\nUser installation complete. You may need to restart your desktop session for all changes to take effect.\n")
        return messages.String(), nil
}

// InstallService (Linux only) installs Conduit as a systemd service.
func InstallService() (string, error) {
        if runtime.GOOS != "linux" { // Redundant with build tags, but good practice
                return "Service installation is only supported on Linux.", fmt.Errorf("unsupported OS")
        }
        if !isCompiledBuild {
                return "Installation must be run from a compiled binary.", fmt.Errorf("not a compiled build")
        }
        if !isRoot() {
                return "Service installation requires root privileges. Please run with sudo.", fmt.Errorf("requires root")
        }
        if !checkSystemctl() {
                return "Systemd (systemctl) not found. Cannot install service.", fmt.Errorf("systemd not found")
        }

        var messages strings.Builder
        exePath, _ := os.Executable()
        // Get the user who executed sudo, or the current user if not sudo.
        // The os/user package is needed for UserCurrent.
        currentUser := os.Getenv("SUDO_USER")
        if currentUser == "" { // If not sudo, or SUDO_USER is empty
                u, err := user.Current() // Corrected: Use user.Current()
                if err != nil {
                        // If we can't get the current user, we can't set the service user.
                        return fmt.Sprintf("Error getting current user information for service user: %v", err), err
                }
                currentUser = u.Username
        }

        userHomeDir, err := os.UserHomeDir()
        if err != nil {
                return "Error getting user home directory.", err
        }

        messages.WriteString("Starting system service installation...\n")

        // 1. Copy executable to /usr/local/bin
        if err := copyFile(exePath, systemExecPathLinux); err != nil {
                return fmt.Sprintf("Error copying executable to %s: %v", systemExecPathLinux, err), err
        }
        messages.WriteString(fmt.Sprintf("- Copied executable to %s\n", systemExecPathLinux))

        // 2. Create systemd service file
        serviceTmpl := template.Must(template.New("service").Parse(serviceFileTemplate))
        // Correct error handling here. This 'serviceFile' and 'err' are local to this block.
        var serviceFile *os.File
        serviceFile, err = os.Create(systemServiceFileLinux)
        if err != nil {
                return fmt.Sprintf("Error creating service file at %s: %v", systemServiceFileLinux, err), err
        }
        defer serviceFile.Close()

        data := map[string]string{
                "User":      currentUser,
                "ExecPath":  systemExecPathLinux,
                "UserHome":  userHomeDir,
        }
        if err := serviceTmpl.Execute(serviceFile, data); err != nil {
                return "Error writing service file content.", err
        }
        messages.WriteString(fmt.Sprintf("- Created systemd service file at %s\n", systemServiceFileLinux))

        // 3. Enable and start the service
        if _, err := runCommand("systemctl", "daemon-reload"); err != nil {
                messages.WriteString(fmt.Sprintf("! Warning: systemctl daemon-reload failed: %v\n", err))
        } else {
                messages.WriteString("- Reloaded systemd daemon.\n")
        }
        if _, err := runCommand("systemctl", "enable", serviceName); err != nil {
                messages.WriteString(fmt.Sprintf("! Warning: systemctl enable failed: %v\n", err))
        } else {
                messages.WriteString("- Enabled conduit service to start on boot.\n")
        }
        if _, err := runCommand("systemctl", "start", "conduit.service"); err != nil { // Use explicit serviceName
                messages.WriteString(fmt.Sprintf("! Warning: systemctl start failed: %v\n", err))
        } else {
                messages.WriteString("- Started conduit service.\n")
        }

        messages.WriteString("\nSystem service installation complete. Conduit should now run as a background service.\n")
        return messages.String(), nil
}

// Uninstall uninstalls user-level and system-level installations for Linux.
func Uninstall() (string, error) {
        if runtime.GOOS != "linux" {
                return "Uninstall is only supported on Linux at this time.", fmt.Errorf("unsupported OS")
        }
        var messages strings.Builder
        messages.WriteString("Starting uninstallation for Linux...\n")
        homeDir, _ := os.UserHomeDir()
        userExecPath := filepath.Join(homeDir, userExecDirLinux, targetExecName)
        userDesktopPath := filepath.Join(homeDir, userDesktopFileDirLinux, desktopFileName)
        // Attempt to remove user-level install
        messages.WriteString("--- User-level Uninstall ---\n")
        if _, err := os.Stat(userExecPath); err == nil {
                if err := os.Remove(userExecPath); err != nil {
                        messages.WriteString(fmt.Sprintf("! Error removing user executable %s: %v\n", userExecPath, err))
                } else {
                        messages.WriteString(fmt.Sprintf("- Removed user executable: %s\n", userExecPath))
                }
        } else {
                messages.WriteString("- User executable not found, skipping removal.\n")
        }
        if _, err := os.Stat(userDesktopPath); err == nil {
                if err := os.Remove(userDesktopPath); err != nil {
                        messages.WriteString(fmt.Sprintf("! Error removing desktop file %s: %v\n", userDesktopPath, err))
                } else {
                        messages.WriteString(fmt.Sprintf("- Removed desktop file: %s\n", userDesktopPath))
                        // Update desktop database if desktop file was removed
                        if _, err := runCommand("update-desktop-database", filepath.Dir(userDesktopPath)); err != nil {
                                messages.WriteString(fmt.Sprintf("! Warning: 'update-desktop-database' failed after desktop file removal: %v\n", err))
                        }
                }
        } else {
                messages.WriteString("- Desktop file not found, skipping removal.\n")
        }
        // Attempt to remove system service install (requires root if it was installed by root)
        messages.WriteString("\n--- System Service Uninstall (requires root if installed) ---\n")
        if isRoot() && checkSystemctl() {
                if _, err := runCommand("systemctl", "stop", "conduit.service"); err != nil { // Use explicit serviceName
                        messages.WriteString(fmt.Sprintf("! Warning: Failed to stop service %s: %v\n", "conduit.service", err))
                } else {
                        messages.WriteString(fmt.Sprintf("- Stopped service: %s\n", "conduit.service"))
                }
                if _, err := runCommand("systemctl", "disable", "conduit.service"); err != nil { // Use explicit serviceName
                        messages.WriteString(fmt.Sprintf("! Warning: Failed to disable service %s: %v\n", "conduit.service", err))
                } else {
                        messages.WriteString(fmt.Sprintf("- Disabled service: %s\n", serviceName))
                }
                if _, err := os.Stat(systemServiceFileLinux); err == nil {
                        if err := os.Remove(systemServiceFileLinux); err != nil {
                                messages.WriteString(fmt.Sprintf("! Error removing service file %s: %v\n", systemServiceFileLinux, err))
                        } else {
                                messages.WriteString(fmt.Sprintf("- Removed service file: %s\n", systemServiceFileLinux))
                                if _, err := runCommand("systemctl", "daemon-reload"); err != nil {
                                        messages.WriteString(fmt.Sprintf("! Warning: systemctl daemon-reload failed after service file removal: %v\n", err))
                                }
                        }
                } else {
                        messages.WriteString("- System service file not found, skipping removal.\n")
                }
                if _, err := os.Stat(systemExecPathLinux); err == nil {
                        if err := os.Remove(systemExecPathLinux); err != nil {
                                messages.WriteString(fmt.Sprintf("! Error removing system executable %s: %v\n", systemExecPathLinux, err))
                        } else {
                                messages.WriteString(fmt.Sprintf("- Removed system executable: %s\n", systemExecPathLinux))
                        }
                } else {
                        messages.WriteString("- System executable not found, skipping removal.\n")
                }
        } else {
                messages.WriteString("- Not running as root or systemctl not available. Cannot attempt system service uninstall.\n")
        }
        messages.WriteString("\nLinux uninstallation attempt complete.\n")
        return messages.String(), nil
}