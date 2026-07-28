package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// ProfileCursor owns a creator's reel list independently from the main feed.
// Entries begin as shortcodes scraped from the profile grid; their PKs are
// filled when Instagram returns the reel payload after direct navigation.
type ProfileCursor struct {
	ctx      context.Context
	username string

	mu     sync.RWMutex
	codes  []string
	pks    []string
	cursor int

	syncMu     sync.Mutex
	syncCtx    context.Context
	syncCancel context.CancelFunc
}

func NewProfileCursor(ctx context.Context, username string, codes []string) *ProfileCursor {
	return &ProfileCursor{
		ctx:      ctx,
		username: username,
		codes:    append([]string(nil), codes...),
		pks:      make([]string, len(codes)),
	}
}

func (pc *ProfileCursor) Username() string { return pc.username }

func (pc *ProfileCursor) Total() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return len(pc.codes)
}

func (pc *ProfileCursor) PKAt(index int) string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	if index < 1 || index > len(pc.pks) {
		return ""
	}
	return pc.pks[index-1]
}

func (pc *ProfileCursor) Current() (int, string, error) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	if pc.cursor < 0 || pc.cursor >= len(pc.codes) || pc.pks[pc.cursor] == "" {
		return 0, "", fmt.Errorf("profile reel is not resolved")
	}
	return pc.cursor + 1, pc.pks[pc.cursor], nil
}

func (pc *ProfileCursor) targetURL(index int) string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	if index < 1 || index > len(pc.codes) {
		return ""
	}
	return "https://www.instagram.com/reel/" + pc.codes[index-1] + "/"
}

// capture associates an intercepted reel payload with its profile-grid entry.
// It never appends to the main feed.
func (pc *ProfileCursor) capture(code, pk string) bool {
	if code == "" || pk == "" {
		return false
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for i, existing := range pc.codes {
		if existing == code {
			pc.pks[i] = pk
			return true
		}
	}
	return false
}

func (pc *ProfileCursor) appendCodes(codes []string) int {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	seen := make(map[string]struct{}, len(pc.codes))
	for _, code := range pc.codes {
		seen[code] = struct{}{}
	}
	for _, code := range codes {
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		pc.codes = append(pc.codes, code)
		pc.pks = append(pc.pks, "")
	}
	return len(pc.codes)
}

func (pc *ProfileCursor) SyncTo(index int) error {
	target := pc.targetURL(index)
	if target == "" {
		return fmt.Errorf("profile index %d out of range", index)
	}

	pc.syncMu.Lock()
	if pc.syncCancel != nil {
		pc.syncCancel()
	}
	ctx, cancel := context.WithCancel(pc.ctx)
	pc.syncCtx = ctx
	pc.syncCancel = cancel
	pc.syncMu.Unlock()
	defer cancel()

	pc.mu.Lock()
	pc.cursor = index - 1
	alreadyResolved := pc.pks[index-1] != ""
	pc.mu.Unlock()

	if err := chromedp.Run(ctx, chromedp.Navigate(target)); err != nil {
		return err
	}
	if alreadyResolved {
		return nil
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pc.PKAt(index) != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out loading @%s reel %d", pc.username, index)
}

func (pc *ProfileCursor) IsSyncing() bool {
	pc.syncMu.Lock()
	defer pc.syncMu.Unlock()
	return pc.syncCtx != nil && pc.syncCtx.Err() == nil
}

func profileCodeFromHref(href string) string {
	const marker = "/reel/"
	i := strings.Index(href, marker)
	if i < 0 {
		return ""
	}
	rest := href[i+len(marker):]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		rest = rest[:slash]
	}
	return strings.TrimSpace(rest)
}
