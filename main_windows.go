//go:build windows
// +build windows

package main

import (
	"os"
	"strings"
	"syscall"
)

const ATTACH_PARENT_PROCESS = ^uint32(0) // (DWORD)-1

var (
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	attachConsole = kernel32.NewProc("AttachConsole")
)

// This init function runs before main() and checks if we should attach to a console.
func init() {
	// If there are command-line arguments, we're likely being run manually.
	// We check for args other than the executable path itself, and we ignore
	// the single URL argument that a protocol launch would provide.
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "conduit://") {
		// Attach to the parent process's console.
		// This allows commands like `conduit --install-user` to print output.
		// We ignore the return value, as failure is not critical.
		attachConsole.Call(uintptr(ATTACH_PARENT_PROCESS))
	}
}

func main() {
	// On Windows, after potentially attaching to a console, we run the server directly.
	runConduitServer()
}
