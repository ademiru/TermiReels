package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

// CreatorProfileState is the terminal-facing profile header state.
type CreatorProfileState struct {
	Username   string
	Generation uint64
	Following  bool
	Known      bool
	Loading    bool
	Total      int
	Error      string
}

// ProfileBackend is an optional Backend extension so test and third-party
// backends do not break when creator browsing is unavailable.
type ProfileBackend interface {
	EnterCreatorProfile(username string) (*ReelInfo, error)
	ExitCreatorProfile() (*ReelInfo, error)
	IsProfileMode() bool
	CreatorProfile() CreatorProfileState
	ToggleCreatorFollow() (bool, error)
	PreloadCreatorReels(ctx context.Context, afterIndex, count int)
}

type CreatorFollowBackend interface {
	CreatorFollowState(username string) (following, known bool)
	RefreshCreatorFollow(username string)
	ToggleCreatorFollowFor(username string) (bool, error)
}

func (b *ChromeBackend) CreatorFollowState(username string) (bool, bool) {
	username = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	b.creatorFollowMu.RLock()
	defer b.creatorFollowMu.RUnlock()
	return b.creatorFollowing[username], b.creatorKnown[username]
}

func (b *ChromeBackend) setCreatorFollowState(username string, following bool) {
	username = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	b.creatorFollowMu.Lock()
	if b.creatorFollowing == nil {
		b.creatorFollowing = make(map[string]bool)
	}
	if b.creatorKnown == nil {
		b.creatorKnown = make(map[string]bool)
	}
	b.creatorFollowing[username] = following
	b.creatorKnown[username] = true
	b.creatorFollowMu.Unlock()
	select {
	case b.events <- Event{Type: EventCreatorFollowUpdated}:
	default:
	}
}

func (b *ChromeBackend) creatorFollowOperation(username string, toggle bool) (bool, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return false, fmt.Errorf("empty creator username")
	}
	b.profileOpMu.Lock()
	defer b.profileOpMu.Unlock()
	ctx, cancel := chromedp.NewContext(b.feedCtx)
	defer cancel()
	ctx, timeoutCancel := context.WithTimeout(ctx, 12*time.Second)
	defer timeoutCancel()

	var result string
	js := fmt.Sprintf(`(() => {
		const toggle = %t;
		const labels = [...document.querySelectorAll('button,[role="button"]')];
		const find = label => labels.find(b => (b.innerText || '').trim() === label);
		const following = find('Following');
		const requested = find('Requested');
		const follow = find('Follow');
		if (requested) return 'following';
		if (following) { if (toggle) following.click(); return toggle ? 'confirm_unfollow' : 'following'; }
		if (follow) { if (toggle) follow.click(); return toggle ? 'followed' : 'not_following'; }
		return 'unavailable';
	})()`, toggle)
	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.instagram.com/"+username+"/"),
		chromedp.WaitReady("body"),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(js, &result),
	); err != nil {
		return false, err
	}
	following := false
	switch result {
	case "following", "followed":
		following = true
	case "not_following":
	case "confirm_unfollow":
		var clicked bool
		if err := chromedp.Run(ctx,
			chromedp.Sleep(250*time.Millisecond),
			chromedp.Evaluate(`(() => {
				const b = [...document.querySelectorAll('button,[role="button"]')]
					.find(x => (x.innerText || '').trim() === 'Unfollow');
				if (!b) return false; b.click(); return true;
			})()`, &clicked),
		); err != nil || !clicked {
			return true, fmt.Errorf("unfollow confirmation unavailable")
		}
	default:
		return false, fmt.Errorf("follow control unavailable")
	}
	b.setCreatorFollowState(username, following)
	return following, nil
}

func (b *ChromeBackend) RefreshCreatorFollow(username string) {
	_, _ = b.creatorFollowOperation(username, false)
}

func (b *ChromeBackend) ToggleCreatorFollowFor(username string) (bool, error) {
	return b.creatorFollowOperation(username, true)
}

func (b *ChromeBackend) collectProfileCodes(profileCtx context.Context, username string) ([]string, CreatorProfileState, error) {
	ctx, cancel := context.WithTimeout(profileCtx, 8*time.Second)
	defer cancel()

	target := "https://www.instagram.com/" + username + "/reels/"
	type snapshot struct {
		Hrefs []string `json:"hrefs"`
		State string   `json:"state"`
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(target),
		chromedp.WaitReady("body"),
	); err != nil {
		return nil, CreatorProfileState{}, err
	}

	// Navigate/WaitReady is not enough on Instagram's SPA: body already
	// exists while React is still replacing the previous Reels feed. Never
	// scrape links until both the URL and profile shell identify the requested
	// creator for consecutive observations.
	expectedPath := "/" + strings.ToLower(username) + "/reels/"
	routeStable := 0
	for routeStable < 3 {
		var route struct {
			Path    string `json:"path"`
			Profile bool   `json:"profile"`
			HasMain bool   `json:"hasMain"`
		}
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
			const expected = `+fmt.Sprintf("%q", expectedPath)+`;
			const path = location.pathname.toLowerCase().replace(/\/+$/, '/') ;
			const main = document.querySelector('main');
			const profileTab = main && [...main.querySelectorAll('a[href]')].some(a => {
				try {
					return new URL(a.href, location.origin).pathname.toLowerCase().replace(/\/+$/, '/') === expected;
				} catch (_) {
					return false;
				}
			});
			return {
				path,
				hasMain: !!main,
				profile: !!profileTab
			};
		})()`, &route)); err != nil {
			return nil, CreatorProfileState{}, err
		}
		if route.Path == expectedPath && route.HasMain && route.Profile {
			routeStable++
		} else {
			routeStable = 0
		}
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, CreatorProfileState{}, fmt.Errorf("profile route did not settle for @%s: %w", username, ctx.Err())
		case <-timer.C:
		}
	}

	seen := make(map[string]struct{})
	var codes []string
	stateName := ""
	stableRounds := 0
	for attempt := 0; attempt < 32; attempt++ {
		var raw snapshot
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
			const expected = `+fmt.Sprintf("%q", expectedPath)+`;
			const path = location.pathname.toLowerCase().replace(/\/+$/, '/');
			if (path !== expected) return {hrefs: [], state: 'route_changed'};
			const root = document.querySelector('main') || document;
			const hrefs = [...root.querySelectorAll('a[href*="/reel/"]')].map(a => a.href);
			const labels = [...document.querySelectorAll('button,[role="button"]')].map(b => (b.innerText || '').trim());
			const state = labels.includes('Following') ? 'following'
				: labels.includes('Requested') ? 'requested'
				: labels.includes('Follow') ? 'not_following' : '';
			return {hrefs, state};
		})()`, &raw)); err != nil {
			return nil, CreatorProfileState{}, err
		}
		if raw.State == "route_changed" {
			return nil, CreatorProfileState{}, fmt.Errorf("profile route changed while collecting @%s", username)
		}
		if raw.State != "" {
			stateName = raw.State
		}
		before := len(codes)
		for _, href := range raw.Hrefs {
			code := profileCodeFromHref(href)
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
		if len(codes) == before {
			stableRounds++
		} else {
			stableRounds = 0
		}
		if len(codes) > 0 && stableRounds >= 4 {
			break
		}
		_ = chromedp.Run(ctx, chromedp.Evaluate(
			`window.scrollBy(0, Math.max(window.innerHeight * 1.5, 600))`, nil))
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, CreatorProfileState{}, ctx.Err()
		case <-timer.C:
		}
	}
	state := CreatorProfileState{Username: username, Known: stateName != "", Total: len(codes)}
	state.Following = stateName == "following" || stateName == "requested"
	return codes, state, nil
}

func (b *ChromeBackend) resolveProfileReel(ctx context.Context, username, code string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if code == "" {
		return "", fmt.Errorf("empty reel shortcode")
	}
	template := b.dm.Template()
	if template == "" {
		return "", fmt.Errorf("reel request template is not ready")
	}
	vars := map[string]interface{}{
		"after":  nil,
		"before": nil,
		"first":  1,
		"last":   nil,
		"data": map[string]interface{}{
			"container_module":              "clips_tab_desktop_page",
			"seen_reels":                    "[]",
			"chaining_media_id":             code,
			"should_refetch_chaining_media": true,
		},
		"__relay_internal__pv__PolarisReelsRecoDebugOverlayEnabledrelayprovider": false,
		"__relay_internal__pv__PolarisAIGMMediaWebLabelEnabledrelayprovider":     false,
	}
	b.profileResolveMu.Lock()
	defer b.profileResolveMu.Unlock()

	resolveCtx := b.dmCtx
	if resolveCtx == nil {
		resolveCtx = ctx
	}
	req, err := newGraphQLRequest(resolveCtx, template, clipsDocID, clipsFriendlyName, readEndpoint, vars)
	if err != nil {
		return "", err
	}
	result, err := execGraphQL(req)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var resp reelResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return "", err
	}
	media, ok := resp.mediaByCode(code)
	if !ok {
		return "", fmt.Errorf("Instagram response did not contain requested reel %s", code)
	}
	if username != "" && !strings.EqualFold(media.User.Username, username) {
		return "", fmt.Errorf("reel %s belongs to @%s, not @%s", code, media.User.Username, username)
	}
	b.reelsMu.Lock()
	if _, exists := b.reels[media.PK]; !exists {
		b.reels[media.PK] = buildReel(media)
	}
	b.reelsMu.Unlock()
	return media.PK, nil
}

// PreloadCreatorReels resolves and downloads upcoming profile entries without
// navigating the visible feed browser. GraphQL work uses the secondary DM
// context and a serialized resolver queue, so it cannot contend with scroll
// navigation or mix responses into the main feed.
func (b *ChromeBackend) PreloadCreatorReels(ctx context.Context, afterIndex, count int) {
	b.modeMu.RLock()
	pc := b.profile
	b.modeMu.RUnlock()
	if pc == nil || count < 1 {
		return
	}
	end := min(afterIndex+count, pc.Total())
	for index := afterIndex + 1; index <= end; index++ {
		if ctx.Err() != nil {
			return
		}
		if pc.PKAt(index) == "" {
			pk, err := b.resolveProfileReel(ctx, pc.Username(), pc.codeAt(index))
			if err != nil || !pc.capture(pc.codeAt(index), pk) {
				continue
			}
		}
		if ctx.Err() != nil {
			return
		}
		b.modeMu.RLock()
		stillActive := b.profile == pc
		b.modeMu.RUnlock()
		if !stillActive {
			return
		}
		_, _, _, _ = b.downloadPKContext(ctx, index, pc.PKAt(index))
	}
}

func (b *ChromeBackend) discoverNextProfileReel(pc *ProfileCursor, afterIndex int) (*ReelInfo, error) {
	// Hydration may have completed after the TUI observed the old total and
	// requested discovery. Consume that newly available entry directly
	// instead of navigating back to the profile grid a second time.
	if afterIndex < pc.Total() {
		next := afterIndex + 1
		if pc.PKAt(next) == "" {
			pk, err := b.resolveProfileReel(pc.Context(), pc.Username(), pc.codeAt(next))
			if err != nil {
				return nil, err
			}
			pc.capture(pc.codeAt(next), pk)
		}
		if err := pc.SyncTo(next); err != nil {
			return nil, err
		}
		return b.GetCurrent()
	}
	if pc.SourceID() != "" {
		return nil, fmt.Errorf("reached the end of the verified @%s reel page", pc.Username())
	}

	_, state, err := b.collectProfileCodes(pc.Context(), pc.Username())
	if err != nil {
		log.Printf("creator profile hydration failed: %v", err)
		_ = pc.SyncTo(afterIndex)
		return nil, err
	}
	trustedCodes := pc.authoritativeCodes()
	if len(trustedCodes) == 0 {
		_ = pc.SyncTo(afterIndex)
		return nil, fmt.Errorf("profile reels could not be verified for @%s", pc.Username())
	}
	total := pc.installGrid(trustedCodes)
	state.Total = total
	state.Loading = false
	b.modeMu.Lock()
	state.Generation = b.profileState.Generation
	b.profileState = state
	b.modeMu.Unlock()
	if afterIndex >= total {
		_ = pc.SyncTo(afterIndex)
		return nil, fmt.Errorf("no more visible reels for @%s", pc.Username())
	}
	if err := pc.SyncTo(afterIndex + 1); err != nil {
		return nil, err
	}
	return b.GetCurrent()
}

func (b *ChromeBackend) hydrateCreatorProfile(pc *ProfileCursor) {
	b.profileOpMu.Lock()
	pc.setHydrating(true)
	defer func() {
		pc.setHydrating(false)
		b.profileOpMu.Unlock()
	}()

	codes, state, err := b.collectProfileCodes(pc.Context(), pc.Username())
	if err != nil {
		// The source reel remains a complete one-item profile feed. A grid
		// refresh can be retried when the user reaches its tail.
		b.modeMu.Lock()
		if b.profile == pc {
			b.profileState.Loading = false
			b.profileState.Error = "REELS UNAVAILABLE"
		}
		b.modeMu.Unlock()
		return
	}
	b.modeMu.RLock()
	stillActive := b.profile == pc
	b.modeMu.RUnlock()
	if !stillActive {
		return
	}
	// DOM codes are deliberately not trusted: the profile page can place
	// recommendations in the same <main>. Only the target_user_id response
	// intercepted while collecting may define profile membership.
	_ = codes
	trustedCodes := pc.authoritativeCodes()
	candidateCodes := pc.candidateCodes()
	for attempts := 0; len(trustedCodes) == 0 && len(candidateCodes) == 0 && attempts < 10; attempts++ {
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-pc.Context().Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		trustedCodes = pc.authoritativeCodes()
		candidateCodes = pc.candidateCodes()
	}
	// Profile grid summaries omit user.username. Resolve only until the first
	// verified reel so the profile opens promptly; the remaining candidates
	// are verified in order in the background.
	for _, code := range candidateCodes {
		pk, resolveErr := b.resolveProfileReel(pc.Context(), pc.Username(), code)
		if resolveErr != nil {
			log.Printf(
				"creator profile candidate rejected: target=@%s code=%s error=%v",
				pc.Username(), code, resolveErr,
			)
			continue
		}
		pc.captureAuthoritative(code, pk)
		log.Printf(
			"creator profile candidate verified: target=@%s code=%s",
			pc.Username(), code,
		)
		break
	}
	trustedCodes = pc.authoritativeCodes()
	if len(trustedCodes) == 0 {
		b.modeMu.Lock()
		if b.profile == pc {
			b.profileState.Loading = false
			b.profileState.Error = "PROFILE REELS NOT VERIFIED"
		}
		b.modeMu.Unlock()
		log.Printf("creator profile rejected: no authoritative target_user_id reels for @%s", pc.Username())
		return
	}
	pc.installGrid(trustedCodes)
	state.Loading = false
	state.Total = pc.Total()
	log.Printf("creator profile hydrated: %d reels", state.Total)
	b.modeMu.Lock()
	state.Generation = b.profileState.Generation
	b.profileState = state
	b.modeMu.Unlock()

	// collectProfileCodes leaves Chromium on the grid. Put it back on the
	// source reel for DOM mutations while the local player keeps rendering.
	if err := pc.SyncTo(1); err != nil {
		log.Printf("creator profile first reel failed: %v", err)
		return
	}
	b.modeMu.RLock()
	stillActive = b.profile == pc
	b.modeMu.RUnlock()
	if stillActive {
		select {
		case b.events <- Event{
			Type: EventProfileReady, Count: pc.Total(), Generation: state.Generation,
		}:
		case <-pc.Context().Done():
		}
	}
	go b.verifyRemainingProfileCandidates(pc, candidateCodes)
	go b.PreloadCreatorReels(context.Background(), 1, 3)
}

func (b *ChromeBackend) verifyRemainingProfileCandidates(pc *ProfileCursor, candidates []string) {
	for _, code := range candidates {
		if pc.Context().Err() != nil {
			return
		}
		alreadyVerified := false
		for _, trusted := range pc.authoritativeCodes() {
			if trusted == code {
				alreadyVerified = true
				break
			}
		}
		if alreadyVerified {
			continue
		}
		pk, err := b.resolveProfileReel(pc.Context(), pc.Username(), code)
		if err != nil {
			log.Printf(
				"creator profile background candidate rejected: target=@%s code=%s error=%v",
				pc.Username(), code, err,
			)
			continue
		}
		if !pc.captureAuthoritative(code, pk) {
			continue
		}
		b.modeMu.Lock()
		if b.profile == pc {
			b.profileState.Total = pc.Total()
		}
		b.modeMu.Unlock()
		log.Printf(
			"creator profile background candidate verified: target=@%s code=%s",
			pc.Username(), code,
		)
	}
}

func (b *ChromeBackend) EnterCreatorProfile(username string) (*ReelInfo, error) {
	b.profileOpMu.Lock()
	defer b.profileOpMu.Unlock()

	username = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if username == "" {
		return nil, fmt.Errorf("empty profile username")
	}
	b.creatorProviderMu.Lock()
	creatorEnabled := b.creatorEnabled
	b.creatorProviderMu.Unlock()
	if !creatorEnabled {
		return nil, fmt.Errorf("creator reel browsing is not enabled")
	}
	log.Printf("creator provider feed requested: @%s", username)
	if b.IsChatMode() {
		return nil, fmt.Errorf("exit chat mode before opening a profile")
	}

	_, returnPK, err := b.feed.Current()
	if err != nil || returnPK == "" {
		return nil, fmt.Errorf("current feed position is unavailable")
	}
	if reel, ok := b.reelByPK(returnPK); !ok || !strings.EqualFold(reel.Username, username) {
		return nil, fmt.Errorf("current reel does not belong to @%s", username)
	}

	resolveCtx, resolveCancel := context.WithTimeout(context.Background(), 15*time.Second)
	b.modeMu.Lock()
	b.profileResolveCancel = resolveCancel
	b.modeMu.Unlock()
	// Revision 1 installs one compact verified page. Asking for the 12 entries
	// the UI can immediately browse avoids waiting for speculative extra grid
	// rows while retaining the same dual-evidence checks for every item.
	snapshot, err := b.resolveCreatorSnapshot(resolveCtx, username, 12)
	resolveCancel()
	b.modeMu.Lock()
	b.profileResolveCancel = nil
	b.modeMu.Unlock()
	if err != nil {
		log.Printf("creator provider feed rejected: @%s: %v", username, err)
		return nil, fmt.Errorf("resolve @%s reels: %w", username, err)
	}
	// Resolution is intentionally non-mutating. Reject a delayed result if
	// the main cursor changed while the sidecar was working.
	_, currentFeedPK, currentErr := b.feed.Current()
	if currentErr != nil || currentFeedPK != returnPK {
		return nil, fmt.Errorf("main feed changed while resolving @%s", username)
	}

	profileCtx, profileCancel := chromedp.NewContext(b.feedCtx)
	codes := make([]string, len(snapshot.Items))
	pks := make([]string, len(snapshot.Items))
	for i, item := range snapshot.Items {
		codes[i] = item.Shortcode
		pks[i] = item.PK
	}
	pc, err := newResolvedProfileCursor(profileCtx, username, snapshot.SourceID, codes, pks)
	if err != nil {
		profileCancel()
		return nil, err
	}
	firstURL := "https://www.instagram.com/reel/" + snapshot.Items[0].Shortcode + "/"
	if err := chromedp.Run(profileCtx,
		chromedp.Navigate(firstURL),
		chromedp.WaitReady("body"),
	); err != nil {
		profileCancel()
		return nil, fmt.Errorf("open verified creator reel: %w", err)
	}
	chromedp.ListenTarget(profileCtx, func(ev interface{}) {
		if e, ok := ev.(*fetch.EventRequestPaused); ok {
			go b.processFeedGraphQLBody(profileCtx, e)
		}
	})
	if err := chromedp.Run(profileCtx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
		{URLPattern: "*graphql*", RequestStage: fetch.RequestStageResponse},
		{URLPattern: "*api/v1/clips/user*", RequestStage: fetch.RequestStageResponse},
	})); err != nil {
		profileCancel()
		return nil, fmt.Errorf("start creator browser: %w", err)
	}

	b.reelsMu.Lock()
	for _, item := range snapshot.Items {
		b.reels[item.PK] = creatorProviderReel(item, username)
	}
	b.reelsMu.Unlock()
	following, known := b.CreatorFollowState(username)

	b.modeMu.Lock()
	generation := b.profileState.Generation + 1
	b.profile = pc
	b.profileCtx = profileCtx
	b.profileCancel = profileCancel
	b.profileState = CreatorProfileState{
		Username: username, Generation: generation, Following: following,
		Known: known, Loading: false, Total: len(snapshot.Items),
	}
	b.active = pc
	b.ctx = profileCtx
	b.modeMu.Unlock()

	info, err := b.GetCurrent()
	if err != nil {
		b.restoreFeedMode()
		return nil, err
	}
	log.Printf(
		"creator provider feed activated: @%s source=%s items=%d",
		username, snapshot.SourceID, len(snapshot.Items),
	)
	go b.PreloadCreatorReels(context.Background(), 1, 3)
	return info, nil
}

func creatorProviderReel(item CreatorProviderItem, profileUsername string) *Reel {
	username := strings.ToLower(item.OwnerUsername)
	if username == "" {
		username = profileUsername
	}
	var music *MusicInfo
	if item.MusicTitle != "" || item.MusicArtist != "" {
		music = &MusicInfo{
			Title: item.MusicTitle, Artist: item.MusicArtist, IsExplicit: item.MusicExplicit,
		}
	}
	return &Reel{
		PK: item.PK, Code: item.Shortcode, VideoURL: item.VideoURL,
		ProfilePicUrl: item.ProfilePicURL, Username: username, Caption: item.Caption,
		Liked: item.Liked, Saved: item.Saved, Reposted: item.Reposted,
		LikeCount: item.LikeCount, CommentCount: item.CommentCount, RepostCount: item.RepostCount,
		CommentsDisabled: item.CommentsDisabled, IsVerified: item.Verified,
		CanViewerReshare: item.CanViewerReshare, Music: music,
	}
}

func (b *ChromeBackend) restoreFeedMode() {
	b.modeMu.Lock()
	cancel := b.profileCancel
	b.profile = nil
	b.profileCtx = nil
	b.profileCancel = nil
	b.active = b.feed
	b.ctx = b.feedCtx
	b.modeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (b *ChromeBackend) ExitCreatorProfile() (*ReelInfo, error) {
	// Interrupt grid hydration/navigation before waiting for its operation
	// lock. Otherwise Back can wait for the full browser timeout.
	b.modeMu.RLock()
	cancel := b.profileCancel
	resolveCancel := b.profileResolveCancel
	b.modeMu.RUnlock()
	if cancel != nil {
		cancel()
	}
	if resolveCancel != nil {
		resolveCancel()
	}

	b.profileOpMu.Lock()
	defer b.profileOpMu.Unlock()

	b.modeMu.RLock()
	active := b.profile != nil
	b.modeMu.RUnlock()
	if !active {
		return b.GetCurrent()
	}
	username := b.CreatorProfile().Username
	b.restoreFeedMode()
	// The main feed lives in its own untouched tab, so returning requires no
	// browser navigation or scroll reconstruction.
	log.Printf("creator provider feed exited: @%s; main feed restored", username)
	return b.GetCurrent()
}

func (b *ChromeBackend) IsProfileMode() bool {
	b.modeMu.RLock()
	defer b.modeMu.RUnlock()
	return b.profile != nil
}

func (b *ChromeBackend) CreatorProfile() CreatorProfileState {
	b.modeMu.RLock()
	defer b.modeMu.RUnlock()
	return b.profileState
}

func (b *ChromeBackend) ToggleCreatorFollow() (bool, error) {
	b.profileOpMu.Lock()
	defer b.profileOpMu.Unlock()

	b.modeMu.RLock()
	pc := b.profile
	state := b.profileState
	b.modeMu.RUnlock()
	if pc == nil {
		return false, fmt.Errorf("not browsing a creator profile")
	}
	index, _, err := pc.Current()
	if err != nil {
		return state.Following, err
	}
	returnURL := pc.targetURL(index)
	profileURL := "https://www.instagram.com/" + pc.Username() + "/"

	var result string
	js := `(() => {
		const buttons = [...document.querySelectorAll('button,[role="button"]')];
		const find = label => buttons.find(b => (b.innerText || '').trim() === label);
		const following = find('Following');
		const requested = find('Requested');
		const follow = find('Follow');
		if (requested) return 'requested';
		if (following) { following.click(); return 'unfollow_dialog'; }
		if (follow) { follow.click(); return 'followed'; }
		return 'unavailable';
	})()`
	if err := chromedp.Run(pc.Context(),
		chromedp.Navigate(profileURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(700*time.Millisecond),
		chromedp.Evaluate(js, &result),
	); err != nil {
		return state.Following, err
	}

	following := state.Following
	switch result {
	case "followed":
		following = true
	case "unfollow_dialog":
		var clicked bool
		if err := chromedp.Run(pc.Context(),
			chromedp.Sleep(300*time.Millisecond),
			chromedp.Evaluate(`(() => {
				const b = [...document.querySelectorAll('button,[role="button"]')]
					.find(x => (x.innerText || '').trim() === 'Unfollow');
				if (!b) return false;
				b.click();
				return true;
			})()`, &clicked),
		); err != nil || !clicked {
			_ = chromedp.Run(pc.Context(), chromedp.Navigate(returnURL))
			if err != nil {
				return state.Following, err
			}
			return state.Following, fmt.Errorf("unfollow confirmation unavailable")
		}
		following = false
	case "requested":
		_ = chromedp.Run(pc.Context(), chromedp.Navigate(returnURL))
		return true, fmt.Errorf("follow request is pending")
	default:
		_ = chromedp.Run(pc.Context(), chromedp.Navigate(returnURL))
		return state.Following, fmt.Errorf("follow control unavailable")
	}

	b.modeMu.Lock()
	b.profileState.Following = following
	b.profileState.Known = true
	b.modeMu.Unlock()
	if err := chromedp.Run(pc.Context(), chromedp.Navigate(returnURL)); err != nil {
		return following, err
	}
	return following, nil
}
