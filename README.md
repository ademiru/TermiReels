# TermiReels

TermiReels is an unofficial Instagram Reels client for terminals that support
the Kitty graphics protocol. It drives a local Chromium session, downloads the
current video to a cache, and renders frames in the terminal.

This project is a modified version of
[njyeung/reels](https://github.com/njyeung/reels). The original project and
this derivative are distributed under the MIT License. TermiReels is not
affiliated with or endorsed by Instagram or Meta.

## Current status

The application is usable on Linux and macOS, but Instagram can change its
page structure and private GraphQL endpoints without notice. Features that
depend on those endpoints may require maintenance after an Instagram update.

Public releases are source-only for now. The repository's existing static
FFmpeg build is used for CI verification, not distributed as a release binary.

## Requirements

- Go 1.25 or newer
- FFmpeg 8 development libraries
- Chromium, Chrome, or Brave
- A terminal with Kitty graphics support

Tested terminal families include Kitty, Ghostty, WezTerm, iTerm2, Konsole and
st. Terminal multiplexers may not forward the required graphics escapes.

On Linux, install the FFmpeg development package supplied by your
distribution. On macOS, use `ffmpeg-full` from Homebrew or an equivalent
FFmpeg 8 build.

## Build

```bash
git clone https://github.com/ademiru/TermiReels.git
cd TermiReels
go build -o termireels .
./termireels
```

The repository URL will be filled in when the new upstream repository is
created.

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
| `p` | pause |
| `m` | mute |
| `[` / `]` | volume down / up |
| `e` | toggle the footer labels |
| `?` | help |
| `q` or `ctrl+c` | quit |

The footer, volume bar, reel progress bar, comment hearts and share panel also
support mouse input. All keyboard bindings can be changed with:

```bash
./termireels --shortcut
```

## Data files

TermiReels currently keeps compatibility with the original application's
paths:

```text
~/.config/reels/reels.conf
~/.cache/reels/
~/.local/share/reels/chrome-data/
~/.local/state/reels/
```

The configuration file is reloaded while the application is running.

## What this fork changes

- responsive reel sizing and a compact layout for small terminals
- fixed side panel for comments on wide terminals
- mouse controls for actions, seeking, volume and panels
- comment and reply likes, including GIF reply layout fixes
- lazy feed continuation instead of stopping at the captured cache boundary
- configurable shortcuts and live configuration reload
- stable scrolling captions and coloured music metadata
- audio prebuffering and underrun recovery

The detailed history is in [CHANGELOG.md](CHANGELOG.md).

## Limitations

- Instagram login and a local Chromium profile are required.
- Instagram frontend changes can break feed, comment or sharing operations.
- The black-box test requires a real account and is not run as part of a
  normal unit-test pass.
- Linux ARM64 requires an installed Chromium-family browser.

## License and attribution

The application source is available under the [MIT License](LICENSE). The
original copyright notice is retained as required by that license.

Embedded Twemoji graphics use a separate CC BY 4.0 license. See
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) before redistributing the
source or building release packages.
