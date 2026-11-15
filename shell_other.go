//go:build !windows

package main

import "errors"

// shellOpen is Windows' (play and open use the OS's own commands elsewhere).
func shellOpen(string, string) error { return errors.New("only on Windows") }
