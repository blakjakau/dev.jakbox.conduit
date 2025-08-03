//go:build !linux && !darwin && !windows
// +build !linux,!darwin,!windows

package main

import "fmt"

// checkIfInstalled checks if the executable is in a known installation path.
func checkIfInstalled() bool {
	return false
}

// InstallUser provides a fallback for unsupported operating systems,
// ensuring that the build does not fail.
func InstallUser() (string, error) {
	return "User installation is not supported on this operating system.", fmt.Errorf("unsupported OS")
}

// InstallService provides a fallback for unsupported operating systems.
func InstallService() (string, error) {
	return "Service installation is not supported on this operating system.", fmt.Errorf("unsupported OS")
}

// Uninstall provides a fallback for unsupported operating systems.
func Uninstall() (string, error) {
	return "Uninstall is not supported on this operating system.", fmt.Errorf("unsupported OS")
}
