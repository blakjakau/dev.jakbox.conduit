//go:build windows

package main

import (
	"io"
	"os/exec"
	"log"
	"strings"
	"github.com/UserExistsError/conpty"
)

// startPty returns an io.ReadWriteCloser and the *exec.Cmd for the PTY process.
// On modern Windows, this automatically uses the native ConPTY API.
func startPty(shell string, homeDir string) (io.ReadWriteCloser, *exec.Cmd, func(cols, rows int), error) {
	// We create a placeholder exec.Cmd. The conpty library starts the process,
	// but does not expose the underlying *os.Process object.
	// Therefore, ptyCmd.Process will be nil. This is handled in handlers.go.
	ptyCmd := exec.Command(shell)
	p, err := conpty.Start(shell)
	if err != nil {
		log.Printf("ERROR: Failed to create ConPTY: %v", err)
		return nil, nil, nil, err
	}
	// The ReadWriteCloser interface is provided by the *Pty object itself.
	// Closing this ptmx object will correctly kill the underlying process.
	ptmx := io.ReadWriteCloser(p)
	resizeFunc := func(cols, rows int) {
		p.Resize(cols, rows) // Call the Resize method of the *Pty object
	}

	// Workaround for starting in the correct directory.
	if strings.HasSuffix(strings.ToLower(shell), "powershell.exe") {
		ptmx.Write([]byte("Set-Location -Path '" + homeDir + "'\r\n"))
	} else { // Assume cmd.exe
		ptmx.Write([]byte("cd /d \"" + homeDir + "\"\r\n"))
	}

	return ptmx, ptyCmd, resizeFunc, nil
}