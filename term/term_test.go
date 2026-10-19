package term

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A setup script's two kinds of question, Y/n and a path, asked the way the
// recomp repos ask them (Read-Host on Windows, read -p elsewhere), answered
// through the terminal, then a failure sentence and a non-zero exit.
func TestAsksAndAnswers(t *testing.T) {
	dir := t.TempDir()
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		os.WriteFile(filepath.Join(dir, "setup.ps1"), []byte("\xef\xbb\xbf"+ // a BOM, or PowerShell 5.1 reads it as ANSI
			"$a = Read-Host '  Install cmake? [Y/n]'\r\n"+
			"Write-Host \"got [$a]\"\r\n"+
			"$g = Read-Host '  Paste the folder'\r\n"+
			"Write-Host \"folder [$g] \u00fcn\u00efcode\"\r\n"+
			"Write-Host 'Setup stopped: not there.' -ForegroundColor Red; exit 3\r\n"), 0o644)
		name, args = "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(dir, "setup.ps1")}
	} else {
		os.WriteFile(filepath.Join(dir, "setup.sh"), []byte(
			"read -r -p '  Install cmake? [Y/n] ' a\n"+
				"echo \"got [$a]\"\n"+
				"read -r -p '  Paste the folder: ' g\n"+
				"echo \"folder [$g] \u00fcn\u00efcode\"\n"+
				"printf '\\033[31mSetup stopped: not there.\\033[0m\\n'; exit 3\n"), 0o755)
		name, args = "bash", []string{filepath.Join(dir, "setup.sh")}
	}
	p, err := Start(dir, name, args...)
	if err != nil {
		t.Fatal(err)
	}
	chunks := make(chan string)
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := p.Read(b)
			if n > 0 {
				chunks <- string(b[:n])
			}
			if err != nil {
				close(chunks)
				return
			}
		}
	}()
	var out string
	waitFor := func(s string) {
		deadline := time.After(20 * time.Second)
		for !strings.Contains(Clean(out), s) {
			select {
			case c, ok := <-chunks:
				if !ok {
					t.Fatalf("ended before %q:\n%s", s, Clean(out))
				}
				out += c
			case <-deadline:
				t.Fatalf("no %q in:\n%s", s, Clean(out))
			}
		}
	}
	waitFor("Install cmake? [Y/n]")
	Answer(p, "y")
	waitFor("got [y]")
	waitFor("Paste the folder")
	Answer(p, `C:\Games\RA2`)
	waitFor(`folder [C:\Games\RA2] ünïcode`)
	waitFor("Setup stopped: not there.")
	for c := range chunks {
		out += c
	}
	if code := p.Wait(); code != 3 {
		t.Fatalf("exit code %d, want 3", code)
	}
	if strings.Contains(Clean(out), "\x1b") {
		t.Fatalf("escapes left in %q", Clean(out))
	}
}

func TestClean(t *testing.T) {
	got := Clean("\x1b[?25l\x1b[2J\x1b[H\x1b[38;5;14mChecking\x1b]0;cmd.exe\x07\x1b[m\r\n  Ask? [Y/n]:\x1b[1Cy\r\n")
	if want := "Checking\n  Ask? [Y/n]: y\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
