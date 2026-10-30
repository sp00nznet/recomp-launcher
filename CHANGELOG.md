# Changelog

Versions follow [SemVer](https://semver.org/).

## [Unreleased]

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
