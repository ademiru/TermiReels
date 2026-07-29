# Windows through WSL2

The supported Windows path runs the Linux TermiReels binary inside WSL2 and
uses WezTerm on the Windows host for Kitty graphics. This keeps playback,
Chromium isolation and the creator provider on the same Linux architecture
used by CI.

## Requirements

- Windows 11 with WSL2 and WSLg
- An Ubuntu 24.04 WSL distribution
- WezTerm on Windows

Go, FFmpeg, Node.js and Docker are bundled or unnecessary for the release
installation.

## Automatic installation

Open PowerShell and run:

```powershell
$p="$env:TEMP\install-termireels.ps1"; irm https://raw.githubusercontent.com/ademiru/TermiReels/main/scripts/install-windows.ps1 -OutFile $p; powershell -ExecutionPolicy Bypass -File $p
```

The installer requests administrator permission, installs WezTerm through
`winget`, installs or configures Ubuntu as WSL2, and then downloads the latest
TermiReels release. A fresh WSL installation can require one Windows restart;
run the same command again afterward. Ubuntu also asks for a Linux username
and password on its first launch.

The WSL payload is checked against the SHA-256 file published with the GitHub
release before it is installed. Versions live under
`~/.local/share/termireels/versions/`; an atomic `current` link selects the
active release and `~/.local/bin/termireels` is the stable launcher.

## Login and run

WSLg displays the controlled Chromium login window on the Windows desktop:

```bash
termireels --login
termireels --creator-provider
```

Later runs reuse the Chromium profile stored inside the WSL home directory.
Do not run the repository or browser profile simultaneously from two WSL
distributions.

## Graphics and audio

Run TermiReels inside WezTerm. Windows Terminal's Kitty keyboard support is not
the Kitty graphics protocol used for video frames.

POSIX shared-memory image transfer cannot cross from WSL into a Windows-hosted
terminal. TermiReels detects WSL and directly uses the Kitty protocol's
chunked transfer without waiting on an unusable shared-memory probe. Fit mode
caps that transport at 720 pixels high to keep ConPTY responsive on large or
HiDPI windows; fixed-size mode remains user-controlled.

Audio is forwarded through WSLg. The installed launcher selects an isolated
PulseAudio-backed ALSA profile only when both WSL and `PULSE_SERVER` are
present, leaving native Linux audio configuration untouched. If the interface
works but audio is absent, update WSL and restart it:

```powershell
wsl --update
wsl --shutdown
```

Then reopen WezTerm and Ubuntu.

## Updating

Run the same PowerShell installer again. It downloads and verifies the latest
GitHub release, installs it alongside the old version and atomically switches
the launcher. Browser login and configuration data are stored separately and
remain intact.

## Building from source

Contributors can still build the pinned FFmpeg 8 environment with Docker
Desktop and the repository helper:

```bash
git clone https://github.com/ademiru/TermiReels.git
cd TermiReels
./scripts/build-wsl.sh --with-creator-provider
```

This source-build path requires Docker Desktop WSL integration and Node.js
20+. End users should use the automatic release installer above.

## Diagnostics

```bash
tail -n 100 ~/.local/state/termireels/reels.log
```

If the terminal capability probe fails, confirm the application is running in
WezTerm itself, not Windows Terminal, an IDE terminal or a nested multiplexer.
Do not use `--skip-terminal-check` as a permanent workaround.
