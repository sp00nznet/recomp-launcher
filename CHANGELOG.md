# Changelog

Versions follow [SemVer](https://semver.org/).

## [Unreleased]

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
