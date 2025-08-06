//go:build !windows

package main

import (
	"io"
	"os/exec"

	"github.com/creack/pty"
)

// startPty returns an io.ReadWriteCloser and the *exec.Cmd for the PTY process.
func startPty(shell string, homeDir string) (io.ReadWriteCloser, *exec.Cmd, func(cols, rows int), error) {
	c := exec.Command(shell)
	c.Dir = homeDir
	c.Env = append(c.Env, "TERM=xterm-256color")

	if shell == "bash" || shell == "zsh" {
		c.Env = append(c.Env, `PROMPT_COMMAND=printf "\033]9;9;%s\033\\" "${PWD}"`)
	}

	// pty.Start returns an *os.File, which satisfies the io.ReadWriteCloser interface.
	ptmx, err := pty.Start(c)
	if err != nil {
		return nil, nil, nil, err
	}

	resizeFunc := func(cols, rows int) {
		pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}

	return ptmx, c, resizeFunc, nil
}
