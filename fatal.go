package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// logPath is where the installer notes why it stopped: beside nothing it
// installs, so it survives the exe being moved or quarantined.
func logPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "recomp-launcher")
	os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "launcher.log")
}

// note appends a line to the log.
func note(format string, a ...any) {
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, a...))
}

// fatal: a windowed exe has no console, so an error that stops it is logged
// and shown (a message box on Windows) instead of vanishing with it.
func fatal(err error) {
	note("stopped: %v", err)
	fmt.Fprintln(os.Stderr, err)
	alert("The installer stopped: " + err.Error() + "\n\nDetails: " + logPath())
	os.Exit(1)
}

// guard logs a panic and its stack, then stops as fatal does.
func guard() {
	if r := recover(); r != nil {
		note("panic: %v\n%s", r, debug.Stack())
		fatal(fmt.Errorf("%v", r))
	}
}
