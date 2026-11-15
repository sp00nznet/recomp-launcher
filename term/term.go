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
	"unicode/utf8"
)

// Proc is a running command. Read its output until EOF, then Wait.
type Proc struct {
	io.Reader            // what it prints; ends when it and its console are gone
	io.Writer            // what the player types
	Pid       int        // for the log
	Wait      func() int // its exit code
	kill      func()
}

// Kill stops the command and everything it started: cmd, PowerShell, the
// compiler. Once the command has ended it does nothing.
func (p *Proc) Kill() { p.kill() }

// Answer is a line typed at a prompt: Enter is a carriage return, as a terminal sends it.
func Answer(w io.Writer, s string) error {
	_, err := io.WriteString(w, s+"\r")
	return err
}

var (
	cursorRight = regexp.MustCompile(`\x1b\[(\d*)C`)
	// ConPTY skips a blank line by moving to the next row's first column
	// (ESC[4;1H) instead of printing another newline.
	toLineStart = regexp.MustCompile(`\x1b\[\d+;1H`)
	// colours, cursor moves, window titles: the log is plain text. A CSI
	// (ESC [, parameters, intermediates, a final byte), an OSC (ESC ], ended
	// by BEL or ESC \), a character set (ESC ( B: what tput sgr0 sends), or
	// another two-byte one (ESC =, ESC 7...).
	escapes = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()*+][ -~]|\x1b[0-9<=>@-Z\\^_a-~]`)
)

// Clean turns terminal output into plain text. A lone \r is kept: it is a
// progress line (git's, say) going back to redraw itself.
func Clean(s string) string {
	s = cursorRight.ReplaceAllStringFunc(s, func(m string) string {
		n, err := strconv.Atoi(m[2 : len(m)-1])
		if err != nil {
			n = 1
		}
		return strings.Repeat(" ", n)
	})
	// ponytail: any jump to a row's start is a new line; a full-screen redraw
	// (not something a setup script does) would come out as extra lines.
	s = toLineStart.ReplaceAllString(s, "\n")
	return strings.ReplaceAll(escapes.ReplaceAllString(s, ""), "\r\n", "\n")
}

// Cleaner is Clean for output read a chunk at a time: an escape sequence, a
// UTF-8 character or a \r\n that a read cut in two is held back until the
// rest of it comes, instead of leaking into the text ("[38;5;14m") or
// turning into a replacement character.
type Cleaner struct{ tail string }

// Clean cleans the next chunk.
func (c *Cleaner) Clean(b []byte) string {
	s := c.tail + string(b)
	cut := unfinished(s)
	c.tail = s[cut:]
	return Clean(s[:cut])
}

// Flush gives what was held back, once the output has ended: an escape it
// never finished is dropped, and so is a last \r (it only moves the cursor).
func (c *Cleaner) Flush() string {
	s := c.tail
	c.tail = ""
	if i := strings.IndexByte(s, 0x1b); i >= 0 {
		s = s[:i]
	}
	return Clean(strings.TrimSuffix(s, "\r"))
}

// unfinished is where the escape sequence, UTF-8 character or \r\n that s
// ends in the middle of begins: len(s) when s ends on a boundary.
func unfinished(s string) int {
	// ponytail: a sequence that never ends isn't held for ever, only 256 bytes
	if i := strings.LastIndexByte(s, 0x1b); i >= 0 && len(s)-i < 256 && !escapeEnds(s[i:]) {
		return i
	}
	if strings.HasSuffix(s, "\r") {
		return len(s) - 1
	}
	for i := len(s) - 1; i >= 0 && i >= len(s)-utf8.UTFMax; i-- {
		if utf8.RuneStart(s[i]) {
			if !utf8.FullRuneInString(s[i:]) {
				return i
			}
			break
		}
	}
	return len(s)
}

// escapeEnds: e, an ESC and what follows it, is the whole sequence, or
// can't become one (so there's nothing to wait for).
func escapeEnds(e string) bool {
	if len(e) < 2 {
		return false
	}
	switch e[1] {
	case '[': // parameters and intermediates, then a final byte
		return strings.IndexFunc(e[2:], func(r rune) bool { return r < 0x20 || r > 0x3f }) >= 0
	case ']': // a window title, to its BEL
		return strings.IndexByte(e, '\x07') >= 0
	case '(', ')', '*', '+': // a character set
		return len(e) >= 3
	}
	return true
}
