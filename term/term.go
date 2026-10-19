// Package term runs a command on a pseudo terminal, so a setup script prints
// its questions as it would in a terminal window and reads the answers typed
// back. A plain pipe isn't enough: PowerShell's Read-Host reads from one but
// never prints its question to it.
package term

import (
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Proc is a running command. Read its output until EOF, then Wait.
type Proc struct {
	io.Reader            // what it prints; ends when it and its console are gone
	io.Writer            // what the player types
	Pid       int        // for Kill
	Wait      func() int // its exit code
}

// Answer is a line typed at a prompt: Enter is a carriage return, as a terminal sends it.
func Answer(w io.Writer, s string) error {
	_, err := io.WriteString(w, s+"\r")
	return err
}

var (
	cursorRight = regexp.MustCompile(`\x1b\[(\d*)C`)
	// colours, cursor moves, window titles: the log is plain text
	escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]|\x1b\][^\x07]*\x07|\x1b[=>]`)
)

// Clean turns terminal output into plain text. A lone \r is kept: it is a
// progress line (git's, say) going back to redraw itself.
//
// ponytail: one chunk at a time, so an escape split across two reads leaks a
// few characters; carry the tail over if that ever shows in a log.
func Clean(s string) string {
	s = cursorRight.ReplaceAllStringFunc(s, func(m string) string {
		n, err := strconv.Atoi(m[2 : len(m)-1])
		if err != nil {
			n = 1
		}
		return strings.Repeat(" ", n)
	})
	return strings.ReplaceAll(escapes.ReplaceAllString(s, ""), "\r\n", "\n")
}
