# TermiReels

TermiReels is an unofficial Instagram Reels client for terminals that support
the Kitty graphics protocol. It drives a local Chromium session, downloads the
current video to a cache, and renders frames in the terminal.

This project is a modified version of
[njyeung/reels](https://github.com/njyeung/reels). The original project and
this derivative are distributed under the MIT License. TermiReels is not
affiliated with or endorsed by Instagram or Meta.

## Current status

The application is usable on Linux and macOS. Windows is supported through
WSL2 and a Windows-hosted WezTerm; see the
[Windows/WSL2 guide](docs/windows-wsl.md). Instagram can change its page
structure and private GraphQL endpoints without notice. Features that depend
on those endpoints may require maintenance after an Instagram update.

Public releases include source archives and a checksummed, self-contained
Linux/WSL x86-64 package. Other platforms currently build from source.

## Requirements

- Go 1.25 or newer
- FFmpeg 8 development libraries
- Chromium, Chrome, or Brave
- A terminal with Kitty graphics support
- Node.js 20 or newer (only for optional creator-Reels browsing)

Tested terminal families include Kitty, Ghostty, WezTerm, iTerm2, Konsole and
st. Terminal multiplexers may not forward the required graphics escapes.

On Linux, install the FFmpeg development package supplied by your
distribution. On macOS, use `ffmpeg-full` from Homebrew or an equivalent
FFmpeg 8 build.

On Windows, download and run the repository's installer from PowerShell. It
sets up the WSL2/WezTerm path and installs the checksummed release payload
without requiring Go, FFmpeg, Node.js or Docker:

```powershell
$p="$env:TEMP\install-termireels.ps1"; irm https://raw.githubusercontent.com/ademiru/TermiReels/main/scripts/install-windows.ps1 -OutFile $p; powershell -ExecutionPolicy Bypass -File $p
```

See the [Windows/WSL2 guide](docs/windows-wsl.md) for the first Ubuntu login,
updates and troubleshooting.

## Build

```bash
git clone https://github.com/ademiru/TermiReels.git
cd TermiReels
go build -o termireels .
./termireels
```

To enable the isolated creator-Reels provider, build its locked TypeScript
sidecar once:

```bash
npm --prefix creator-provider ci
npm --prefix creator-provider run build
./termireels --creator-provider
```

## Login

On first use:

```bash
./termireels --login
```

Complete the login in the Chromium window. Later runs reuse the same browser
profile.

Useful flags:

```text
--headed               keep the controlled browser visible
--login                open the browser for login
--creator-provider     enable isolated creator-Reels browsing
--creator-audit        verify creator results without changing the active feed
--shortcut             edit keyboard shortcuts and exit
--skip-terminal-check  bypass the Kitty graphics capability probe
--version              print the build version
```

## Controls

| Key | Action |
| --- | --- |
| `j` / `k` | next / previous reel |
| `h` / `l` | seek backward / forward |
| `space` | like |
| `r` | repost |
| `b` | save |
| `c` / `C` | open / close comments |
| `s` / `S` | open share panel / send and close |
| `d` / `D` | open / close reels shared in DMs |
| `x` / `X` | open / close reactions in DM mode |
| `u` | open the current creator's reels (provider mode) |
| `f` | follow or unfollow the creator |
| `g` or `esc` | return from creator reels to the main feed |
| `p` | pause |
| `m` | mute |
| `[` / `]` | volume down / up |
| `e` | toggle the footer labels |
| `?` | help |
| `q` or `ctrl+c` | quit |

The footer, creator name, volume bar, reel progress bar, comment hearts and
share panel also support mouse input. When enabled, creator reels are
cross-checked against both the visible profile grid and Instagram's
target-user response before a separate cursor is installed. Profile
verification does not block `j`/`k`; scrolling cancels the pending switch.
Returning to the main feed restores the reel where you left it. All
keyboard bindings can be changed with:

```bash
./termireels --shortcut
```

## Data files

```text
~/.config/termireels/reels.conf
~/.cache/termireels/
~/.local/share/termireels/chrome-data/
~/.local/state/termireels/
```

Legacy `reels` directories are moved on first start only when the corresponding
TermiReels directory does not already exist. A failed move falls back to the
legacy path without deleting or merging data. The configuration file is
reloaded while the application is running.

## What this fork changes

- responsive reel sizing and a compact layout for small terminals
- fixed side panel for comments on wide terminals
- mouse controls for actions, seeking, volume and panels
- comment and reply likes, including GIF reply layout fixes
- lazy feed continuation instead of stopping at the captured cache boundary
- configurable shortcuts and live configuration reload
- stable scrolling captions and coloured music metadata
- audio prebuffering and underrun recovery
- an opt-in Playwright provider that isolates and verifies creator-Reels feeds

The detailed history is in [CHANGELOG.md](CHANGELOG.md).

## Limitations

- Instagram login and a local Chromium profile are required.
- Instagram frontend changes can break feed, comment or sharing operations.
- The black-box test requires a real account and is not run as part of a
  normal unit-test pass.
- Linux ARM64 requires an installed Chromium-family browser.
- Creator-Reels browsing currently installs the first 12 verified entries;
  provider pagination remains disabled rather than falling back to
  recommendations.

## License and attribution

The application source is available under the [MIT License](LICENSE). The
original copyright notice is retained as required by that license.

Embedded Twemoji graphics use a separate CC BY 4.0 license. See
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) before redistributing the
source or building release packages.
