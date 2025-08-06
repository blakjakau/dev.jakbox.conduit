//go:build windows
// +build windows

package main

import (
	"fmt"
	"io"
	"os/exec"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Install path constants for Windows
const (
	userLocalAppDataSubDirWindows = "Conduit"
	targetExecName                = "conduit"
)

// checkIfInstalled checks if the executable is running from a known installation path.
func checkIfInstalled() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return false
	}

	targetAppDir := filepath.Join(localAppData, userLocalAppDataSubDirWindows)
	installedPath := filepath.Join(targetAppDir, targetExecName+".exe")

	// Use EqualFold for case-insensitive path comparison on Windows.
	return strings.EqualFold(exePath, installedPath)
}

// InstallService is a placeholder for Windows service installation.
func InstallService() (string, error) {
	return "Service installation is not supported on Windows yet.", fmt.Errorf("unsupported OS")
}

// Uninstall handles the uninstallation process on Windows.
func Uninstall() (string, error) {
	var messages strings.Builder
	messages.WriteString("Starting Windows uninstallation...\n")

	// 1. Remove registry keys first, as this doesn't involve file locks.
	cmdReg := exec.Command("reg", "delete", `HKCU\Software\Classes\conduit`, "/f")
	regOut, errReg := cmdReg.CombinedOutput()
	// We check the output because `reg delete` returns an error if the key doesn't exist.
	// We only care about actual errors, not "key not found" messages.
	if errReg != nil && !strings.Contains(string(regOut), "cannot find") {
		messages.WriteString(fmt.Sprintf("! Warning during registry key deletion: %v\n", errReg))
	} else {
		messages.WriteString("- Removed registry entries for conduit:// protocol.\n")
	}

	// 2. Schedule self-deletion of files via a temporary batch script.
	// This is necessary because the running executable cannot delete itself.
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		msg := "! Error: LOCALAPPDATA environment variable not set. Cannot perform file uninstallation."
		messages.WriteString(msg)
		return messages.String(), fmt.Errorf("LOCALAPPDATA not set")
	}
	targetAppDir := filepath.Join(localAppData, userLocalAppDataSubDirWindows)

	// Create a temporary batch file to perform the deletion after we exit.
	batchFilePath := filepath.Join(os.TempDir(), "conduit_uninstall.bat")
	// The batch script waits 2 seconds, removes the application directory, and then deletes itself.
	batchContent := fmt.Sprintf(
		"@echo off\r\n"+
			"timeout /t 2 /nobreak > nul\r\n"+
			"rd /s /q \"%s\"\r\n"+
			"del \"%s\"\r\n",
		targetAppDir, batchFilePath,
	)

	if err := os.WriteFile(batchFilePath, []byte(batchContent), 0755); err != nil {
		msg := fmt.Sprintf("! Error creating uninstall script: %v", err)
		messages.WriteString(msg)
		return messages.String(), err
	}

	// Execute the batch script in a new, detached process.
	cmd := exec.Command("cmd.exe", "/C", "start", "/b", batchFilePath)
	if err := cmd.Start(); err != nil {
		msg := fmt.Sprintf("! Error starting uninstall script: %v", err)
		messages.WriteString(msg)
		return messages.String(), err
	}

	messages.WriteString(fmt.Sprintf("- Scheduled removal of application directory: %s\n", targetAppDir))
	messages.WriteString("\nWindows uninstallation process has been initiated.\n")
	messages.WriteString("The application will now exit. Files will be removed in the background shortly.\n")

	return messages.String(), nil
}

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

// InstallUser provides the user-level installation for Windows.
func InstallUser() (string, error) {
	if !isCompiledBuild {
		return "Installation must be run from a compiled binary.", fmt.Errorf("not a compiled build")
	}

	var messages strings.Builder
	exePath, _ := os.Executable()

	messages.WriteString("Starting user installation...\n")

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "LOCALAPPDATA environment variable not set.", fmt.Errorf("LOCALAPPDATA not set")
	}

	targetAppDir := filepath.Join(localAppData, userLocalAppDataSubDirWindows)
	targetExecPath := filepath.Join(targetAppDir, targetExecName+".exe")

	if err := os.MkdirAll(targetAppDir, 0755); err != nil {
		return fmt.Sprintf("Error creating directory %s: %v", targetAppDir, err), err
	}
	messages.WriteString(fmt.Sprintf("- Ensured directory exists: %s\n", targetAppDir))

	if err := copyFile(exePath, targetExecPath); err != nil {
		return fmt.Sprintf("Error copying executable to %s: %v", targetExecPath, err), err
	}
	messages.WriteString(fmt.Sprintf("- Copied executable to %s\n", targetExecPath))

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes`, registry.SET_VALUE|registry.CREATE_SUB_KEY)
	if err != nil {
		return fmt.Sprintf("Failed to open registry key Software\\Classes: %v", err), err
	}
	defer k.Close()

	conduitKey, _, err := registry.CreateKey(k, `conduit`, registry.SET_VALUE|registry.CREATE_SUB_KEY)
	if err != nil {
		return fmt.Sprintf("Failed to create registry key conduit: %v", err), err
	}
	defer conduitKey.Close()

	if err := conduitKey.SetStringValue("", "URL:Conduit Protocol"); err != nil {
		return fmt.Sprintf("Failed to set default value for conduit key: %v", err), err
	}
	if err := conduitKey.SetStringValue("URL Protocol", ""); err != nil {
		return fmt.Sprintf("Failed to set URL Protocol value for conduit key: %v", err), err
	}
	messages.WriteString("- Created registry key HKEY_CURRENT_USER\\Software\\Classes\\conduit\n")

	commandKey, _, err := registry.CreateKey(conduitKey, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return fmt.Sprintf("Failed to create registry key conduit\\shell\\open\\command: %v", err), err
	}
	defer commandKey.Close()

	commandValue := fmt.Sprintf(`"%s" "%%1"`, targetExecPath)
	if err := commandKey.SetStringValue("", commandValue); err != nil {
		return fmt.Sprintf("Failed to set command value for conduit protocol: %v", err), err
	}
	messages.WriteString(fmt.Sprintf("- Set registry command to: %s\n", commandValue))

	messages.WriteString("\nWindows user installation complete. The 'conduit://' protocol handler should now be active.\n")
	return messages.String(), nil
}
