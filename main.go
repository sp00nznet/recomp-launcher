// Recomp installer: one game's. Here's my game, press Build, here's my recomp.
//
// Each game repo builds this with its own game/launcher.json (CI:
// .github/workflows/installer.yml). It fetches the repo and its toolkit,
// runs the repo's own setup script on a pseudo terminal (package term), turns
// the script's "[n/N] step" lines into a progress bar and the launchers it
// leaves into Play buttons. Nothing from any game is bundled or downloaded:
// the build is made from the player's copy.
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/sp00nznet/recomp-launcher/term"
)

// LW, LH: the window, in device-independent pixels.
const LW, LH = 900, 580

//go:embed game
var gameFS embed.FS

// commit is the game repo's commit this installer fetches (-ldflags "-X main.commit=<sha>").
var commit = "main"

var windows = runtime.GOOS == "windows"

var (
	bg     = rgb(0x10131a)
	panel  = rgb(0x1a1f2b)
	edge   = rgb(0x2a3142)
	ink    = rgb(0xe8eaf0)
	dim    = rgb(0x9aa3b5)
	good   = rgb(0x4cc38a)
	warn   = rgb(0xe5b450)
	bad    = rgb(0xff6b6b)
	stepRe = regexp.MustCompile(`^\[(\d+)/(\d+)\]\s*(.+)$`)
)

func rgb(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

type screen int

const (
	scStart screen = iota
	scWork
	scDone
	scFail
)

// state is what the player chose and built, kept in the OS's config folder.
type state struct {
	Root  string `json:"root"`  // the game's repo and its toolkit go in here
	Game  string `json:"game"`  // the game's folder
	Built string `json:"built"` // the commit last built
}

type hit struct {
	r  image.Rectangle
	fn func()
}

type App struct {
	m       manifest
	st      state
	stPath  string
	accent  color.RGBA
	scr     screen
	found   string // where the game was found, for the start screen
	msg     string // a line under the buttons
	details bool

	k     float64 // device pixels per logical pixel
	faces map[[2]int]*text.GoTextFace
	srcs  [3]*text.GoTextFaceSource

	frame          int
	hits, nextHits []hit
	hover          bool

	mu        sync.Mutex // the job goroutine writes these
	lines     []string   // the log; the last is the line being written
	step      int
	steps     int
	stepText  string
	start     time.Time
	pid       int
	reason    string
	logPath   string
	later     []func()
	launchers []string

	shots    string // --shots: render each screen to a PNG there and quit
	shotN    int
	quitting bool
}

func main() {
	shots := flag.String("shots", "", "render each screen into this folder as a PNG, then quit")
	now := flag.Bool("build", false, "start building at once (the game found or chosen before)")
	flag.Parse()
	defer guard()
	note("start %s (%s)", os.Args[0], commit)
	a, err := newApp()
	if err != nil {
		fatal(err)
	}
	a.shots = *shots
	if *now {
		a.build()
	}
	ebiten.SetWindowTitle(a.m.Title + " — recomp installer")
	ebiten.SetWindowSize(LW, LH)
	ebiten.SetWindowClosingHandled(true)
	ebiten.SetTPS(30)
	if err := ebiten.RunGame(a); err != nil && err != ebiten.Termination {
		fatal(err)
	}
}

func newApp() (*App, error) {
	b, err := gameFS.ReadFile("game/launcher.json")
	if err != nil {
		return nil, err
	}
	m, err := loadManifest(b)
	if err != nil {
		return nil, fmt.Errorf("launcher.json: %w", err)
	}
	a := &App{m: m, lines: []string{""}, faces: map[[2]int]*text.GoTextFace{}, accent: rgb(0xd0342c)}
	if v, err := strconv.ParseUint(strings.TrimPrefix(m.Accent, "#"), 16, 32); err == nil {
		a.accent = rgb(uint32(v))
	}
	for i, ttf := range [][]byte{goregular.TTF, gobold.TTF, gomono.TTF} {
		if a.srcs[i], err = text.NewGoTextFaceSource(bytes.NewReader(ttf)); err != nil {
			return nil, err
		}
	}
	os.Setenv("RECOMP_LAUNCHER", "1") // the scripts' "Press Enter to close" waits for no one here
	if runtime.GOOS == "darwin" {     // an app opened from Finder has no shell's PATH: Homebrew isn't on it
		os.Setenv("PATH", "/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	}

	d, err := os.UserConfigDir()
	if err != nil {
		d = home()
	}
	a.stPath = filepath.Join(d, "recomp-launcher", m.name()+".json")
	a.st.Root = filepath.Join(home(), "recomp")
	if b, err := os.ReadFile(a.stPath); err == nil {
		json.Unmarshal(b, &a.st)
	}
	switch {
	case m.isGame(a.st.Game):
		a.found = "The folder you chose before"
	case m.isGame(filepath.Join(a.repoDir(), "game")):
		a.st.Game, a.found = filepath.Join(a.repoDir(), "game"), "Already in the build folder"
	default:
		a.st.Game, a.found = m.findGame()
	}
	a.findLaunchers()
	if len(a.launchers) > 0 {
		a.scr = scDone
	}
	return a, nil
}

func (a *App) repoDir() string { return filepath.Join(a.st.Root, a.m.name()) }

func (a *App) save() {
	os.MkdirAll(filepath.Dir(a.stPath), 0o755)
	b, _ := json.MarshalIndent(a.st, "", "  ")
	os.WriteFile(a.stPath, b, 0o644)
}

// findLaunchers: what the setup left in the repo's root to play with.
func (a *App) findLaunchers() {
	a.launchers = nil
	exts := map[string]bool{".sh": true, ".command": true}
	if windows {
		exts = map[string]bool{".cmd": true, ".lnk": true, ".bat": true}
	}
	m, _ := filepath.Glob(filepath.Join(a.repoDir(), a.m.Play))
	for _, p := range m {
		if exts[strings.ToLower(filepath.Ext(p))] {
			a.launchers = append(a.launchers, p)
		}
	}
	sort.Strings(a.launchers)
}

// ------------------------------------------------------------------ the build

func (a *App) build() {
	if !a.m.isGame(a.st.Game) {
		return
	}
	a.save()
	a.mu.Lock()
	a.scr, a.msg, a.details = scWork, "", false
	a.lines, a.step, a.steps, a.stepText, a.reason = []string{""}, 0, 0, "Getting the recomp's source", ""
	a.start = time.Now()
	a.mu.Unlock()

	logs := filepath.Join(a.st.Root, "logs")
	os.MkdirAll(logs, 0o755)
	a.logPath = filepath.Join(logs, a.m.name()+"-"+a.start.Format("20060102-150405")+".log")
	logf, _ := os.Create(a.logPath)
	if logf != nil {
		fmt.Fprintf(logf, "recomp installer: %s at %s, %s/%s\ngame: %s\nbuilding in: %s\n",
			a.m.Repo, commit, runtime.GOOS, runtime.GOARCH, a.st.Game, a.repoDir())
	}
	go func() {
		code := a.runBuild(logf)
		a.finish(code, logf)
	}()
}

func (a *App) runBuild(logf *os.File) int {
	srcs := append([]source{{a.m.Repo, commit, a.m.name()}}, a.m.Sources...)
	for _, s := range srcs {
		a.out(fmt.Sprintf("Downloading %s (%s)\n", s.Repo, s.Ref), logf)
		if err := fetch(s.Repo, s.Ref, filepath.Join(a.st.Root, s.Dir)); err != nil {
			a.out(err.Error()+"\n", logf)
			a.setReason("Could not download " + s.Repo + ". Check the internet connection and try again.")
			return -1
		}
	}
	argv := append([]string(nil), perOS(a.m.Setup)...)
	for i := range argv {
		argv[i] = strings.ReplaceAll(argv[i], "{game}", a.st.Game)
	}
	if len(argv) == 0 {
		a.setReason("This installer has no setup command for " + runtime.GOOS + ".")
		return -1
	}
	a.out("\n> "+strings.Join(argv, " ")+"\n", logf)
	p, err := term.Start(a.repoDir(), argv[0], argv[1:]...)
	if err != nil {
		a.out(err.Error()+"\n", logf)
		a.setReason("Could not start the setup: " + err.Error())
		return -1
	}
	a.mu.Lock()
	a.pid = p.Pid
	a.mu.Unlock()
	buf := make([]byte, 4096)
	for {
		n, err := p.Read(buf)
		if n > 0 {
			a.out(term.Clean(string(buf[:n])), logf)
		}
		if err != nil {
			break
		}
	}
	return p.Wait()
}

// out adds output to the log: \n ends a line (a "[n/N] step" one moves the
// bar), a lone \r redraws one.
func (a *App) out(s string, logf *os.File) {
	if logf != nil {
		logf.WriteString(strings.ReplaceAll(s, "\r", "\n"))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	last := len(a.lines) - 1
	for _, r := range s {
		switch r {
		case '\n':
			if m := stepRe.FindStringSubmatch(strings.TrimSpace(a.lines[last])); m != nil {
				a.step, _ = strconv.Atoi(m[1])
				a.steps, _ = strconv.Atoi(m[2])
				a.stepText = m[3]
			}
			a.lines = append(a.lines, "")
			last++
		case '\r':
			a.lines[last] = ""
		case '\t':
			a.lines[last] += "    "
		default:
			a.lines[last] += string(r)
		}
	}
	if len(a.lines) > 5000 {
		a.lines = a.lines[len(a.lines)-5000:]
	}
}

func (a *App) setReason(s string) {
	a.mu.Lock()
	a.reason = s
	a.mu.Unlock()
}

// finish: the launchers it left and a zero exit code are success; otherwise
// the scripts' own "Setup stopped: <why>" is the reason shown.
func (a *App) finish(code int, logf *os.File) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pid = 0
	if a.scr != scWork { // cancelled
		code = -2
	}
	a.later = append(a.later, func() {
		a.findLaunchers()
		if code == 0 && len(a.launchers) > 0 {
			a.st.Built = commit
			a.save()
			a.scr = scDone
		} else if code != -2 {
			a.scr = scFail
		}
	})
	if a.reason == "" {
		for i := len(a.lines) - 1; i >= 0; i-- {
			if _, why, ok := strings.Cut(a.lines[i], "Setup stopped:"); ok {
				a.reason = strings.TrimSpace(why)
				break
			}
		}
	}
	if a.reason == "" && code != 0 {
		a.reason = fmt.Sprintf("The setup ended with exit code %d.", code)
	}
	if a.reason != "" {
		a.reason = strings.ToUpper(a.reason[:1]) + a.reason[1:]
	}
	if logf != nil {
		fmt.Fprintf(logf, "\n== exit code %d after %s\n", code, time.Since(a.start).Truncate(time.Second))
		logf.Close()
	}
}

func (a *App) cancel() {
	a.mu.Lock()
	pid := a.pid
	a.scr, a.msg = scStart, "Stopped. Build again to carry on: finished steps are kept."
	a.mu.Unlock()
	if pid != 0 {
		term.Kill(pid)
	}
}

// ------------------------------------------------------------------ actions

func (a *App) chooseGame() {
	a.pickFolder("The folder "+a.m.Game.Name+" is installed in", func(p string) {
		if !a.m.isGame(p) {
			a.msg = "That folder has no " + a.m.filesText() + ". Pick the one the game is installed in."
			return
		}
		a.st.Game, a.found, a.msg = p, "Chosen by you", ""
	})
}

func (a *App) chooseRoot() {
	a.pickFolder("Where to build it", func(p string) { a.st.Root = p; a.findLaunchers() })
}

// pickFolder asks the OS for a folder, then runs then(path) on the UI's thread.
func (a *App) pickFolder(title string, then func(string)) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = term.Command("powershell", "-NoProfile", "-STA", "-Command",
			"Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; "+
				"$d.Description = $env:PICK_TITLE; if ($d.ShowDialog() -eq 'OK') { $d.SelectedPath }")
		cmd.Env = append(os.Environ(), "PICK_TITLE="+title)
	case "darwin":
		cmd = exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "`+title+`")`)
	default:
		if _, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.Command("zenity", "--file-selection", "--directory", "--title="+title)
		} else if _, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.Command("kdialog", "--getexistingdirectory", home(), "--title", title)
		} else {
			a.msg = "No folder picker here: install zenity or kdialog."
			return
		}
	}
	go func() {
		out, err := cmd.Output()
		p := strings.TrimSpace(string(out))
		if len(p) > 1 {
			p = strings.TrimRight(p, `/\`)
		}
		if err == nil && p != "" {
			a.mu.Lock()
			a.later = append(a.later, func() { then(p) })
			a.mu.Unlock()
		}
	}()
}

func (a *App) play(p string) {
	var cmd *exec.Cmd
	if windows { // start: a .cmd or .lnk, the way a double-click opens it
		cmd = term.Command("cmd", "/c", "start", "", filepath.Base(p))
	} else {
		cmd = exec.Command("/bin/sh", p)
	}
	cmd.Dir = filepath.Dir(p)
	if err := cmd.Start(); err != nil {
		a.msg = "Could not start it: " + err.Error()
		return
	}
	go cmd.Wait()
}

// desktop puts a shortcut to each launcher on the desktop (Windows) or in the
// applications menu and on the desktop (Linux).
func (a *App) desktop() {
	if windows {
		cmd := term.Command("powershell", "-NoProfile", "-Command",
			"$d = [Environment]::GetFolderPath('Desktop'); foreach ($t in $env:TARGETS -split '[|]') { "+
				"$s = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path $d ([IO.Path]::GetFileNameWithoutExtension($t) + '.lnk'))); "+
				"$s.TargetPath = $t; $s.WorkingDirectory = Split-Path $t; $s.Save() }")
		cmd.Env = append(os.Environ(), "TARGETS="+strings.Join(a.launchers, "|"))
		if err := cmd.Run(); err != nil {
			a.msg = "Could not make the shortcuts: " + err.Error()
			return
		}
		a.msg = "The shortcuts are on your desktop."
		return
	}
	dirs := []string{filepath.Join(home(), ".local", "share", "applications")}
	if out, err := exec.Command("xdg-user-dir", "DESKTOP").Output(); err == nil && isDir(strings.TrimSpace(string(out))) {
		dirs = append(dirs, strings.TrimSpace(string(out)))
	}
	for _, p := range a.launchers {
		name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=\"%s\"\nPath=%s\nTerminal=false\n",
			name, p, filepath.Dir(p))
		for _, d := range dirs {
			os.MkdirAll(d, 0o755)
			if err := os.WriteFile(filepath.Join(d, a.m.name()+"-"+slug(name)+".desktop"), []byte(entry), 0o755); err != nil {
				a.msg = "Could not make the shortcuts: " + err.Error()
				return
			}
		}
	}
	a.msg = "The shortcuts are in your applications menu and on the desktop."
}

func open(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = term.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

// ------------------------------------------------------------------ update

func (a *App) Layout(ow, oh int) (int, int) {
	a.k = ebiten.Monitor().DeviceScaleFactor()
	return int(float64(ow) * a.k), int(float64(oh) * a.k)
}

func (a *App) Update() error {
	a.frame++
	if ebiten.IsWindowBeingClosed() || a.quitting {
		a.mu.Lock()
		pid := a.pid
		a.mu.Unlock()
		if pid != 0 {
			term.Kill(pid)
		}
		return ebiten.Termination
	}
	a.mu.Lock()
	later := a.later
	a.later = nil
	a.mu.Unlock()
	for _, fn := range later {
		fn()
	}
	if a.shots != "" {
		a.shotScene()
	}
	a.hits = a.nextHits
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		p := image.Pt(int(float64(x)/a.k), int(float64(y)/a.k))
		for _, h := range a.hits {
			if p.In(h.r) {
				h.fn()
				break
			}
		}
	}
	if a.hover {
		ebiten.SetCursorShape(ebiten.CursorShapePointer)
	} else {
		ebiten.SetCursorShape(ebiten.CursorShapeDefault)
	}
	return nil
}

// ------------------------------------------------------------------ draw

const (
	regular = iota
	bold
	mono
)

func (a *App) face(size float64, kind int) *text.GoTextFace {
	key := [2]int{int(size * a.k * 4), kind}
	f, ok := a.faces[key]
	if !ok {
		f = &text.GoTextFace{Source: a.srcs[kind], Size: size * a.k}
		a.faces[key] = f
	}
	return f
}

func (a *App) text(dst *ebiten.Image, s string, x, y, size float64, kind int, c color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x*a.k, y*a.k)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, a.face(size, kind), op)
}

func (a *App) width(s string, size float64, kind int) float64 {
	w, _ := text.Measure(s, a.face(size, kind), 0)
	return w / a.k
}

// wrap breaks s into lines no wider than w.
func (a *App) wrap(s string, size float64, kind int, w float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			if line != "" && a.width(line+" "+word, size, kind) > w {
				out = append(out, line)
				line = word
			} else if line == "" {
				line = word
			} else {
				line += " " + word
			}
		}
		out = append(out, line)
	}
	return out
}

// fit cuts s in the middle (a path: its start and its end matter) to w.
func (a *App) fit(s string, size float64, kind int, w float64) string {
	r := []rune(s)
	for n := len(r) / 2; n > 4 && a.width(s, size, kind) > w; n-- {
		s = string(r[:n/2]) + " … " + string(r[len(r)-n/2:])
	}
	return s
}

func (a *App) rect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	vector.DrawFilledRect(dst, float32(x*a.k), float32(y*a.k), float32(w*a.k), float32(h*a.k), c, true)
}

func (a *App) round(dst *ebiten.Image, x, y, w, h, r float64, c color.Color) {
	r = min(r, h/2, w/2)
	a.rect(dst, x+r, y, w-2*r, h, c)
	a.rect(dst, x, y+r, w, h-2*r, c)
	for _, p := range [][2]float64{{x + r, y + r}, {x + w - r, y + r}, {x + r, y + h - r}, {x + w - r, y + h - r}} {
		vector.DrawFilledCircle(dst, float32(p[0]*a.k), float32(p[1]*a.k), float32(r*a.k), c, true)
	}
}

func (a *App) dot(dst *ebiten.Image, x, y float64, c color.Color) {
	vector.DrawFilledCircle(dst, float32(x*a.k), float32(y*a.k), float32(5*a.k), c, true)
}

func lighten(c color.RGBA, d int) color.RGBA {
	f := func(v uint8) uint8 { return uint8(min(255, max(0, int(v)+d))) }
	return color.RGBA{f(c.R), f(c.G), f(c.B), 255}
}

// button draws one and registers its click; primary ones are in the accent colour.
func (a *App) button(dst *ebiten.Image, x, y, w, h float64, label string, primary, on bool, fn func()) {
	c, fg := edge, ink
	if primary {
		c, fg = a.accent, rgb(0xffffff)
	}
	r := image.Rect(int(x), int(y), int(x+w), int(y+h))
	cx, cy := ebiten.CursorPosition()
	over := image.Pt(int(float64(cx)/a.k), int(float64(cy)/a.k)).In(r)
	switch {
	case !on:
		c, fg = panel, dim
	case over:
		c = lighten(c, 24)
		a.hover = true
	}
	a.round(dst, x, y, w, h, 8, c)
	size := 15.0
	if h >= 50 {
		size = 18
	}
	kind := regular
	if primary {
		kind = bold
	}
	a.text(dst, label, x+(w-a.width(label, size, kind))/2, y+(h-size*1.25)/2, size, kind, fg)
	if on {
		a.nextHits = append(a.nextHits, hit{r, fn})
	}
}

func (a *App) Draw(dst *ebiten.Image) {
	a.nextHits, a.hover = nil, false
	dst.Fill(bg)
	a.rect(dst, 0, 0, LW, 96, rgb(0x0b0e14))
	a.rect(dst, 0, 92, LW, 4, a.accent)
	a.text(dst, a.m.Title, 36, 20, 28, bold, ink)
	a.text(dst, "Recompiled for your computer, from your own copy of the game", 36, 58, 15, regular, dim)

	a.mu.Lock()
	switch a.scr {
	case scStart:
		a.drawStart(dst)
	case scWork:
		a.drawWork(dst)
	case scDone:
		a.drawDone(dst)
	case scFail:
		a.drawFail(dst)
	}
	a.mu.Unlock()
	if a.msg != "" {
		a.text(dst, a.fit(a.msg, 14, regular, LW-72), 36, 548, 14, regular, warn)
	}
	if a.shots != "" && a.frame%8 == 7 {
		a.saveShot(dst)
	}
}

func (a *App) card(dst *ebiten.Image, y, h float64, label string) {
	a.round(dst, 36, y, LW-72, h, 10, panel)
	a.text(dst, label, 56, y+16, 12, bold, dim)
}

func (a *App) drawStart(dst *ebiten.Image) {
	a.card(dst, 120, 120, "YOUR GAME")
	if a.m.isGame(a.st.Game) {
		a.dot(dst, 62, 167, good)
		a.text(dst, a.found, 76, 156, 17, bold, ink)
		a.text(dst, a.fit(a.st.Game, 13, mono, 600), 56, 190, 13, mono, dim)
	} else {
		a.dot(dst, 62, 167, warn)
		a.text(dst, a.m.Game.Name+" was not found yet", 76, 156, 17, bold, ink)
		a.text(dst, "Choose the folder it is installed in: the one with "+a.m.filesText()+".", 56, 190, 14, regular, dim)
	}
	a.button(dst, LW-56-170, 162, 170, 40, "Choose folder…", false, true, a.chooseGame)

	a.card(dst, 256, 100, "WHERE TO BUILD IT")
	a.text(dst, a.fit(a.repoDir(), 13, mono, 600), 56, 296, 13, mono, ink)
	a.text(dst, "The build stays here; your game folder is not changed.", 56, 320, 14, regular, dim)
	a.button(dst, LW-56-170, 288, 170, 40, "Change…", false, true, a.chooseRoot)

	y := 376.0
	for _, l := range a.wrap(perOS(a.m.Note), 15, regular, LW-72) {
		a.text(dst, l, 36, y, 15, regular, dim)
		y += 22
	}
	a.text(dst, "Nothing from the game is downloaded or shared.", 36, 506, 13, regular, dim)
	a.button(dst, LW-36-230, 478, 230, 54, "Build my recomp", true, a.m.isGame(a.st.Game), a.build)
}

func (a *App) drawWork(dst *ebiten.Image) {
	a.text(dst, "Building your recomp", 36, 120, 24, bold, ink)
	label := a.stepText
	if a.steps > 0 {
		label = fmt.Sprintf("Step %d of %d:  %s", a.step, a.steps, a.stepText)
	}
	a.text(dst, a.fit(label, 16, regular, LW-72), 36, 162, 16, regular, ink)

	// the bar: the steps done, and a sweep so it is plainly alive inside a long one
	frac := 0.02
	if a.steps > 0 {
		frac = max(frac, float64(a.step-1)/float64(a.steps))
	}
	bw := float64(LW - 72)
	a.round(dst, 36, 196, bw, 14, 7, panel)
	a.round(dst, 36, 196, bw*frac, 14, 7, a.accent)
	sweep := float64(a.frame%90) / 90
	a.round(dst, 36+bw*frac*sweep, 196, min(60, bw*frac), 14, 7, lighten(a.accent, 50))

	el := time.Since(a.start).Truncate(time.Second)
	a.text(dst, fmt.Sprintf("%s so far. The first build is long; you can leave it running.", el), 36, 222, 14, regular, dim)

	if a.details {
		a.round(dst, 36, 256, LW-72, 250, 8, rgb(0x0a0c11))
		n := 14
		start := max(0, len(a.lines)-n)
		for i, l := range a.lines[start:] {
			a.text(dst, a.fit(l, 12, mono, LW-104), 52, 268+float64(i)*16.5, 12, mono, dim)
		}
	}
	lbl := "Show details"
	if a.details {
		lbl = "Hide details"
	}
	a.button(dst, 36, 518, 160, 40, lbl, false, true, func() { a.details = !a.details })
	a.button(dst, LW-36-160, 518, 160, 40, "Cancel", false, true, a.cancel)
}

// player's label for a launcher: "Yuri's Revenge (recomp).cmd" plays Yuri's Revenge.
func playLabel(p string) string {
	n := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	return "Play " + strings.TrimSpace(strings.TrimSuffix(n, "(recomp)"))
}

func (a *App) drawDone(dst *ebiten.Image) {
	a.text(dst, "Ready to play", 36, 120, 24, bold, ink)
	a.text(dst, a.fit("Built in "+a.repoDir(), 14, regular, LW-72), 36, 156, 14, regular, dim)
	for i, p := range a.launchers {
		a.button(dst, 36, 196+float64(i)*72, 420, 58, playLabel(p), true, true, func() { a.play(p) })
	}
	x, y := LW-36-250.0, 196.0
	if runtime.GOOS != "darwin" {
		a.button(dst, x, y, 250, 42, "Add shortcuts to the desktop", false, true, a.desktop)
		y += 54
	}
	a.button(dst, x, y, 250, 42, "Open the build folder", false, true, func() { open(a.repoDir()) })
	y += 54
	rebuild := "Rebuild"
	if a.st.Built != "" && a.st.Built != commit {
		rebuild = "Update to this version"
	}
	a.button(dst, x, y, 250, 42, rebuild, false, true, func() { a.scr = scStart })
	a.text(dst, "In the game: F11 is fullscreen, F12 changes the scaling.", 36, 506, 14, regular, dim)
}

func (a *App) drawFail(dst *ebiten.Image) {
	a.text(dst, "The build stopped", 36, 120, 24, bold, bad)
	y := 160.0
	for _, l := range a.wrap(a.reason, 16, regular, LW-72) {
		a.text(dst, l, 36, y, 16, regular, ink)
		y += 24
	}
	// the script's last words: what to install, say
	var tail []string
	for i := len(a.lines) - 1; i >= 0 && len(tail) < 12; i-- {
		if l := strings.TrimRight(a.lines[i], " "); strings.TrimSpace(l) != "" {
			tail = append([]string{l}, tail...)
		}
	}
	top := max(y+8, 220)
	a.round(dst, 36, top, LW-72, 506-top, 8, rgb(0x0a0c11))
	for i, l := range tail {
		if ly := top + 12 + float64(i)*16.5; ly < 490 {
			a.text(dst, a.fit(l, 12, mono, LW-104), 52, ly, 12, mono, dim)
		}
	}
	a.button(dst, 36, 518, 160, 40, "Back", false, true, func() { a.scr = scStart })
	a.button(dst, 208, 518, 160, 40, "Open the log", false, a.logPath != "", func() { open(a.logPath) })
	a.button(dst, LW-36-200, 518, 200, 40, "Try again", true, true, a.build)
}

// ------------------------------------------------------------------ --shots

// shotScene sets up the scene for the next picture: every screen, with made-up output.
func (a *App) shotScene() {
	if a.frame%8 != 1 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.start = time.Now().Add(-754 * time.Second)
	a.lines = []string{"[2/11] Copying your copy of Red Alert 2 into game\\", "  Already in game\\ (skipping).", "",
		"[3/11] Yuri's Revenge: analysing gamemd.exe (seconds)", "  Already done (skipping).", "",
		"[4/11] Yuri's Revenge: finding every function (about 15 minutes, once)", "  Disassembling..."}
	a.step, a.steps, a.stepText = 4, 11, "Yuri's Revenge: finding every function (about 15 minutes, once)"
	a.msg = ""
	switch a.shotN {
	case 0: // a made-up install, so it shows as found
		d := filepath.Join(os.TempDir(), "SteamLibrary", "steamapps", "common", a.m.Game.Steam)
		os.MkdirAll(d, 0o755)
		for _, f := range a.m.Game.Files {
			f, _, _ = strings.Cut(f, "|") // the first alternative is enough
			os.WriteFile(filepath.Join(d, strings.NewReplacer("*", "game", "?", "x").Replace(f)), nil, 0o644)
		}
		a.scr, a.st.Game, a.found = scStart, d, "Found in your Steam library"
	case 1:
		a.scr, a.st.Game = scStart, ""
	case 2:
		a.scr, a.details = scWork, false
	case 3:
		a.scr, a.details = scWork, true
	case 4:
		a.scr = scDone
		a.launchers = []string{filepath.Join(a.repoDir(), "Yuri's Revenge (recomp).cmd"), filepath.Join(a.repoDir(), "Red Alert 2 (recomp).cmd")}
	case 5:
		a.scr, a.logPath = scFail, "x"
		a.reason = "the Visual Studio Build Tools did not install. Install them with 'Desktop development with C++', then run Setup.cmd again."
		a.lines = append(a.lines, "", "[1/11] Checking the tools the pipeline needs", "  Python: py -3",
			"  Visual Studio 2022 with the C++ tools is not installed.",
			"  Install the Visual Studio 2022 Build Tools with the C++ workload now (winget, about 3 GB, asks for admin)? yes", "",
			"Setup stopped: the Visual Studio Build Tools did not install. Install them with 'Desktop development with C++', then run Setup.cmd again.")
	default:
		a.quitting = true
	}
}

func (a *App) saveShot(dst *ebiten.Image) {
	b := dst.Bounds()
	img := image.NewRGBA(b)
	dst.ReadPixels(img.Pix)
	os.MkdirAll(a.shots, 0o755)
	if f, err := os.Create(filepath.Join(a.shots, fmt.Sprintf("screen-%d.png", a.shotN))); err == nil {
		png.Encode(f, img)
		f.Close()
	}
	a.shotN++
}

// ------------------------------------------------------------------ small

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func isDir(p string) bool      { st, err := os.Stat(p); return err == nil && st.IsDir() }
func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

func slug(s string) string {
	return strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(s), "-"), "-")
}
