# recomp-launcher

The installer for a sp00nznet recomp: **here's my game, Build, here's my recomp.**

Each game repo builds its own installer from this one, with a
`launcher.json` describing the game. The player opens it, and it:

1. **Finds the game**: in the Steam libraries, or in the registry where a store
   (the EA App, say) records it. Not found, **Choose folder…** picks it.
2. **Builds**: downloads the game repo (at the commit the installer was built
   from) and its toolkit as zips, so no git is needed, then runs the repo's own
   setup script with no questions asked. Its `[3/11] What it's doing` lines
   drive the progress bar; the full output is under **Show details** and in
   `<build folder>/logs/`.
3. **Plays**: a **Play** button for each launcher the setup left, and
   desktop shortcuts on request. Opened again later, it starts here.

If the setup stops, it shows the script's own one-sentence reason and its
last lines, with **Try again**.

Windows, Linux and macOS, one Go program (Ebitengine).

## The rules it keeps

- **Nothing from any game is bundled or downloaded.** The build is made from
  the player's copy and stays on their machine.
- **The setup scripts stay the source of truth.** The installer knows what
  `launcher.json` says and nothing else: no build steps, no flags of its own.
  The scripts run the same from a terminal.

## Adding a game

In the game's repo:

1. `launcher/launcher.json` (redalert2-recomp's is [the sample](game/launcher.json)):

   | Key | |
   |---|---|
   | `title` | The heading |
   | `repo` | `owner/name` of the game's repo |
   | `accent` | `#rrggbb` for the buttons and the bar |
   | `note` | What the build needs and how long it takes, by OS (`windows`, `darwin`, `linux`) or `default` |
   | `game.name`, `game.files` | The game, and the files that prove a folder is it (a glob such as `*.iso` matches a file the player named; `a|b` is either, such as `HOVER.EXE|hover*.zip`) |
   | `game.steam` | Its folder under `steamapps/common` |
   | `game.registry` | Windows: `HKLM\key\value` entries holding its folder |
   | `sources` | More repos the setup expects beside the game's (`{"repo", "ref", "dir"}`), its toolkit |
   | `setup` | The setup command by OS or `default`; `{game}` is the game's folder. It should install what's missing without asking. |
   | `play` | A glob for the launchers the setup leaves in the repo's root |
   | `choices` | Optional, pick-your-own: `[{"id", "label", "note"}]`. Build first shows a box per choice; the ticked ids reach `setup` as `{choices}`, comma-separated (skifree's consoles installer lists its consoles here, so only the toolchains picked are downloaded) |

2. The setup prints each step as `[n/N] What it's doing` (for the bar), ends
   a failure with `Setup stopped: <why>`, exits non-zero on failure, and
   doesn't wait for Enter at the end when `RECOMP_LAUNCHER` is set.

3. `.github/workflows/installer.yml`:

   ```yaml
   on:
     push: { branches: [main], tags: ["v*"] }
   permissions: { contents: write }
   jobs:
     installer:
       uses: sp00nznet/recomp-launcher/.github/workflows/installer.yml@main
       with: { name: My-Game-Installer }
   ```

   Every push leaves the three installers as artifacts; a `v*` tag puts them
   on its release.
   A game whose setup runs on fewer systems adds `platforms: windows` (any of
   `windows linux macos`) to `with:`.

## Building and trying it

```
go run .                                       # the sample game in game/
go test ./...
go build -ldflags=-H=windowsgui -o installer.exe .   # Windows, no console window
go run . --shots screens                       # every screen as a PNG, then quit
go run . --build                               # start building at once
```

Linux needs `libgl1-mesa-dev xorg-dev libasound2-dev` (Ebitengine, with cgo).
A game's own installer is this with its `launcher.json` in `game/` and
`-ldflags "-X main.commit=<the game repo's commit>"`.

The installers aren't signed by a developer: Windows SmartScreen asks (**More
info**, **Run anyway**). On a Mac the app is signed ad hoc only: open it once,
then **Open Anyway** in System Settings, Privacy & Security. If macOS still
says it is damaged (an installer built before 0.2.1), run
`xattr -dr com.apple.quarantine` on the app.

## License

MIT: see [LICENSE](LICENSE).
