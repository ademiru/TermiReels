# Changelog

## [Unreleased]

- Add a documented Windows 11 support path through WSL2 and WezTerm, including
  a reproducible FFmpeg 8 Docker build helper and Windows-host clipboard
  integration through `clip.exe`
- Add a one-command PowerShell installer and a checksummed WSL release bundle
  containing the FFmpeg-linked application, creator provider, Playwright
  runtime and pinned Node.js runtime
- Tune WSL transport by skipping impossible POSIX shared-memory probes,
  capping fit-mode direct Kitty frames at 720p and routing ALSA through WSLg's
  PulseAudio bridge without changing native Linux audio
- Add an opt-in TypeScript/Playwright creator-Reels provider with a versioned
  NDJSON protocol, supervised process lifetime, cancellation and strict Go
  payload validation
- Make creator-feed switching transactional: resolve and verify in an isolated
  browser page, prepare a separate cursor, then atomically switch without
  moving the main feed
- Require creator-grid links and `target_user_id` API responses to agree before
  admitting a shortcode, preserving verified collaborations while rejecting
  recommendations
- Resolve verified profile PKs through Instagram's authenticated read-only
  media-info path before bounded exact-page fallback, avoiding slow serial
  profile opens
- Keep reel navigation responsive while a creator feed is being verified;
  scrolling cancels the pending switch without touching the main cursor
- Cache only complete verified creator snapshots for 60 seconds, giving rapid
  reopen a fresh source identity without trusting partial or stale results
- Warm the optional provider while the main feed starts and request only the
  12 entries installed by protocol revision 1, reducing first-open latency
  without weakening creator-grid or target-user verification
- Add audit-only rollout, real-process supervisor tests, sidecar fixture tests
  and Node verification to CI and release checks
- Redesign startup as a full RGB brand stage with extruded 3D lettering, a running rainbow cat, shimmering status copy, responsive overflow protection, and professional offline messages
- Fix creator-profile opening stalls with bounded navigation, direct GraphQL reel resolution, and one-hop restoration of the source reel on failure
- Make creator browsing responsive with instant source-reel seeding, background profile hydration, isolated metadata resolution, three-reel preloading, and optimistic cached playback
- Pin profile prefetches to immutable reel PKs and cancel them at source transitions so rapid enter/exit input cannot download from the wrong cursor
- Isolate creator browsing in its own Chromium tab and route intercepted GraphQL responses by tab, keeping the main feed DOM, cursor, and lazy responses untouched
- Replace the fragile one-shot profile-grid selector with bounded React polling, cumulative virtual-grid collection, visible sync/error state, and no-refresh consumption of newly hydrated reels
- Switch the TUI into the hydrated creator grid automatically, preserving resolved PKs while installing Instagram's real profile order
- Require exact shortcode matches when resolving profile and DM reels so the first recommendation edge can never masquerade as the requested video
- Add isolated creator-Reels browsing, follow controls, clickable creator names, and reliable return to the previous main-feed position
- Stop infinite silent playback restart loops, expose background failures, and track rendered/dropped frames, errors, and recoveries
- Cancel stale reel prefetch requests immediately when navigation changes
- Migrate legacy `reels` data directories to `termireels` with no-overwrite and safe fallback behavior
- Cache the FFmpeg build environment between CI and release verification runs
- Unify footer button states and add stable scrolling caption/music tickers
- Render music metadata with a colourful brand-gradient treatment
- Smooth playback with audio prebuffering, automatic underrun recovery, and deeper packet queues
- UI: use one background-free active-state language across footer controls so coloured icons remain clear
- UI: add a compact cinema layout below 32 terminal rows, reserving only six chrome rows so the reel stays large in small windows
- UI: compact cinema keeps creator, direct volume and icon controls while hiding secondary metadata and footer labels; the full premium layout returns automatically when resized
- Fix: sanitize music metadata for terminal display, removing invalid UTF-8, invisible format/private-use glyphs and decorative symbol runs that rendered as replacement diamonds
- Fix: retry PK-based comment likes with the complete Instagram web header set, then use a guarded visible-button fallback
- Fix: child replies use the same independent comment-like path and retain their own optimistic/rollback state
- UI: add a precise mouse-draggable volume slider with percentage readout above the footer
- UI: add small aligned labels beneath every footer action, with the label row sharing the same click targets
- UI: group footer actions into engagement, library, and playback sections, with an automatic compact mode on narrow terminals
- UI: replace empty comment loading text with a stable skeleton and add navigation cues to the fixed header
- UI: render transient HUD messages as compact dark badges above the reel
- Fix: actively paint every terminal row so a footer from the previous frame cannot survive as a duplicate
- UI: top-anchor the reel and stop double-reserving vertical chrome, using the recovered rows to enlarge video
- UI: compact HUD messages into the small band above the enlarged reel
- UI: rebuild reel metadata as distinct creator, music, prose, and hashtag layers with controlled wrapping and a quiet one-line navigation hint
- Fix: reserve a terminal safety row below the footer and render a full-height frame, preventing emoji-width wrapping from leaving a duplicate action bar
- UI: long comment authors and metadata yield to the heart control instead of widening the fixed side card
- UI: comments show the current cursor position, and unavailable comment/share controls are visibly disabled
- UI: footer counters reserve stable space so controls do not shift between reels
- UI: action controls now live in a stable footer instead of above the reel
- UI: comments open beside the reel on wide terminals, with the below-reel layout retained as a responsive fallback
- Performance: initialize the system audio device only when an audio stream is actually opened
- Fix: comment likes now target the comment's stable Instagram PK instead of guessing from author and text
- Fix: GIF replies keep their thread branch visible through the animation rows
- Fix: reaching the captured reel boundary loads the next lazy feed page instead of behaving like the feed ended
- Replies are drawn as a thread: a tee for a reply with siblings below it, an elbow for the last of a group, and a rule carried down its wrapped lines
- Sending a reel over DM confirms itself above the reel with the number of friends it went to, instead of only ticking the share icon for a second
- Reposting announces itself above the reel as well as turning the icon
- The share panel's send button is a full-width SEND swept with the brand gradient
- Fix: one wheel notch moved several rows. Ghostty on Wayland forwards high-resolution scrolling, so a notch arrives as a burst of events; they are collapsed to one step
- `reels --shortcut` opens a shortcut editor: pick an action, press a key, done. Changes save immediately and a running reels picks them up without a restart
- The repost icon turns for a moment when you repost, and no longer shows a count — you have either reposted a reel or you haven't, so it only ever read 0 or 1
- Mouse support: scroll to move between reels or scroll the open panel, click the status icons, click the reel to pause
- Status controls highlight under the pointer, so the row reads as clickable
- Click a friend in the share panel to pick them, and a [ Send ] button in its header to send — sharing a reel no longer needs the keyboard at all
- Share panel: an explicit checkbox per friend instead of colour alone, a cursor bar, a running selected count, and the keys in the header
- Drag the reel's progress bar to scrub; the drag continues after the pointer leaves the bar, and the timecode is flashed as you go
- Like a comment by clicking its heart. Comment liking had no backend at all before — HasLikedComment was read from Instagram and never written
- Comments panel: age, like count and a heart on every comment; a cursor bar; replies marked with a vertical rule; an empty state instead of a blank panel
- Share panel: long friend names are clipped to the panel instead of wrapping and breaking the fixed rows-per-friend the avatars depend on
- Fix: in fit mode the view was one line taller than the terminal, so the terminal scrolled and every row shifted up. Mouse targets, which derive their rows from the reel position, then sat one row below what was drawn
- Colour pass: each status control has its own hue and lights up when active, hashtags are picked out in captions, navbar keys are picked out from their labels
- Long captions and music metadata scroll horizontally instead of being cut off with an ellipsis
- Fix: the caption started flush left, underneath the creator's avatar. Everything below the reel now shares one left edge clear of it
- Fix: status icons with ambiguous East Asian width (⇄ ↗ ⚐ ▶) could be drawn one or two columns wide depending on the terminal, putting every click target after them in the wrong place. All icons are now a fixed two columns
- The status row's click target includes the blank row beneath it, so the controls aren't a one-row sliver
- Fix: the music marquee stepped by rune instead of display column, so titles with emoji or CJK scrolled unevenly
- `reel_fit` (on by default) scales the reel to the terminal on every resize, at native device resolution
- Edits to `reels.conf` apply while running, no restart needed
- Log in from the login screen with `enter` instead of restarting with `--login`
- Probe the terminal for Kitty graphics support at startup and explain the problem instead of emitting garbage (`--skip-terminal-check` to bypass)
- Error screen now offers a retry and points at the log and browser profile
- Seeking flashes the new position
- Fix: `panel_shrink_steps` was written to the config as `panel_shrink`, so the setting never took effect
- Fix: rewriting the config no longer discards user comments and unrecognised keys
- Fix: config writes are atomic and serialized; rapid volume changes could previously interleave
- Fix: opening a panel no longer persists the shrunken reel size

## [1.4.1]
- Enable viewing comment replies (expand a comment to read its replies)
- Fix: consistent reel border and progress bar on macOS
- Fix: comment panel colors
- Fix: Update Instagram comments doc_ids to match new frontend

## [1.4.0]
- Reacting to reels that your friends have shared in dms (default x to open and X to close panel)
- Display your and your friends' reactions as floating twemojis on chat reels
- Shows a blue border and your friend's pfp while viewing their shared reels
- Fetch DM reel URLs directly from Instagram's API instead of waiting for the page to load, speed improves UX
- Revisit a friend's DM page when all shared reels have been seen
- Fix: memory leak on macOS, shared memory metadata map grew unbounded
- Fix: music ticker speed doubled on every reel load
- Fix: show loading spinner while syncing
- Fix: DM panel could open before shared reels were ready

## [1.3.3]
- Fix: Instagram frontend change breaks reel navigation (domPK() ancestor traversal off by 1)
- Update loading animation

## [1.3.2]
- Fix: Update Instagram comments doc_ids to match new frontend
- Move liked/repost badge right

## [1.3.1]
- Fix: Instagram CORs now blocks requests from the browser context. Switched to direct HTTP requests

## [1.3.0]
- View Reels shared by friends in DMs
- Renamed key_share_select to key_select, unified key for share and dm menu
- Fix: bug with prefetching reels

## [1.2.11]
- Add automatic self-update for npm-installed binaries on launch

## [1.2.10]
- Color @mentions blue in captions and comments
- Add heart/repost badge icons on floating reaction profile pictures
- Fix: resizing reel now repositions comment gifs
- Fix: Update Instagram comments pagination doc_id to match new frontend

## [1.2.9]
- Add reposting reels (default r)
- Show friend who have reposted/liked the current reel
- Fix: Update Instagram comments pagination doc_id to match new frontend
- Fix: fallback when mp4s have no audio stream
- Fix: video centering off-by-one
- Fix: colors 

## [1.2.8]
- Auto install Chrome if not found on system (except for Linux ARM64)
- Black box test harness, does not introduce any new code into user binary

## [1.2.7]
- Statically link ffmpeg for Linux and macOS
- No longer requires ffmpeg as a prerequisite

## [1.2.6]
- Adding seeking and progress overlay
- Add separate open/close binds for comments, share, and help panels. Configurable in reels.conf 
- Updated colors to align with instagram's colors a bit more
- Fix: video dimension and position calculations (off by one)
- Fix: prefetch index
- Fix: Removed redundant calls to updateVideoPosition

## [1.2.5]
- Add arm Linux support
- Add save button
- Optimize shared memory writing
- Fix: Disable actions (liking, opening panels) triggering while SyncTo is working
- Fix: Instagram's new send button
- Fix: comment prefetching to adjust to new comments layout
- Fix: video centering on non-16:9 reels
- Fix: video ready signal not firing at the right time

## [1.2.4]

- Use @rpath for macOS FFmpeg linking to support broader install locations
- Fix rendering loop responsiveness when paused for gifs and images
- Fix spelling

## [1.2.3]

- Fix loading text jitter
- Update loading messages

## [1.2.2]

- Add loading screen
- Add shared memory rendering package for macOS
- Fix video offset persistence
- Fix flickering profile pictures

## [1.2.1]

- Add help panel

## [1.2.0]

- Add DM sharing to friends
- Refactor image pipeline (profile pictures, frame pruning)
- Add comments pagination with loading indicator
- Unify file paths for macOS and Linux

## [1.1.7]

- Fix rendering bug
- Optimize profile picture rendering

## [1.1.6]

- Add sharing via link
- Add gif comment support
- Fix comments
- Fix cleanup race condition
- Refactor rendering loop

## [1.1.5]

- Add shared memory rendering for terminals with kitty protocol support
- Add user-defined keybinds via reels.conf
- Add npm package
- Fix shared memory cleanup on quit

## [1.1.4]

- Clean up comments UI

## [1.1.3]

- Add comments support
- Add music info display
- Add reel resizing with - and =
- Fix scrolling stabilization

## [1.1.2]

- Fix settings bug

## [1.1.1]

- Fix dynamic TUI sizing based on reel dimensions
- Add hardware decoding
- Fix verified badge placement
- Fix loading spinner position

## [1.1.0]

- Add adjustable video width and height
- Add retina display support
- Add persistent settings
- Add profile picture rendering
- Add AUR and Homebrew distribution
- Fix TUI layout centering
- Fix terminal resize positioning

## [1.0.1]

- Fix login flow

## [1.0.0]

- Initial release
