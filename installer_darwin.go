//go:build darwin
// +build darwin

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Install path constants for macOS
const (
	userApplicationsDirMacOS = "Applications"
	targetExecName           = "conduit"
)

// macOS-specific Info.plist template
const infoPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>conduit</string>
	<key>CFBundleIdentifier</key>
	<string>com.jakbox.conduit</string>
	<key>CFBundleName</key>
	<string>Conduit</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLName</key>
			<string>Conduit URL Scheme</string>
			<key>CFBundleURLSchemes</key>
			<array><string>conduit</string></array>
		</dict>
	</array>
</dict>
</plist>
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

// checkIfInstalled checks if the executable is running from a known installation path.
func checkIfInstalled() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	appBundleName := "Conduit.app"
	targetAppDir := filepath.Join(homeDir, userApplicationsDirMacOS)
	macOSDir := filepath.Join(targetAppDir, appBundleName, "Contents", "MacOS")
	targetExecPath := filepath.Join(macOSDir, targetExecName)

	return exePath == targetExecPath
}

// InstallUser provides the user-level installation for macOS.
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

	messages.WriteString("Starting macOS user installation...\n")

	appBundleName := "Conduit.app"
	targetAppDir := filepath.Join(homeDir, userApplicationsDirMacOS)
	appBundlePath := filepath.Join(targetAppDir, appBundleName)
	contentsDir := filepath.Join(appBundlePath, "Contents")
	macOSDir := filepath.Join(contentsDir, "MacOS")
	targetExecPath := filepath.Join(macOSDir, targetExecName)
	infoPlistPath := filepath.Join(contentsDir, "Info.plist")

	if err := os.MkdirAll(macOSDir, 0755); err != nil {
		return fmt.Sprintf("Error creating macOS app directories: %v", err), err
	}
	messages.WriteString(fmt.Sprintf("- Created app bundle structure at %s\n", appBundlePath))

	if err := copyFile(exePath, targetExecPath); err != nil {
		return fmt.Sprintf("Error copying executable to app bundle: %v", err), err
	}
	messages.WriteString(fmt.Sprintf("- Copied executable to %s\n", targetExecPath))

	if err := os.WriteFile(infoPlistPath, []byte(infoPlistTemplate), 0644); err != nil {
		return fmt.Sprintf("Error writing Info.plist: %v", err), err
	}
	messages.WriteString(fmt.Sprintf("- Created Info.plist at %s\n", infoPlistPath))

	lsRegisterPath := "/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister"
	if _, err := runCommand(lsRegisterPath, "-f", appBundlePath); err != nil {
		messages.WriteString(fmt.Sprintf("! Warning: 'lsregister' failed: %v\n", err))
	} else {
		messages.WriteString(fmt.Sprintf("- Registered app bundle with Launch Services: %s\n", appBundlePath))
	}
	messages.WriteString("\nmacOS user installation complete. The 'conduit://' protocol handler should now be active.\n")

	return messages.String(), nil
}

// InstallService is not supported on macOS in this application.
func InstallService() (string, error) {
	return "Service installation is not supported on macOS.", fmt.Errorf("unsupported OS")
}

// Uninstall removes the user-level installation for macOS.
func Uninstall() (string, error) {
	var messages strings.Builder
	messages.WriteString("Starting macOS uninstallation...\n")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "Could not determine user home directory for uninstallation.", err
	}

	appBundleName := "Conduit.app"
	appBundlePath := filepath.Join(homeDir, userApplicationsDirMacOS, appBundleName)

	messages.WriteString("--- User-level Uninstall (macOS) ---\n")
	if _, err := os.Stat(appBundlePath); err == nil {
		if err := os.RemoveAll(appBundlePath); err != nil {
			messages.WriteString(fmt.Sprintf("! Error removing app bundle %s: %v\n", appBundlePath, err))
		} else {
			messages.WriteString(fmt.Sprintf("- Removed app bundle: %s\n", appBundlePath))
		}
	} else {
		messages.WriteString("- App bundle not found, skipping removal.\n")
	}

	messages.WriteString("\nmacOS uninstallation attempt complete.\n")
	return messages.String(), nil
}
