# Creator Reels Provider

## Goal

Creator browsing must never mutate, contaminate, or delay the main Reels feed.
The feature is optional: if its provider is missing, unhealthy, or uncertain,
TermiReels continues in the main feed and follow/unfollow remains available.

The implementation uses a small TypeScript + Playwright sidecar. It attaches to
the Chromium instance TermiReels already exposes on loopback CDP port `6767`,
then creates its own page in the existing authenticated browser context. It
does not launch another browser, reuse the main-feed page, or own playback.

## Trust model

Neither a DOM selector nor an Instagram response is authoritative by itself.
Instagram can render recommendations under a profile and can return chaining
or suggested media in the same network response as profile media.

A shortcode is admitted only when all of these are true:

1. The page URL and active Reels tab resolve to the requested normalized
   username for three consecutive observations.
2. The shortcode appears as a tile-sized `/{username}/reel/{shortcode}/` link
   below the active tab, not merely anywhere under `<main>`.
3. The same shortcode appears in a profile-Reels network response whose
   `target_user_id` equals the resolved numeric user ID.
4. The numeric target ID is selected only from a response whose shortcodes
   overlap that creator-scoped grid.
5. That target response supplies the PK. A playable HTTPS payload is then
   accepted only from that same scoped response, Instagram's authenticated
   media-info endpoint for the verified PK, or the exact shortcode page.

Membership is the intersection of grid evidence and target-user response
evidence. This preserves collaboration posts displayed in the real grid while
excluding recommendations owned by unrelated creators.

If the provider cannot establish all five facts, it returns no item. It never
falls back to the main feed, generic `/reel/` links, or the first media edge in
a response.

## Process boundary

```text
Bubble Tea UI
     |
     v
Go CreatorFeedService ---- JSON-RPC/stdin+stdout ---- Playwright sidecar
     |                                                |
     |                                                v
     |                                      isolated Chromium page
     v
immutable CreatorCursor
     |
     v
existing downloader/player

Main FeedCursor and its Chromium page are never passed to the sidecar.
```

The sidecar is supervised by Go. One malformed line, crash, timeout, or protocol
version mismatch disables creator browsing for that request only. The process
can be restarted once with backoff; the main feed is not restarted.

## Protocol

Messages are newline-delimited JSON. Both sides discard unknown fields.

Request:

```json
{
  "version": 1,
  "id": "request-uuid",
  "method": "creator.resolve",
  "params": {
    "username": "normalized_name",
    "limit": 12
  }
}
```

Successful response:

```json
{
  "version": 1,
  "id": "request-uuid",
  "result": {
    "source_id": "snapshot-uuid",
    "username": "normalized_name",
    "instagram_user_id": "123456",
    "revision": 1,
    "items": [
      {
        "ordinal": 1,
        "shortcode": "ABC123",
        "pk": "987654",
        "owner_username": "creator_or_collaborator",
        "video_url": "https://...",
        "grid_seen": true,
        "target_response_seen": true
      }
    ],
    "next_cursor": "opaque-or-empty"
  }
}
```

Cancellation uses `creator.cancel`. Pagination is deliberately absent from
protocol revision 1; reaching the end of the verified page never falls back to
suggested media. A future `creator.more` method will carry `source_id`,
`revision`, and an opaque cursor.

The sidecar writes diagnostics to stderr only. Stdout is reserved for protocol
frames so log output can never corrupt a response.

## Playwright collection

1. Attach with `chromium.connectOverCDP("http://127.0.0.1:6767")`.
2. Use the existing default authenticated context and create one provider page.
3. Navigate directly to `/{username}/reels/`.
4. Wait until URL and active Reels tab agree for three consecutive
   observations.
5. Record only visible, tile-sized creator-scoped reel links below that tab;
   generic `/reel/` links are rejected.
6. Intercept profile-Reels requests and require their decoded
   `target_user_id` to equal the resolved profile user ID.
7. Preserve grid order and intersect grid shortcodes with response shortcodes.
8. Use complete playable payloads from the scoped response. Resolve summary
   payloads through the authenticated, read-only media-info endpoint using
   only their already verified PKs; open an exact shortcode page only for any
   item still missing playable metadata.
9. Return the first verified page. The protocol permits up to 24 entries;
   the revision-1 UI requests 12 so opening does not wait for speculative
   off-screen grid rows.

The page is reset to `about:blank` after completion and closed on cancellation.
No follow, like, comment, save, repost, or share action is executed by this
provider.

Complete verified snapshots are cached in the sidecar for 60 seconds. A cache
hit receives a fresh source ID and still passes the full Go validation. Failed,
partial, identity-mismatched, and empty results are never cached.

## Go integration

The backend exposes a narrow supervised client:

```go
type CreatorFeedProvider interface {
    Resolve(ctx context.Context, username string, limit int) (CreatorSnapshot, error)
    Close() error
}
```

Source switching is transactional:

1. Keep the main `FeedCursor` active and playing.
2. Request a creator snapshot without changing `active`, `ctx`, or cache state.
3. Validate the entire response again in Go: protocol version, normalized
   username, unique shortcode/PK, consecutive ordinals, both evidence flags,
   and non-empty playable URL.
4. Build a fully resolved `ProfileCursor` locally.
5. Under one mode lock, install a new source generation and switch `active`.
6. Keep the source ID and generation attached to the installed profile state.

Any failure before step 5 leaves the main feed untouched. Back/cancel restores
the exact main cursor because it was never navigated.

Downloads are pinned to the reel PK before asynchronous work starts. Cursor
membership is the exact `(shortcode, pk)` set from the validated snapshot;
numeric index alone is never accepted as identity.

## Cache and playback isolation

Creator and main-feed downloads share the bounded media cache because a
shortcode identifies the same Instagram reel in either source. Every download
pins and rechecks the PK before reading metadata, preventing a source switch
from redirecting an in-flight request to the same numeric index.

## Follow isolation

Follow/unfollow remains a separate service and is not part of
`CreatorFeedProvider`. It may use its own temporary page, but it cannot switch
the active cursor or open creator browsing. A provider outage therefore does
not remove the `FOLLOW` / `FOLLOWED` control.

## Failure policy

- Provider unavailable: keep playing main feed and show `CREATOR FEED UNAVAILABLE`.
- Empty/private profile: keep main feed and show a precise non-destructive
  message.
- Identity mismatch: reject the complete snapshot and log expected/actual IDs.
- Sidecar timeout: cancel the provider page, then kill and restart the sidecar
  once with backoff. Resolution has a 15-second upper bound.
- Verified-page boundary: retain the current item and report the boundary.
- Back or reel navigation during loading: cancel request immediately; reel
  navigation remains responsive and never waits for the provider timeout.
- Any unexpected main-feed cursor change during resolution is a test failure
  and a runtime invariant violation.

## Test gates

The feature is opt-in with `--creator-provider`; audit-only rollout uses
`--creator-audit`. The default main-feed reliability boundary does not include
Node or Playwright.

1. Sidecar fixture tests with profile items, recommendations, collaborations,
   missing usernames, reordered edges, and virtualized grids.
2. Go protocol tests for malformed JSON, stale generations, duplicate PKs,
   wrong username/user ID, timeouts, cancellation, and sidecar death.
3. Backend tests proving failed resolution leaves main cursor, current PK,
   Chromium context, comments, and cache namespace unchanged.
4. Playback tests proving equal numeric indices from two sources cannot share a
   file or completion event.
5. End-to-end tests across public, private, empty, collaboration-heavy, and
   recommendation-heavy profiles.
6. Rapid open/back/open and scroll-during-load stress tests under Go's race
   detector.

The provider should become a default setting only after these gates pass across
multiple real accounts and release platforms.

## Delivery order

1. Add the sidecar protocol and fixture-only collector.
2. Add the Go supervisor and fake-provider contract tests.
3. Run the real provider in audit mode: collect and log, but do not switch UI.
4. Compare provider results with the visible Instagram grid on multiple
   accounts.
5. Enable transactional creator cursor behind an explicit command-line flag.
6. Add packaging and release artifacts for supported platforms.

This order prevents an experimental provider from becoming part of the main
feed's reliability boundary.

## Local setup

```bash
npm --prefix creator-provider ci
npm --prefix creator-provider test
go build -buildvcs=false -o termireels .
./termireels --creator-provider
```

Use `u` or click the creator identity to request the verified creator feed.
Use `g`/`esc` or the `FEED` header control to restore the untouched main feed.
Follow/unfollow remains on `f` and the inline `FOLLOW` control.
