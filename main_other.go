//go:build !darwin && !windows
// +build !darwin,!windows

package main

func main() {
	// On non-macOS systems, we just run the server directly.
	// Command-line arguments are handled inside.
	runConduitServer()
}
