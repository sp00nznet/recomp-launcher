package main

// The all-games launcher: the same installer, opening on a list of every
// released game. Picking one fetches its launcher.json at its newest release
// tag, and from there it is that game's installer. Built with
// -ldflags "-X main.all=true", or run with --all.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed catalog.json
var catalogJSON []byte

var all = "false"

// entry is one game in catalog.json: a repo whose releases carry launcher/launcher.json.
type entry struct {
	Repo     string `json:"repo"`
	Title    string `json:"title"`
	Platform string `json:"platform"` // what it was made for: PC, N64, Xbox 360...
}

// platforms: the catalog's platforms, most games first, for the filter tabs.
func platforms(games []entry) []string {
	n := map[string]int{}
	var ps []string
	for _, e := range games {
		if n[e.Platform]++; n[e.Platform] == 1 {
			ps = append(ps, e.Platform)
		}
	}
	sort.SliceStable(ps, func(i, j int) bool { return n[ps[i]] > n[ps[j]] })
	return ps
}

func loadCatalog() ([]entry, error) {
	var c []entry
	return c, json.Unmarshal(catalogJSON, &c)
}

func get(url string) ([]byte, error) {
	c := http.Client{Timeout: 20 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// release: repo's newest release (pre-releases included) and its launcher.json there.
func release(repo string) (string, manifest, error) {
	b, err := get("https://api.github.com/repos/" + repo + "/releases?per_page=5")
	if err != nil {
		return "", manifest{}, err
	}
	tag, _ := newer(b, "")
	if tag == "" {
		return "", manifest{}, fmt.Errorf("%s has no release yet", repo)
	}
	if b, err = get("https://raw.githubusercontent.com/" + repo + "/" + tag + "/launcher/launcher.json"); err != nil {
		return "", manifest{}, err
	}
	m, err := loadManifest(b)
	return tag, m, err
}

// built: the player has built this game before, here or with its own installer.
func built(repo string) bool {
	d, _ := os.UserConfigDir()
	var st state
	b, _ := os.ReadFile(filepath.Join(d, "recomp-launcher", path.Base(repo)+".json"))
	return json.Unmarshal(b, &st) == nil && st.Built != ""
}

func (a *App) pick(e entry) {
	a.msg = "Getting " + e.Title + "…"
	go func() {
		tag, m, err := release(e.Repo)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.later = append(a.later, func() {
			if err != nil {
				a.msg = "Couldn't get " + e.Title + ": " + err.Error()
				return
			}
			commit, a.msg = tag, ""
			a.load(m)
			ebiten.SetWindowTitle(m.Title + " — recomp launcher " + version)
		})
	}()
}

func (a *App) toPicker() {
	a.m, a.scr, a.msg, a.accent = manifest{}, scPick, "", rgb(0xd0342c)
	for i, e := range a.games {
		a.isBuilt[i] = built(e.Repo)
	}
}

// drawPick: a tab per platform, then its games three to a row; a dot on the ones already built.
func (a *App) drawPick(dst *ebiten.Image) {
	x := 36.0
	for _, p := range append([]string{""}, platforms(a.games)...) {
		l := p
		if l == "" {
			l = "All games"
		}
		w := a.width(l, 15, regular) + 28
		a.button(dst, x, 110, w, 32, l, a.plat == p, true, func() { a.plat = p })
		x += w + 8
	}
	const cols, w, h, gap = 3, 268.0, 32.0, 12.0
	n := 0
	for i, e := range a.games {
		if a.plat != "" && e.Platform != a.plat {
			continue
		}
		x, y := 36+float64(n%cols)*(w+gap), 158+float64(n/cols)*(h+6)
		n++
		a.button(dst, x, y, w, h, a.fit(e.Title, 15, regular, w-40), false, true, func() { a.pick(e) })
		if a.isBuilt[i] {
			a.dot(dst, x+14, y+h/2, good)
		}
	}
}
