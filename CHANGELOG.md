# Changelog

Versions follow [SemVer](https://semver.org/).

## [Unreleased]

## [0.7.0] - 2026-11-22

### Added
- 36 more in the catalog, 66 in all: 4 released before, 19 games, 7 apps
  (under PC), Stunt Race FX (SNES), and an OS tab: Windows 95, 98 SE,
  NT 4.0, 2000, XP and TempleOS. The list scrolls.
- `game.get` in launcher.json: an https page where the player can get the
  game themselves. Start shows **Where to get it…** under **Choose folder…**
  until the game is found, and opens the page in their browser.

### Fixed
- **Cancel** while the sources download stops them. Before, the download went
  on and the setup then started anyway, unseen, with no way to stop it but
  closing the window. Build is refused while a cancelled one is still
  stopping, so two never run in one folder.
- The setup and everything it started stop with **Cancel**, or with the
  installer if it is closed or dies mid-build. On Windows they run in a job
  object: `taskkill /T` missed a process whose parent had already exited,
  and one with a console of its own outlived the installer.
- An escape sequence or a UTF-8 character cut in two by a read no longer
  leaks into the log and the window (`[38;5;14m`, `�`), where it could also
  hide a `[n/N]` step from the bar. `tput sgr0`'s `ESC ( B` and titles ended
  by `ESC \` are cleaned too.
- Windows: a folder with accents picked with **Choose folder…**
  (`C:\Jogos\Coleção`) is read right, not in the OEM code page; the picker
  opens in front of the installer; a drive's root (a disc: `E:\`) stays
  `E:\`, here and from the registry.
- Windows: **Play** and **Open** use the shell's own open, not `cmd /c start`:
  a `%` or `^` in a path is just a character, and a launcher that can't start
  says so.
- A build folder with brackets in its path (`Games [old]`) finds its
  launchers; a glob in a subfolder of `game.files` (`DATA/*.iso`) matches.
- A path or log line too long for its space loses only what doesn't fit, not
  half of it. The bar's sweep stays inside the part done.
- The game list scrolls with the mouse wheel once it is longer than the
  window (a few more games would have fallen off it), and the window's title
  goes back to the list's after a game.
- A panic in the window's update or drawing is logged and shown, as one at
  start is: they run on Ebitengine's goroutine, which main's guard can't see.
- A setup command missing for this OS is said before the downloads, not
  after. Windows: one given as a path (`tools\setup.cmd`) is the repo's, as
  elsewhere.
- macOS: a `"` in a game's name can't break the folder prompt. Linux: desktop
  shortcuts quote their path as the Desktop Entry spec has it, and the Snap
  Steam library is searched.

## [0.6.0] - 2026-10-30

### Added
- The game list has a tab per platform (PC, N64, Xbox 360...); each game in
  `catalog.json` has a `platform`.

## [0.5.0] - 2026-10-30

### Added
- The all-games launcher (`-X main.all=true` or `--all`): a list of every
  game in `catalog.json`; picking one fetches its `launcher.json` at its
  newest release, then it is that game's installer, outputs included. Built
  and released as Recomp-Launcher by `all-games.yml`.
- README screenshots of every screen.

### Changed
- CI runs on macOS 15 (macOS 14 runners retire on 2026-11-02).

## [0.4.0] - 2026-10-22

### Added
- Pick-your-own builds: a `choices` list in `launcher.json` puts a screen of
  checkboxes between the start screen and the build, and the ticked ids reach
  the setup as `{choices}`. What was ticked is remembered for next time. For
  a game that builds for many systems (skifree's consoles), only what the
  player picks is built, so only its toolchain is downloaded.

## [0.3.0] - 2026-10-21

### Added
- The installer shows its version (the game's tag, or the short commit for a
  build off a branch) in the window's corner and title, and on start asks
  GitHub for the game's newest release: a newer one gets a download button.
  Offline it says nothing.
- `game.files` takes globs: `*.iso` makes a folder holding a disc image the
  game, for a title whose setup takes an image rather than an install.
- `installer.yml` takes `platforms` (default `windows linux macos`), so a
  Windows-only game doesn't publish installers that can't build it.
- A `game.files` entry may be alternatives, `a|b`: `HOVER.EXE|hover*.zip` is
  the game's folder or a folder holding a zip of it.

- The Windows installer carries version info and a manifest, and a release
  has `SHA256SUMS.txt`, so antivirus heuristics and players can check it.
- A log of why the installer stopped (`%APPDATA%\recomp-launcher\launcher.log`
  on Windows), and an error box instead of the window vanishing.

### Fixed
- On Windows a step line after a blank line was glued to the line before it
  (ConPTY skips the blank line with a cursor move), so the bar missed that step.

## [0.2.0] - 2026-10-19

One installer per game instead of one launcher for every game.

### Added
- A game's installer is this program built with the `launcher.json` from
  the game's repo, by the reusable workflow `.github/workflows/installer.yml`
  (Windows `.exe`, Linux `.tar.gz`, macOS `.app` zip; artifacts on every push,
  release files on a `v*` tag).
- Three screens: the game (found in Steam libraries or the registry, or
  chosen), building (a progress bar from the setup's `[n/N]` lines, details
  on request, cancel), and Play buttons with desktop shortcuts. A failed build
  shows the setup's own reason and **Try again**.
- No git needed: the game's repo (at the installer's commit) and its toolkit
  come as zips.
- `--shots DIR` renders every screen to PNGs; `--build` starts at once.

### Changed
- A plain look in place of the cracktro one.

### Removed
- The list of every recomp, the catalog tool and the triage report.

## [0.1.0]

The first launcher: every sp00nznet recomp with a setup script in one list,
set up and played from one app.
