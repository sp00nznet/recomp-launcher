//go:build !windows

package term

import (
	"io"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// Start runs name in dir on a new pty, in a session of its own.
func Start(dir, name string, args ...string) (*Proc, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 250, Rows: 50})
	if err != nil {
		return nil, err
	}
	return &Proc{Reader: eofOnEIO{f}, Writer: f, Pid: cmd.Process.Pid, Wait: func() int {
		cmd.Wait()
		f.Close()
		return cmd.ProcessState.ExitCode()
	}}, nil
}

// Kill stops the command and everything it started.
func Kill(pid int) { syscall.Kill(-pid, syscall.SIGTERM) }

// On Linux a pty's reader gets EIO, not EOF, once the child has gone.
type eofOnEIO struct{ r io.Reader }

func (e eofOnEIO) Read(b []byte) (int, error) {
	n, err := e.r.Read(b)
	if err != nil && n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

// Command is exec.Command (Windows hides its console window).
func Command(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }
