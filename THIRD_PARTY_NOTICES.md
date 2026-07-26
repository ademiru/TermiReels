# Third-party notices

TermiReels contains or uses components that are not covered solely by the
repository's MIT License.

## Original reels project

TermiReels is derived from
[njyeung/reels](https://github.com/njyeung/reels).

Copyright (c) 2026 Nicholas Yeung

The original source is used and modified under the MIT License. The complete
license text is retained in [`LICENSE`](LICENSE). The changes made by this fork
are described in [`CHANGELOG.md`](CHANGELOG.md) and the repository history.

## Twemoji graphics

Files under `player/emojis/` are Twemoji graphics obtained from the
[jdecked/twemoji](https://github.com/jdecked/twemoji) project. They are
embedded without visual modification and are used to render reaction emoji.

Copyright (c) 2014–2021 Twitter

Copyright (c) 2022–present Jason Sofonia & Justine De Caires

The graphics are licensed under the
[Creative Commons Attribution 4.0 International License](https://creativecommons.org/licenses/by/4.0/).
The upstream graphics license is available as
[`LICENSE-GRAPHICS`](https://github.com/jdecked/twemoji/blob/main/LICENSE-GRAPHICS).

Twemoji is not affiliated with or responsible for TermiReels.

## FFmpeg

The player links to FFmpeg through `go-astiav`. FFmpeg contains components
under the LGPL and, depending on its build configuration, the GPL.

TermiReels does not currently publish prebuilt FFmpeg-linked binaries. Anyone
distributing such a binary must determine the effective license of the exact
FFmpeg build and satisfy its corresponding source, notice, relinking and
installation-information requirements.

See [FFmpeg Legal](https://ffmpeg.org/legal.html) and the license files shipped
with the exact FFmpeg source used for the build.

## dav1d

The optional release build compiles the dav1d AV1 decoder. dav1d is distributed
under the two-clause BSD license. Its source and license are available from
[VideoLAN](https://code.videolan.org/videolan/dav1d).

## Go modules

Go dependencies are listed in `go.mod` and `go.sum`. Each dependency remains
under its own license. Redistributors are responsible for carrying the notices
required by the exact dependency versions included in their build.
