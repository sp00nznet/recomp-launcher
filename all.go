package main

// The all-games launcher: the same installer, opening on a list of every
// released game. Picking one fetches its launcher.json at its newest release
// tag, and from there it is that game's installer. Built with
// -ldflags "-X main.all=true", or run with --all.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
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
	a.picking++
	n := a.picking
	a.msg = "Getting " + e.Title + "…"
	go func() {
		defer guard()
		tag, m, err := release(e.Repo)
		a.ui(func() {
			if n != a.picking { // another game was clicked while this one came: that one is wanted
				return
			}
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
	ebiten.SetWindowTitle("Recomp launcher " + version) // not the last game's any more
	for i, e := range a.games {
		a.isBuilt[i] = built(e.Repo)
	}
}

// drawPick: a tab per platform, then its games three to a row; a dot on the
// ones already built. A list longer than the window scrolls with the wheel.
func (a *App) drawPick(dst *ebiten.Image) {
	x := 36.0
	for _, p := range append([]string{""}, platforms(a.games)...) {
		l := p
		if l == "" {
			l = "All games"
		}
		w := a.width(l, 15, regular) + 28
		a.button(dst, x, 110, w, 32, l, a.plat == p, true, func() { a.plat, a.scroll = p, 0 })
		x += w + 8
	}
	const cols, w, h, gap, top, bottom = 3, 268.0, 32.0, 12.0, 158, 540
	var shown []int
	for i, e := range a.games {
		if a.plat == "" || e.Platform == a.plat {
			shown = append(shown, i)
		}
	}
	view := float64(bottom - top)
	over := float64((len(shown)+cols-1)/cols)*(h+6) - 6 - view // how much doesn't fit
	a.scroll = max(0, min(a.scroll, over))
	// drawn on the list's part of the window only, and clicked there only
	list := dst.SubImage(image.Rect(0, int(top*a.k), int(LW*a.k), int(bottom*a.k))).(*ebiten.Image)
	a.clip = image.Rect(0, top, LW, bottom)
	for n, i := range shown {
		e := a.games[i]
		x, y := 36+float64(n%cols)*(w+gap), top+float64(n/cols)*(h+6)-a.scroll
		a.button(list, x, y, w, h, a.fit(e.Title, 15, regular, w-40), false, true, func() { a.pick(e) })
		if a.isBuilt[i] {
			a.dot(list, x+14, y+h/2, good)
		}
	}
	a.clip = image.Rectangle{}
	if over > 0 { // a scroll bar: there's more
		a.round(dst, LW-28, top+a.scroll/(over+view)*view, 4, view*view/(over+view), 2, edge)
	}
}
