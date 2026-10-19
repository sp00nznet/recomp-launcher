package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, p string, files map[string]string) {
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range files {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	f.Close()
}

func TestUnzipStripsTopFolderAndRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	writeZip(t, zp, map[string]string{"repo-abc/setup.sh": "echo", "repo-abc/tools/x.py": "pass"})
	dest := filepath.Join(dir, "out")
	if err := unzip(zp, dest); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(dest, "setup.sh")) || !fileExists(filepath.Join(dest, "tools", "x.py")) {
		t.Fatal("not extracted without its top folder")
	}
	writeZip(t, zp, map[string]string{"repo-abc/../../evil": "x"})
	if err := unzip(zp, dest); err == nil {
		t.Fatal("a path outside the folder was extracted")
	}
}

func TestStepLinesMoveTheBar(t *testing.T) {
	a := &App{lines: []string{""}}
	a.out("\n[3/11] Yuri's Revenge: analysing gamemd.exe\n  Already done.\n", nil)
	if a.step != 3 || a.steps != 11 || a.stepText != "Yuri's Revenge: analysing gamemd.exe" {
		t.Fatalf("step %d/%d %q", a.step, a.steps, a.stepText)
	}
	a.out("[4/11] Yuri's Revenge: lifting", nil) // not ended yet
	if a.step != 3 {
		t.Fatal("an unfinished line moved the bar")
	}
}

func TestGameFolderAndLabels(t *testing.T) {
	dir := t.TempDir()
	m := manifest{}
	m.Game.Files = []string{"gamemd.exe", "game.exe"}
	os.WriteFile(filepath.Join(dir, "gamemd.exe"), nil, 0o644)
	if m.isGame(dir) {
		t.Fatal("half the files is not the game")
	}
	os.WriteFile(filepath.Join(dir, "game.exe"), nil, 0o644)
	if !m.isGame(dir) || m.isGame("") {
		t.Fatal("isGame")
	}
	if got := playLabel("/x/Yuri's Revenge (recomp).cmd"); got != "Play Yuri's Revenge" {
		t.Fatal(got)
	}
	b, _ := gameFS.ReadFile("game/launcher.json")
	gm, err := loadManifest(b)
	if err != nil || gm.Repo == "" || len(perOS(gm.Setup)) == 0 || gm.Play == "" {
		t.Fatalf("game/launcher.json: %v %+v", err, gm)
	}
}
