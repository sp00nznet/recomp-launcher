//go:build !windows

package term

import (
	"io"
	"os/exec"
	"sync"
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
	pid := cmd.Process.Pid
	var mu sync.Mutex
	ended := false
	return &Proc{Reader: eofOnEIO{f}, Writer: f, Pid: pid,
		kill: func() { // its session's process group: everything it started
			mu.Lock()
			defer mu.Unlock()
			if !ended { // once it's gone, its pid may be someone else's
				syscall.Kill(-pid, syscall.SIGTERM)
			}
		},
		Wait: func() int {
			cmd.Wait()
			mu.Lock()
			ended = true
			mu.Unlock()
			f.Close()
			return cmd.ProcessState.ExitCode()
		}}, nil
}

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
