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
	iso := filepath.Join(t.TempDir(), "Games [USA]")
	os.MkdirAll(filepath.Join(iso, "x.iso"), 0o755) // a folder named like one isn't one
	m.Game.Files = []string{"*.iso"}
	if m.isGame(iso) {
		t.Fatal("a folder matched a file glob")
	}
	os.WriteFile(filepath.Join(iso, "Burnout 3 - Takedown (USA).ISO"), nil, 0o644)
	if !m.isGame(iso) {
		t.Fatal("*.iso missed an image in a folder with brackets in its path")
	}
	m.Game.Files = []string{"HOVER.EXE|hover*.zip|FUNSTUFF/HOVER/HOVER.EXE"}
	if m.isGame(dir) {
		t.Fatal("none of the alternatives is there")
	}
	os.WriteFile(filepath.Join(dir, "hoverwindows95.zip"), nil, 0o644)
	if !m.isGame(dir) {
		t.Fatal("one alternative is enough")
	}
	cd := t.TempDir()
	os.MkdirAll(filepath.Join(cd, "FUNSTUFF", "HOVER"), 0o755)
	os.WriteFile(filepath.Join(cd, "FUNSTUFF", "HOVER", "HOVER.EXE"), nil, 0o644)
	if !m.isGame(cd) {
		t.Fatal("an alternative in a subfolder")
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

func TestFilesTextReadsAlternatives(t *testing.T) {
	m := manifest{}
	m.Game.Files = []string{"GAME/MAIN.EXE|game/main.exe|setup_*.exe", "a.dat"}
	if got, want := m.filesText(), "GAME/MAIN.EXE or setup_*.exe and a.dat"; got != want {
		t.Fatalf("filesText = %q, want %q", got, want)
	}
}

func TestNewerSkipsDraftsAndKnowsItsOwn(t *testing.T) {
	list := []byte(`[{"tag_name":"v0.4","draft":true},{"tag_name":"v0.3","html_url":"u3"},{"tag_name":"v0.2"}]`)
	if tag, url := newer(list, "v0.2"); tag != "v0.3" || url != "u3" {
		t.Fatal(tag, url)
	}
	if tag, _ := newer(list, "v0.3"); tag != "" {
		t.Fatal(tag)
	}
	if tag, _ := newer([]byte(`{"message":"rate limited"}`), "v0.3"); tag != "" {
		t.Fatal(tag)
	}
}

func TestPickedKeepsManifestOrderAndDropsUnknown(t *testing.T) {
	m, err := loadManifest([]byte(`{"choices": [{"id": "psp"}, {"id": "vita"}, {"id": "dreamcast"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := m.picked([]string{"dreamcast", "gone", "psp"})
	if len(got) != 2 || got[0] != "psp" || got[1] != "dreamcast" {
		t.Fatalf("picked = %v, want [psp dreamcast]", got)
	}
}
