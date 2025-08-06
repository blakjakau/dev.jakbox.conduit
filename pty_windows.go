//go:build windows

package main

import (
	"io"
	"os/exec"
	"log" // Add this import for logging
	"strings"

	"github.com/creack/pty" // Use the same library as Unix
)

// startPty returns an io.ReadWriteCloser and the *exec.Cmd for the PTY process.
// On modern Windows, this automatically uses the native ConPTY API.
func startPty(shell string, homeDir string) (io.ReadWriteCloser, *exec.Cmd, error) {
	c := exec.Command(shell)
	c.Dir = homeDir
	c.Env = append(c.Env, "TERM=xterm-256color")

	// The `pty.Start` function on Windows will automatically use ConPTY if it is available.
	ptmx, err := pty.Start(c)
	if err != nil {
		log.Printf("DEBUG: pty.Start failed with error: %v (Type: %T)", err, err) // Log the specific error
		return nil, nil, err
	}

	// Workaround for starting in the correct directory.
	// After the shell starts, we send a `cd` command to it.
	if strings.HasSuffix(strings.ToLower(shell), "powershell.exe") {
		// For PowerShell, we use Set-Location. Add a newline to execute it.
		ptmx.Write([]byte("Set-Location -Path '" + homeDir + "'\r\n"))
	} else { // Assume cmd.exe
		ptmx.Write([]byte("cd /d \"" + homeDir + "\"\r\n"))
	}

	return ptmx, c, nil
}
