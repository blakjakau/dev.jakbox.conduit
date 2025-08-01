//go:build windows
// +build windows

package main

import (
	"fmt"
	"io"
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

// InstallService is a placeholder for Windows service installation.
func InstallService() (string, error) {
	return "Service installation is not supported on Windows yet.", fmt.Errorf("unsupported OS")
}

// Uninstall is a placeholder for Windows uninstallation.
func Uninstall() (string, error) {
	return "Uninstall is not supported on Windows yet.", fmt.Errorf("unsupported OS")
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
