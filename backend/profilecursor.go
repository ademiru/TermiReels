package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
)

// ProfileCursor owns a creator's reel list independently from the main feed.
// Entries begin as shortcodes scraped from the profile grid; their PKs are
// filled when Instagram returns the reel payload after direct navigation.
type ProfileCursor struct {
	ctx      context.Context
	username string
	sourceID string
	resolve  func(context.Context, string) (string, error)

	mu     sync.RWMutex
	codes  []string
	pks    []string
	cursor int
	// authoritative contains only entries captured from Instagram's
	// target_user_id profile-Reels response, never DOM recommendations.
	authoritative []string
	candidates    []string

	syncMu     sync.Mutex
	syncCtx    context.Context
	syncCancel context.CancelFunc
	hydrating  atomic.Bool
}

func newResolvedProfileCursor(
	ctx context.Context,
	username, sourceID string,
	codes, pks []string,
) (*ProfileCursor, error) {
	if sourceID == "" || len(codes) == 0 || len(codes) != len(pks) {
		return nil, fmt.Errorf("invalid resolved creator cursor")
	}
	pc := &ProfileCursor{
		ctx:      ctx,
		username: username,
		sourceID: sourceID,
		codes:    append([]string(nil), codes...),
		pks:      append([]string(nil), pks...),
	}
	for i := range pc.codes {
		if pc.codes[i] == "" || pc.pks[i] == "" {
			return nil, fmt.Errorf("creator cursor item %d is incomplete", i+1)
		}
	}
	return pc, nil
}

func NewProfileCursor(ctx context.Context, username string, codes []string) *ProfileCursor {
	return newProfileCursor(ctx, username, codes, nil)
}

func newProfileCursor(
	ctx context.Context,
	username string,
	codes []string,
	resolve func(context.Context, string) (string, error),
) *ProfileCursor {
	return &ProfileCursor{
		ctx:      ctx,
		username: username,
		resolve:  resolve,
		codes:    append([]string(nil), codes...),
		pks:      make([]string, len(codes)),
	}
}

func (pc *ProfileCursor) Username() string         { return pc.username }
func (pc *ProfileCursor) Context() context.Context { return pc.ctx }
func (pc *ProfileCursor) SourceID() string         { return pc.sourceID }

func (pc *ProfileCursor) contains(code, pk string) bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	for i := range pc.codes {
		if pc.codes[i] == code && pc.pks[i] == pk {
			return true
		}
	}
	return false
}

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

func (pc *ProfileCursor) codeAt(index int) string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	if index < 1 || index > len(pc.codes) {
		return ""
	}
	return pc.codes[index-1]
}

// prime marks a reel already known from the main feed as resolved and makes
// it the profile cursor's current position. Entering a creator profile can
// therefore reuse the video that is already playing instead of reopening and
// redownloading the same reel before the UI responds.
func (pc *ProfileCursor) prime(code, pk string) int {
	if code == "" || pk == "" {
		return 0
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for i, existing := range pc.codes {
		if existing != code {
			continue
		}
		pc.pks[i] = pk
		pc.cursor = i
		return i + 1
	}
	// The source reel can be older than the part of the profile grid that is
	// currently materialized in the DOM. Keep it as the first profile entry
	// so opening the creator still responds immediately and never loses the
	// reel the user came from.
	pc.codes = append([]string{code}, pc.codes...)
	pc.pks = append([]string{pk}, pc.pks...)
	pc.cursor = 0
	return 1
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

func (pc *ProfileCursor) captureAuthoritative(code, pk string) bool {
	if code == "" {
		return false
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for i, existing := range pc.codes {
		if existing == code {
			if pk != "" {
				pc.pks[i] = pk
			}
			for _, trusted := range pc.authoritative {
				if trusted == code {
					return true
				}
			}
			pc.authoritative = append(pc.authoritative, code)
			return true
		}
	}
	pc.codes = append(pc.codes, code)
	pc.pks = append(pc.pks, pk)
	pc.authoritative = append(pc.authoritative, code)
	return true
}

func (pc *ProfileCursor) captureCandidate(code string) bool {
	if code == "" {
		return false
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for _, existing := range pc.candidates {
		if existing == code {
			return false
		}
	}
	pc.candidates = append(pc.candidates, code)
	return true
}

func (pc *ProfileCursor) candidateCodes() []string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return append([]string(nil), pc.candidates...)
}

func (pc *ProfileCursor) authoritativeCodes() []string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return append([]string(nil), pc.authoritative...)
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

// installGrid replaces the temporary source-only list with Instagram's actual
// profile-grid order while preserving every PK already resolved by prefetch.
func (pc *ProfileCursor) installGrid(codes []string) int {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	currentCode := ""
	if pc.cursor >= 0 && pc.cursor < len(pc.codes) {
		currentCode = pc.codes[pc.cursor]
	}
	resolved := make(map[string]string, len(pc.codes))
	for i, code := range pc.codes {
		if i < len(pc.pks) && pc.pks[i] != "" {
			resolved[code] = pc.pks[i]
		}
	}
	seen := make(map[string]struct{}, len(codes))
	nextCodes := make([]string, 0, len(codes))
	for _, code := range codes {
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		nextCodes = append(nextCodes, code)
	}
	nextPKs := make([]string, len(nextCodes))
	for i, code := range nextCodes {
		nextPKs[i] = resolved[code]
	}
	pc.codes = nextCodes
	pc.pks = nextPKs
	pc.cursor = 0
	// Hydration may insert newer reels before the one already on screen.
	// Follow its immutable shortcode instead of silently changing the visible
	// owner/content just because the numeric position moved.
	for i, code := range nextCodes {
		if code == currentCode {
			pc.cursor = i
			break
		}
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
	ctx, cancel := context.WithTimeout(pc.ctx, 12*time.Second)
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

	// Direct reel navigation usually triggers the fetch interceptor before
	// Navigate returns. When Instagram changes that request ordering, resolve
	// the shortcode explicitly instead of waiting on an event that may never
	// arrive.
	if pc.PKAt(index) != "" {
		return nil
	}
	if pc.resolve == nil {
		return fmt.Errorf("profile reel resolver unavailable")
	}
	pk, err := pc.resolve(ctx, pc.codeAt(index))
	if err != nil {
		return fmt.Errorf("resolve @%s reel %d: %w", pc.username, index, err)
	}
	if !pc.capture(pc.codeAt(index), pk) {
		return fmt.Errorf("resolved reel is not in @%s profile grid", pc.username)
	}
	return nil
}

func (pc *ProfileCursor) IsSyncing() bool {
	if pc.hydrating.Load() {
		return true
	}
	pc.syncMu.Lock()
	defer pc.syncMu.Unlock()
	return pc.syncCtx != nil && pc.syncCtx.Err() == nil
}

func (pc *ProfileCursor) setHydrating(value bool) {
	pc.hydrating.Store(value)
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
