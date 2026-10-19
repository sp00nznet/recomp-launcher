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
		Files    []string `json:"files"`    // a folder holding all of these is the game
		Steam    string   `json:"steam"`    // its folder under steamapps/common
		Registry []string `json:"registry"` // Windows: HKLM\key\value naming its folder
	} `json:"game"`
	Sources []source            `json:"sources"` // more repos the setup expects beside the game's (its toolkit)
	Setup   map[string][]string `json:"setup"`   // the setup command by GOOS or "default"; {game} is the game folder
	Play    string              `json:"play"`    // a glob for the launchers the setup leaves in the repo's root
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

// isGame: dir holds every one of the game's files.
func (m manifest) isGame(dir string) bool {
	if dir == "" {
		return false
	}
	for _, f := range m.Game.Files {
		if !fileExists(filepath.Join(dir, f)) {
			return false
		}
	}
	return true
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
