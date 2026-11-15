package term

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	// what ConPTY sent for Write-Host 'two'; Write-Host ''; Write-Host '[2/5] three'
	got = Clean("\x1b[?1004l\r\ntwo\x1b[4;1H[2/5] three\r\n\x1b[?25h")
	if want := "\ntwo\n[2/5] three\n"; got != want {
		t.Fatalf("a skipped blank line: got %q, want %q", got, want)
	}
	// tput sgr0's character set, a title ended by ESC \, a CSI with an intermediate
	got = Clean("\x1b(B\x1b[mok\x1b]0;title\x1b\\ \x1b[0 qdone")
	if want := "ok done"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Output read 4096 bytes at a time is cut anywhere: however it's cut, the
// text is what the whole of it cleans to.
func TestCleanerHoldsWhatAReadCut(t *testing.T) {
	s := "\x1b[?25l\x1b[38;5;14m[1/3] Ünïcode\x1b]0;cmd.exe\x07\x1b[m\r\n" +
		"  Ask? [Y/n]:\x1b[1Cy\r\n\x1b(Bdone\x1b[4;1Hnext\r\nlast\r"
	want := "[1/3] Ünïcode\n  Ask? [Y/n]: y\ndone\nnext\nlast"
	for i := range len(s) + 1 {
		var c Cleaner
		if got := c.Clean([]byte(s[:i])) + c.Clean([]byte(s[i:])) + c.Flush(); got != want {
			t.Fatalf("cut at %d: got %q, want %q", i, got, want)
		}
	}
	var c Cleaner
	got := ""
	for i := range len(s) {
		got += c.Clean([]byte{s[i]})
	}
	if got += c.Flush(); got != want {
		t.Fatalf("a byte at a time: got %q, want %q", got, want)
	}
	if got := c.Clean([]byte("end\x1b[3")) + c.Flush(); got != "end" {
		t.Fatalf("an escape the output never finished: %q", got)
	}
}

// startTree runs a setup that starts a process and lets its parent exit, as
// a build tool's helper does; it returns once that process is running. Four
// seconds on, left alone, it writes late.txt in dir.
func startTree(t *testing.T, dir string) (p *Proc, ended <-chan struct{}) {
	write := func(name, s string) { os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644) }
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		ps := "Start-Process -WindowStyle Hidden -FilePath powershell -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File',"
		write("setup.ps1", ps+"'middle.ps1'\r\n"+
			"while (-not (Test-Path started.txt)) { Start-Sleep -Milliseconds 100 }\r\nWrite-Host ready\r\nStart-Sleep 60\r\n")
		write("middle.ps1", ps+"'late.ps1'\r\n")
		write("late.ps1", "Set-Content started.txt x\r\nStart-Sleep 4\r\nSet-Content late.txt x\r\n")
		name, args = "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "setup.ps1"}
	} else {
		write("setup.sh", "( (touch started.txt; sleep 4; touch late.txt) & )\n"+
			"while [ ! -e started.txt ]; do sleep 0.1; done\necho ready\nsleep 60\n")
		name, args = "bash", []string{"setup.sh"}
	}
	p, err := Start(dir, name, args...)
	if err != nil {
		t.Fatal(err)
	}
	ready, done := make(chan struct{}), make(chan struct{})
	go func() {
		var once sync.Once
		b, out := make([]byte, 4096), ""
		for {
			n, err := p.Read(b)
			if out += string(b[:n]); strings.Contains(Clean(out), "ready") {
				once.Do(func() { close(ready) })
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()
	select {
	case <-ready:
	case <-done:
		t.Fatal("the setup ended before it was ready")
	case <-time.After(60 * time.Second):
		t.Fatal("the setup was never ready")
	}
	return p, done
}

// Cancel stops the setup and all it started, a process whose parent has
// already gone included.
func TestKillStopsAllItStarted(t *testing.T) {
	dir := t.TempDir()
	p, ended := startTree(t, dir)
	p.Kill()
	<-ended
	p.Wait()
	p.Kill() // ended: nothing to do
	time.Sleep(6 * time.Second)
	if _, err := os.Stat(filepath.Join(dir, "late.txt")); err == nil {
		t.Fatal("a process the setup started carried on after Kill")
	}
}
