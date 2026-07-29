package backend

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// FeedCursor navigates the main /reels page by scrolling. PKs are appended
// as Instagram returns clip responses (see processReelResponse). Discovery is
// implicit via fetch interception, not via the cursor itself.
type FeedCursor struct {
	ctx context.Context

	mu  sync.RWMutex
	pks []string

	syncMu     sync.Mutex
	syncCtx    context.Context
	syncCancel context.CancelFunc
}

// NewFeedCursor wires the cursor to the feed window's chromedp context.
func NewFeedCursor(ctx context.Context) *FeedCursor {
	return &FeedCursor{ctx: ctx}
}

// append records a newly captured PK at the tail. The caller (processReelResponse)
// has already deduped via the reels map, so no membership check is needed here.
// Caller must hold ChromeBackend.reelsMu so the b.reels insert and this append
// are atomic.
func (fc *FeedCursor) append(pk string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.pks = append(fc.pks, pk)
}

// Total returns the number of captured reels.
func (fc *FeedCursor) Total() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return len(fc.pks)
}

// PKAt returns the PK at 1-based index, or "" if out of range.
func (fc *FeedCursor) PKAt(index int) string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	if index < 1 || index > len(fc.pks) {
		return ""
	}
	return fc.pks[index-1]
}

// indexOf returns the 1-based index of pk in fc.pks, or 0 if absent. Caller
// must not hold fc.mu.
func (fc *FeedCursor) indexOf(pk string) int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	for i, p := range fc.pks {
		if p == pk {
			return i + 1
		}
	}
	return 0
}

// Current probes the DOM for the visible reel and resolves it to a 1-based
// index in the captured list.
func (fc *FeedCursor) Current() (int, string, error) {
	pk, err := fc.domPK()
	if err != nil {
		return 0, "", err
	}
	idx := fc.indexOf(pk)
	if idx == 0 {
		return 0, "", fmt.Errorf("reel pk=%s not in captured list", pk)
	}
	return idx, pk, nil
}

// domPK extracts the pk of the currently visible reel from the DOM.
func (fc *FeedCursor) domPK() (string, error) {
	return fc.domPKContext(fc.ctx)
}

func (fc *FeedCursor) domPKContext(ctx context.Context) (string, error) {
	var imgSrc string
	js := `
		(() => {
			const videos = document.querySelectorAll('video[playsinline]');
			for (const video of videos) {
				const rect = video.getBoundingClientRect();
				const viewportHeight = window.innerHeight;
				const videoCenter = rect.top + rect.height / 2;
				if (videoCenter > 0 && videoCenter < viewportHeight) {
					let parent = video.parentElement;
					for (let i = 0; i < 12; i++) {
						if (!parent) break;
						const img = parent.querySelector('img[src*="ig_cache_key"]');
						if (img) return img.src;
						parent = parent.parentElement;
					}
				}
			}
			return "";
		})()
	`

	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &imgSrc)); err != nil {
		return "", err
	}

	if imgSrc == "" {
		return "", fmt.Errorf("no visible reel found")
	}

	matches := pkRegex.FindStringSubmatch(imgSrc)
	if len(matches) < 2 {
		return "", fmt.Errorf("no ig_cache_key found")
	}

	decoded, err := url.QueryUnescape(matches[1])
	if err != nil {
		return "", err
	}
	b64Part := strings.Split(decoded, ".")[0]

	pkBytes, err := base64.StdEncoding.DecodeString(b64Part)
	if err != nil {
		return "", err
	}

	pk := string(pkBytes)
	if len(pk) > InstagramPKLength {
		pk = pk[:InstagramPKLength]
	}
	return pk, nil
}

// scrollDown sends a single ArrowDown to advance to the next reel.
func (fc *FeedCursor) scrollDown() error {
	return fc.scrollDownContext(fc.ctx)
}

func (fc *FeedCursor) scrollDownContext(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchKeyEvent(input.KeyDown).
				WithKey("ArrowDown").
				WithCode("ArrowDown").
				WithWindowsVirtualKeyCode(40).
				WithNativeVirtualKeyCode(40).
				Do(ctx)
		}),
	)
}

// scrollUp sends a single ArrowUp to go back to the previous reel.
func (fc *FeedCursor) scrollUp() error {
	return fc.scrollUpContext(fc.ctx)
}

func (fc *FeedCursor) scrollUpContext(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchKeyEvent(input.KeyDown).
				WithKey("ArrowUp").
				WithCode("ArrowUp").
				WithWindowsVirtualKeyCode(38).
				WithNativeVirtualKeyCode(38).
				Do(ctx)
		}),
	)
}

// SyncTo scrolls the feed window until the reel at index is the visible one.
// Cancels any in-flight SyncTo so a newer one can supersede it.
func (fc *FeedCursor) SyncTo(index int) error {
	fc.syncMu.Lock()
	if fc.syncCancel != nil {
		fc.syncCancel()
	}
	ctx, cancel := context.WithCancel(fc.ctx)
	fc.syncCtx = ctx
	fc.syncCancel = cancel
	fc.syncMu.Unlock()

	defer cancel()

	currentPK, _ := fc.domPKContext(ctx)

	fc.mu.RLock()
	if index < 1 || index > len(fc.pks) {
		fc.mu.RUnlock()
		return fmt.Errorf("index %d out of range", index)
	}
	targetPK := fc.pks[index-1]
	if currentPK == targetPK {
		fc.mu.RUnlock()
		return nil
	}
	fc.mu.RUnlock()

	for i := 0; i < MaxRetries; i++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		pk, err := fc.domPKContext(ctx)
		if err == nil && pk == targetPK {
			return nil
		}

		currentIndex := 0
		if err == nil {
			currentIndex = fc.indexOf(pk)
		}
		direction := feedSyncDirection(currentIndex, index, pk, targetPK)
		switch direction {
		case 1:
			if err := fc.scrollDownContext(ctx); err != nil {
				return err
			}
		case -1:
			if err := fc.scrollUpContext(ctx); err != nil {
				return err
			}
		default:
			// A missing/stale DOM identity is not directional evidence. Wait
			// for React to settle instead of guessing down and then correcting
			// upward on the next pass.
			if err := waitFeedSync(ctx, 150*time.Millisecond); err != nil {
				return nil
			}
			continue
		}

		// One key event per observed identity change prevents high-latency DOM
		// updates from receiving a burst of duplicate scrolls.
		for poll := 0; poll < 12; poll++ {
			if err := waitFeedSync(ctx, 100*time.Millisecond); err != nil {
				return nil
			}
			observed, observeErr := fc.domPKContext(ctx)
			if observeErr == nil && observed == targetPK {
				return nil
			}
			if observeErr == nil && observed != "" && observed != pk {
				break
			}
		}
	}

	return fmt.Errorf("failed to sync to index %d after %d scrolls", index, MaxRetries)
}

// feedSyncDirection returns a direction only when the DOM provides a known
// position. Equal indexes with different PKs indicate stale or inconsistent
// DOM data and must never trigger the old down/up recovery oscillation.
func feedSyncDirection(currentIndex, targetIndex int, currentPK, targetPK string) int {
	if currentPK != "" && currentPK == targetPK {
		return 0
	}
	if currentIndex == 0 || targetIndex == 0 || currentIndex == targetIndex {
		return 0
	}
	if currentIndex < targetIndex {
		return 1
	}
	return -1
}

func waitFeedSync(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// IsSyncing returns true if a SyncTo is in flight (its derived ctx not yet done).
func (fc *FeedCursor) IsSyncing() bool {
	fc.syncMu.Lock()
	defer fc.syncMu.Unlock()
	return fc.syncCtx != nil && fc.syncCtx.Err() == nil
}
