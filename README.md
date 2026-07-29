<div align="center">

<img src="assets/banner.svg" width="760" alt="TermiReels animated ASCII banner">

<img src="assets/subtitle.svg" width="520" alt="Doomscroll in the terminal">

<p>
  <a href="https://github.com/ademiru/TermiReels/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/ademiru/TermiReels/ci.yml?branch=main&amp;style=for-the-badge&amp;logo=githubactions&amp;logoColor=white&amp;label=CI&amp;color=8b5cf6"></a>
  <a href="https://github.com/ademiru/TermiReels/releases"><img alt="Release" src="https://img.shields.io/github/v/release/ademiru/TermiReels?style=for-the-badge&amp;color=ff3d81"></a>
  <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-22c55e?style=for-the-badge"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&amp;logo=go&amp;logoColor=white">
</p>

<p><strong>Instagram Reels, rendered inside a real terminal.</strong></p>

<p>
  Keyboard-first navigation · Kitty graphics · Chromium-backed sessions ·
  comments, replies, likes, saves, shares, volume control and creator feeds
</p>

<p>
  <a href="#windows--wsl2--one-command">Windows / WSL2</a> ·
  <a href="#linux--macos">Linux &amp; macOS</a> ·
  <a href="#controls">Controls</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#limitations">Limitations</a>
</p>

</div>

---

## What is TermiReels?

TermiReels is an unofficial Instagram Reels client for graphics-capable
terminals. It drives a local Chromium session, keeps your login in a local
browser profile, fetches the active reel, and renders video frames through the
Kitty graphics protocol.

It is designed as a terminal application—not a browser page squeezed into a
terminal:

- responsive reel sizing from small panes to fullscreen terminals;
- a fixed right-side comments panel on wide layouts;
- keyboard and mouse controls for playback, actions, panels, seeking and volume;
- comment and reply likes, threaded replies and GIF-aware layouts;
- lazy feed continuation instead of stopping at the first captured batch;
- coloured, scrolling captions and music metadata;
- audio prebuffering and underrun recovery;
- verified, isolated creator-Reels browsing with an exact return to the main feed.

<table>
  <tr>
    <td align="center"><img src="assets/demo_arch.gif" alt="TermiReels on Arch Linux" width="280"></td>
    <td align="center"><img src="assets/demo_macos.gif" alt="TermiReels on macOS" width="378"></td>
    <td align="center"><img src="assets/demo_popos.gif" alt="TermiReels on Pop!_OS" width="252"></td>
  </tr>
  <tr>
    <td align="center"><sub>Arch Linux</sub></td>
    <td align="center"><sub>macOS</sub></td>
    <td align="center"><sub>Pop!_OS</sub></td>
  </tr>
</table>

> [!IMPORTANT]
> TermiReels is not affiliated with, maintained by, or endorsed by Instagram or
> Meta. Instagram can change its frontend and private endpoints without notice.

## Windows / WSL2 — one command

The supported Windows setup runs TermiReels inside **Ubuntu 24.04 on WSL2** and
uses **WezTerm on Windows** for Kitty graphics. Open PowerShell and run:

```powershell
$p="$env:TEMP\install-termireels.ps1"; irm https://raw.githubusercontent.com/ademiru/TermiReels/main/scripts/install-windows.ps1 -OutFile $p; powershell -ExecutionPolicy Bypass -File $p
```

The installer:

1. requests administrator permission;
2. installs WezTerm with `winget`;
3. installs or configures Ubuntu 24.04 as WSL2;
4. downloads the latest TermiReels WSL release;
5. verifies its SHA-256 checksum before installation;
6. installs versions atomically without deleting your login or configuration.

A new WSL installation may ask for one Windows restart. If it does, restart
Windows and run the same PowerShell command again. Ubuntu will also ask you to
create a Linux username and password on first launch.

Then open WezTerm:

```powershell
wsl -d Ubuntu-24.04
```

Log in once from the Ubuntu shell:

```bash
termireels --login
```

Complete the Instagram login in the Chromium window opened by WSLg. After that:

```bash
termireels --creator-provider
```

Run the same PowerShell installer later to update. The complete setup,
WSLg/audio notes, source-build path and diagnostics are in the
[Windows / WSL2 guide](docs/windows-wsl.md).

## Linux & macOS

### Requirements

- Go 1.25 or newer;
- FFmpeg 8 development libraries;
- Chromium, Chrome or Brave;
- a terminal that implements the Kitty graphics protocol;
- Node.js 20 or newer only for creator-Reels browsing.

Kitty, Ghostty, WezTerm, iTerm2, Konsole and `st` are supported terminal
families. A multiplexer or IDE terminal may not forward the required graphics
escapes.

### Build

```bash
git clone https://github.com/ademiru/TermiReels.git
cd TermiReels
go build -o termireels .
./termireels --login
```

To enable the isolated creator-Reels provider, build its locked Playwright
sidecar once:

```bash
npm --prefix creator-provider ci
npm --prefix creator-provider run build
./termireels --creator-provider
```

On Linux, install the FFmpeg development packages supplied by your
distribution. On macOS, use `ffmpeg-full` from Homebrew or another compatible
FFmpeg 8 build.

## Controls

| Key | Action | Key | Action |
|:---:|---|:---:|---|
| `j` / `k` | next / previous reel | `h` / `l` | seek backward / forward |
| `space` | like | `r` | repost |
| `b` | save | `p` | pause / resume |
| `c` / `C` | open / close comments | `s` / `S` | share panel / send |
| `d` / `D` | open / close DM reels | `x` / `X` | DM reactions |
| `u` | open creator's verified reels | `g` / `esc` | return to main feed |
| `f` | follow / unfollow creator | `m` | mute / unmute |
| `[` / `]` | volume down / up | `e` | toggle footer labels |
| `?` | help | `q` / `ctrl+c` | quit |

The footer, creator name, volume and progress bars, comment hearts and share
panel accept mouse input. All keyboard bindings can be changed:

```bash
termireels --shortcut
```

## Useful options

```text
--headed               keep the controlled browser visible
--login                open the browser for login
--creator-provider     enable isolated creator-Reels browsing
--creator-audit        verify creator results without changing the active feed
--shortcut             edit keyboard shortcuts and exit
--skip-terminal-check  bypass the Kitty graphics capability probe
--version              print the build version
```

## How it works

```text
   ┌──────────────────┐       authenticated session       ┌──────────────────┐
   │ Chromium profile │ ◀────────────────────────────────▶ │ Instagram Web    │
   └────────┬─────────┘                                   └────────┬─────────┘
            │ captured reel + metadata                              │
            ▼                                                       │
   ┌──────────────────┐      decode / audio prebuffer      ┌────────▼─────────┐
   │ feed coordinator │ ─────────────────────────────────▶ │ FFmpeg pipeline  │
   └────────┬─────────┘                                   └────────┬─────────┘
            │ transactional cursor                                  │ frames
            ▼                                                       ▼
   ┌──────────────────┐      Kitty graphics protocol       ┌──────────────────┐
   │ TUI + side panels│ ◀───────────────────────────────── │ terminal renderer│
   └──────────────────┘                                   └──────────────────┘
            ▲
            │ verified creator snapshot
   ┌────────┴─────────┐
   │ isolated provider│   Playwright · bounded requests · cancellation · cache
   └──────────────────┘
```

Creator feeds are not accepted from a guessed username alone. Results are
cross-checked against the visible profile grid and Instagram's target-user
response before a separate feed cursor is installed. Profile loading does not
block `j` / `k`; scrolling cancels a pending switch, and `g` restores the exact
main-feed position.

## Data and privacy

TermiReels operates a local browser profile and does not ask you to paste an
Instagram password into the terminal.

```text
~/.config/termireels/reels.conf
~/.cache/termireels/
~/.local/share/termireels/chrome-data/
~/.local/state/termireels/
```

Legacy `reels` directories are moved on first start only when the corresponding
TermiReels path is unused. If a move fails, the legacy path is retained; data
is not silently merged or deleted. Configuration is reloaded while the
application is running.

## Reliability

Every pull request is checked with Go tests, the race detector, `go vet`, the
creator-provider test suite, installer syntax checks and a production build.
The release workflow produces a checksummed Linux/WSL x86-64 package with its
own Node runtime for the optional creator provider.

For local verification:

```bash
npm --prefix creator-provider test
go test -race ./...
go vet ./...
go build -buildvcs=false -o termireels .
```

## Limitations

- Instagram login and a local Chromium profile are required.
- Instagram frontend changes can break feed, comment or sharing operations.
- The black-box creator test needs a real account and is not part of normal CI.
- Linux ARM64 currently requires a separately installed Chromium-family browser.
- Creator browsing installs the first 12 verified entries. Pagination remains
  disabled instead of silently mixing recommendations into a creator feed.

See [CHANGELOG.md](CHANGELOG.md) for the detailed history.

## Contributing

Bug reports and focused pull requests are welcome. Please read
[CONTRIBUTING.md](CONTRIBUTING.md) before changing browser selectors, playback
coordination or release packaging.

## License and attribution

TermiReels is a modified version of
[njyeung/reels](https://github.com/njyeung/reels). The original project and
this derivative are distributed under the [MIT License](LICENSE), and the
original copyright notice is retained.

Embedded Twemoji graphics are licensed separately under CC BY 4.0. See
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) before redistributing source
or release packages.

---

<div align="center">

<pre>
╭──────────────────────────────────────────────────────────╮
│  TERMIREELS // local chromium // terminal-native video  │
╰──────────────────────────────────────────────────────────╯
</pre>

<sub>Built for people who believe every application eventually becomes a terminal application.</sub>

</div>
