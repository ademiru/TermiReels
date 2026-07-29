<p align="center">🔴 ━━━ 🟠 ━━━ 🟡 ━━━ 🟢 ━━━ 🔵 ━━━ 🟣</p>

```text
TTTTT EEEEE RRRR  M   M IIIII RRRR  EEEEE EEEEE L      SSSS
  T   E     R   R MM MM   I   R   R E     E     L     S
  T   EEEE  RRRR  M M M   I   RRRR  EEEE  EEEE  L      SSS
  T   E     R R   M   M   I   R R   E     E     L         S
  T   EEEEE R  RR M   M IIIII R  RR EEEEE EEEEE LLLLL SSSS
```

<p align="center">
  <code>INSTAGRAM REELS // KITTY GRAPHICS // LOCAL CHROMIUM // GO TUI</code>
</p>

TermiReels is a keyboard-first Instagram Reels client that plays inside a
graphics-capable terminal. Chromium keeps the authenticated web session;
TermiReels coordinates the feed, FFmpeg decodes the media, and the terminal
draws each frame through the Kitty graphics protocol.

`v1.5.1` · [Releases](https://github.com/ademiru/TermiReels/releases) ·
[CI](https://github.com/ademiru/TermiReels/actions/workflows/ci.yml) ·
[MIT](LICENSE)

> [!NOTE]
> This is an unofficial project. It is not affiliated with Instagram or Meta,
> and Instagram frontend changes can require client updates.

## Start here

### Windows 11 + WSL2

Open PowerShell and paste one command:

```powershell
$p="$env:TEMP\install-termireels.ps1"; irm https://raw.githubusercontent.com/ademiru/TermiReels/main/scripts/install-windows.ps1 -OutFile $p; powershell -ExecutionPolicy Bypass -File $p
```

The installer sets up WezTerm and Ubuntu 24.04 on WSL2, downloads the latest
stable package, verifies its SHA-256 checksum, and switches versions
atomically. Your browser session and settings live outside the installed
version, so an update does not erase either one.

If Windows asks for a restart, restart and run the same command again. Then
open WezTerm and enter:

```powershell
wsl -d Ubuntu-24.04
```

Inside Ubuntu, authenticate once:

```bash
termireels --login
```

After completing the Instagram login in the Chromium window:

```bash
termireels --creator-provider
```

The full Windows notes—including WSLg audio, updates, diagnostics and the
source-build route—are in [docs/windows-wsl.md](docs/windows-wsl.md).

### Linux and macOS

Building locally requires Go 1.25+, FFmpeg 8 development libraries, and a
Chromium-family browser:

```bash
git clone https://github.com/ademiru/TermiReels.git
cd TermiReels
go build -o termireels .
./termireels --login
```

Creator-specific Reels use an isolated Playwright provider. Node.js 20+ is
needed only when building that provider from source:

```bash
npm --prefix creator-provider ci
npm --prefix creator-provider run build
./termireels --creator-provider
```

TermiReels expects a terminal that implements Kitty graphics. Kitty, Ghostty,
WezTerm, iTerm2, Konsole and compatible `st` builds are suitable. Many IDE
terminals and nested multiplexers do not forward the necessary escape
sequences.

## What is different here?

TermiReels is not a rename-only fork. Its current runtime adds:

- a responsive cinema layout that uses the available terminal height;
- a stable footer with mouse targets, labels, seek and volume controls;
- a fixed right-side comments card on wide terminals;
- threaded replies, GIF-aware branches, and independent likes for replies;
- lazy continuation when the captured feed batch is exhausted;
- caption and music tickers that remain stable while content scrolls;
- audio prebuffering, underrun recovery and bounded playback restarts;
- follow controls and cancellable creator-feed transitions;
- verified creator feeds that never silently mix in recommendations;
- dedicated WSL2 transport, clipboard and WSLg audio handling.

## Daily controls

| Input | Result | Input | Result |
|:---:|---|:---:|---|
| `j` / `k` | next / previous | `h` / `l` | seek backward / forward |
| `space` | like / unlike | `r` | repost |
| `b` | save / unsave | `p` | pause / resume |
| `c` / `C` | comments open / close | `s` / `S` | share open / send |
| `d` / `D` | DM reels open / close | `x` / `X` | DM reactions |
| `u` | verified creator feed | `g` / `esc` | return to main feed |
| `f` | follow / unfollow | `m` | mute / unmute |
| `[` / `]` | volume down / up | `e` | footer labels |
| `?` | help | `q` / `ctrl+c` | quit |

The reel, creator name, progress bar, volume slider, action footer, comment
hearts and share panel also accept mouse input.

Bindings are editable:

```bash
termireels --shortcut
```

## Feed guarantees

Opening a creator feed is treated as a transaction, not a blind page jump:

```text
current reel
    │
    ├── keep main cursor untouched
    │
    └── request creator snapshot
            │
            ├── profile grid agrees?
            ├── target user id agrees?
            ├── reel metadata resolves?
            │
            └── yes to all ──▶ install isolated creator cursor
                                      │
                                      └── g ──▶ restore exact main position
```

The request has a deadline and can be cancelled. Pressing `j` or `k` while a
creator transition is pending keeps the main feed responsive and discards the
stale transition. Only complete, verified snapshots enter the short-lived
creator cache.

## Runtime map

```text
Instagram Web
      │ authenticated Chromium session
      ▼
feed capture ──▶ cursor coordinator ──▶ FFmpeg ──▶ Kitty frame transport
      │                   │                              │
      │                   └── comments / share / DM      ▼
      └── creator verifier                         terminal interface
```

The browser profile stays local. TermiReels never asks you to type your
Instagram password into the TUI.

## Command-line options

```text
--login                open Chromium for authentication
--headed               leave the controlled browser visible
--creator-provider     enable isolated creator feeds
--creator-audit        verify creator results without switching feeds
--shortcut             edit bindings and exit
--skip-terminal-check  bypass the graphics capability probe
--version              print the build version
```

## Files on disk

```text
~/.config/termireels/reels.conf             settings
~/.cache/termireels/                        downloaded media
~/.local/share/termireels/chrome-data/      Chromium profile
~/.local/state/termireels/                  logs and runtime state
```

An older `reels` directory is migrated only when the matching TermiReels
location is unused. Migration never merges or overwrites two existing data
trees. Settings are reloaded while the application runs.

## Verification

Pull requests exercise the Go code, race detector, `go vet`, production build,
creator-provider suite, and Windows/Bash installer syntax. To run the core
checks locally:

```bash
npm --prefix creator-provider test
go test -race ./...
go vet ./...
go build -buildvcs=false -o termireels .
```

Release tags additionally rebuild the FFmpeg environment and produce a
checksummed, self-contained Linux/WSL x86-64 archive.

## Known boundaries

- A real Instagram login and local Chromium profile are required.
- Instagram can change undocumented page structures and endpoints.
- The real-account creator audit is deliberately excluded from ordinary CI.
- Linux ARM64 needs a system Chromium, Chrome or Brave installation.
- Creator mode currently admits the first 12 verified entries; it stops there
  instead of filling the cursor with unrelated recommendations.

## Project history and license

TermiReels began as a modification of
[njyeung/reels](https://github.com/njyeung/reels). The original copyright
notice is retained, and both the original code and this derivative are
distributed under the [MIT License](LICENSE).

Twemoji assets have separate CC BY 4.0 terms. Redistribution details are in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Development history is in
[CHANGELOG.md](CHANGELOG.md), and contribution expectations are in
[CONTRIBUTING.md](CONTRIBUTING.md).

```text
┌─ TERMIREELS ────────────────────────────────────────────────────┐
│ authenticated locally · rendered directly · controlled by you │
└────────────────────────────────────────────────────────────────┘
```

<p align="center">🔴 ━━━ 🟠 ━━━ 🟡 ━━━ 🟢 ━━━ 🔵 ━━━ 🟣</p>
