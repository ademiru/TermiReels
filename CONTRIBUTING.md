# Contributing

Bug reports should include the operating system, terminal emulator, TermiReels
version and steps that reproduce the problem.

Before submitting a change, run:

```bash
gofmt -w .
go test ./...
go vet ./...
```

The main packages are:

- `backend/`: Chromium automation, Instagram requests, caching and settings
- `player/`: FFmpeg decoding, audio and Kitty graphics rendering
- `tui/`: Bubble Tea interface and input handling

Instagram changes its frontend frequently. Changes to GraphQL identifiers or
DOM selectors should include the date and the browser request or element used
to verify them. Do not include account cookies, tokens or personal request
headers in issues or commits.

Large changes should be discussed in an issue before implementation. Keep
commits focused and describe user-visible changes in `CHANGELOG.md`.

By contributing, you agree that your contribution may be distributed under
the repository's MIT License. Do not submit code, graphics or other material
that you do not have permission to redistribute.
