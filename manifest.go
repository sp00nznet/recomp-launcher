package main

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// manifest is the game's launcher.json, built into its installer (game/).
// Everything the installer knows about the game is in it; the rest is the
// repo's own setup script.
type manifest struct {
	Title  string            `json:"title"`  // the window's heading
	Repo   string            `json:"repo"`   // owner/name, fetched at the installer's commit
	Accent string            `json:"accent"` // "#rrggbb"
	Note   map[string]string `json:"note"`   // what the build needs and how long it takes, by GOOS or "default"
	Game   struct {
		Name     string   `json:"name"`
		Files    []string `json:"files"`    // a folder holding all of these is the game; "a|b": either
		Steam    string   `json:"steam"`    // its folder under steamapps/common
		Registry []string `json:"registry"` // Windows: HKLM\key\value naming its folder
	} `json:"game"`
	Sources []source            `json:"sources"` // more repos the setup expects beside the game's (its toolkit)
	Setup   map[string][]string `json:"setup"`   // the setup command by GOOS or "default"; {game} is the game folder
	Play    string              `json:"play"`    // a glob for the launchers the setup leaves in the repo's root
	Choices []choice            `json:"choices"` // pick-your-own: the ticked ids reach the setup as {choices}, comma-separated
}

// choice is one thing the player may tick before building (a console, say).
type choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Note  string `json:"note"` // a few words under the label: its size, what it runs on
}

// picked: the ids the player ticked that the manifest still offers, in its order.
func (m manifest) picked(ticked []string) []string {
	var ids []string
	for _, c := range m.Choices {
		for _, t := range ticked {
			if t == c.ID {
				ids = append(ids, c.ID)
				break
			}
		}
	}
	return ids
}

type source struct {
	Repo string `json:"repo"` // owner/name
	Ref  string `json:"ref"`  // a branch, tag or commit
	Dir  string `json:"dir"`  // beside the game's repo
}

func loadManifest(b []byte) (manifest, error) {
	var m manifest
	err := json.Unmarshal(b, &m)
	return m, err
}

func perOS[T any](m map[string]T) T {
	if v, ok := m[runtime.GOOS]; ok {
		return v
	}
	return m["default"]
}

func (m manifest) name() string { return path.Base(m.Repo) }

// filesText: the game's files for a sentence. "a|b" reads "a or b", and
// spellings that differ only in case ("GAME/MAIN.EXE|game/main.exe", for
// Linux installs) are shown once.
func (m manifest) filesText() string {
	var all []string
	for _, f := range m.Game.Files {
		var alts []string
		seen := map[string]bool{}
		for _, a := range strings.Split(f, "|") {
			if k := strings.ToLower(a); !seen[k] {
				seen[k] = true
				alts = append(alts, a)
			}
		}
		all = append(all, strings.Join(alts, " or "))
	}
	return strings.Join(all, " and ")
}

// isGame: dir holds every one of the game's files.
func (m manifest) isGame(dir string) bool {
	if dir == "" {
		return false
	}
	for _, f := range m.Game.Files {
		if !hasFile(dir, f) {
			return false
		}
	}
	return true
}

// hasFile: dir holds a file named f, or one matching it when f is a glob
// ("*.iso": a disc image whose name the player chose). The glob is matched
// against the names, not the path, so a folder like "Games [USA]" is safe.
// "a|b" is either: the game's folder or a zip or image of it.
func hasFile(dir, f string) bool {
	if alts := strings.Split(f, "|"); len(alts) > 1 {
		for _, a := range alts {
			if hasFile(dir, a) {
				return true
			}
		}
		return false
	}
	if !strings.ContainsAny(f, `*?[`) {
		return fileExists(filepath.Join(dir, f))
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		// case-blind: Windows and macOS folders are, and an ISO is as often .ISO
		if ok, _ := filepath.Match(strings.ToLower(f), strings.ToLower(e.Name())); ok && !e.IsDir() {
			return true
		}
	}
	return false
}

// findGame looks where stores install it: the registry entries, then every
// Steam library. It returns the folder and where it was found.
func (m manifest) findGame() (string, string) {
	for _, r := range m.Game.Registry {
		if d := strings.TrimRight(regValue(r), `\`); m.isGame(d) {
			return d, "Found on this computer"
		}
	}
	if m.Game.Steam != "" {
		for _, lib := range steamLibraries() {
			if d := filepath.Join(lib, "steamapps", "common", m.Game.Steam); m.isGame(d) {
				return d, "Found in your Steam library"
			}
		}
	}
	return "", ""
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// steamLibraries is every Steam library folder: each install's own, and the
// ones its libraryfolders.vdf lists.
func steamLibraries() []string {
	var roots []string
	switch runtime.GOOS {
	case "windows":
		roots = []string{regValue(`HKCU\Software\Valve\Steam\SteamPath`),
			regValue(`HKLM\SOFTWARE\WOW6432Node\Valve\Steam\InstallPath`), `C:\Program Files (x86)\Steam`}
	case "darwin":
		roots = []string{filepath.Join(home(), "Library", "Application Support", "Steam")}
	default:
		roots = []string{filepath.Join(home(), ".local", "share", "Steam"), filepath.Join(home(), ".steam", "steam"),
			filepath.Join(home(), ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam")}
	}
	var libs []string
	for _, r := range roots {
		if r == "" || !isDir(r) {
			continue
		}
		libs = append(libs, r)
		b, _ := os.ReadFile(filepath.Join(r, "steamapps", "libraryfolders.vdf"))
		for _, m := range vdfPath.FindAllStringSubmatch(string(b), -1) {
			libs = append(libs, strings.ReplaceAll(m[1], `\\`, `\`))
		}
	}
	return libs
}
