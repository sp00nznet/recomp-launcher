package term

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The installer closed from Task Manager mid-build: what the setup started
// must not go on building unseen.
func TestTreeStopsWithTheInstaller(t *testing.T) {
	if dir := os.Getenv("TERM_TEST_INSTALLER"); dir != "" { // this is the installer
		startTree(t, dir)
		os.Exit(0) // gone, with no Kill
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestTreeStopsWithTheInstaller$")
	cmd.Env = append(os.Environ(), "TERM_TEST_INSTALLER="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	time.Sleep(6 * time.Second)
	if _, err := os.Stat(filepath.Join(dir, "late.txt")); err == nil {
		t.Fatal("what the setup started carried on without the installer")
	}
}
