package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// NewChromeBackend creates a new Chrome-based backend
func NewChromeBackend(userDataDir, cacheDir, configDir string) *ChromeBackend {
	b := ChromeBackend{
		reels:       make(map[string]*Reel),
		comments:    &CommentsState{},
		dm:          &dmState{},
		events:      make(chan Event, 100),
		userDataDir: userDataDir,
		cacheDir:    cacheDir,
		configDir:   configDir,
	}

	b.initStorage()

	return &b
}

// Start initializes Chrome and navigates to Instagram homepage
func (b *ChromeBackend) Start(headless bool) error {
	// Create user data directory for persistent sessions
	err := os.MkdirAll(b.userDataDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create user data dir: %w", err)
	}

	// Find or download Chrome
	execPath, err := EnsureChromium(b.userDataDir)
	if err != nil {
		return fmt.Errorf("chrome not found: %w", err)
	}

	// Chrome options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.UserDataDir(b.userDataDir),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("remote-debugging-port", "6767"),
		chromedp.Flag("remote-allow-origins", "*"),
	)
	if headless {
		opts = append(opts, chromedp.Flag("headless", "new"))
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	b.allocCancel = allocCancel

	feedCtx, feedCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	b.feedCtx = feedCtx
	b.feedCancel = feedCancel
	b.ctx = feedCtx
	b.feed = NewFeedCursor(feedCtx)
	b.active = b.feed

	chromedp.ListenTarget(feedCtx, func(ev interface{}) {
		if e, ok := ev.(*fetch.EventRequestPaused); ok {
			go b.processFeedGraphQLBody(feedCtx, e)
		}
	})

	// Enable fetch interception and navigate
	err = chromedp.Run(feedCtx,
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{
			{
				URLPattern:   "*graphql*",
				RequestStage: fetch.RequestStageResponse,
			},
		}),
		chromedp.Navigate("https://www.instagram.com/"),
		chromedp.Sleep(2*time.Second), // sleep to let page load
	)
	if err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	return nil
}

// NeedsLogin checks if login is required by looking for login form elements
func (b *ChromeBackend) NeedsLogin() (bool, error) {
	var needsLogin bool
	err := chromedp.Run(b.ctx,
		chromedp.Evaluate(`
			document.querySelector('input[name="username"]') !== null ||
			document.querySelector('input[name="email"]') !== null ||
			document.querySelector('input[aria-label="Phone number, username, or email"]') !== null
		`, &needsLogin),
	)
	return needsLogin, err
}

// NavigateToReels goes to /reels and syncs to first captured reel
func (b *ChromeBackend) NavigateToReels() error {
	if err := chromedp.Run(b.feedCtx,
		chromedp.Navigate("https://www.instagram.com/reels/"),
		chromedp.Sleep(2*time.Second),
	); err != nil {
		return fmt.Errorf("failed to navigate to reels: %w", err)
	}

	// initial sync
	for i := 0; i < MaxRetries; i++ {
		info, err := b.GetCurrent()
		if err == nil && info != nil {
			b.events <- Event{Type: EventSyncComplete}
			if err := b.startDMSession(); err != nil {
				log.Printf("dm session: %v", err)
			}
			return nil
		}
		if err := b.feed.scrollDown(); err != nil {
			return err
		}
		time.Sleep(time.Duration(1500+rand.Intn(500)) * time.Millisecond)
	}
	return fmt.Errorf("could not complete initial sync")
}

// Stop closes the browser
func (b *ChromeBackend) Stop() {
	b.stopDMSession()
	if b.feedCancel != nil {
		b.feedCancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
	close(b.events)
}

// Events returns the event channel
func (b *ChromeBackend) Events() <-chan Event {
	return b.events
}

// activeCursor returns whichever cursor user actions should route through.
// Today this is always b.feed; modeMu protects future swaps when DM mode lands.
func (b *ChromeBackend) activeCursor() Cursor {
	b.modeMu.RLock()
	defer b.modeMu.RUnlock()
	return b.active
}

// mutateReelByPK applies fn to the reel with the given PK if present.
// Returns true if a reel was mutated.
func (b *ChromeBackend) mutateReelByPK(pk string, fn func(*Reel)) bool {
	b.reelsMu.Lock()
	defer b.reelsMu.Unlock()
	r, ok := b.reels[pk]
	if !ok {
		return false
	}
	fn(r)
	return true
}

// reelByPK returns a copy of the reel with the given PK, or false if absent.
func (b *ChromeBackend) reelByPK(pk string) (Reel, bool) {
	b.reelsMu.RLock()
	defer b.reelsMu.RUnlock()
	r, ok := b.reels[pk]
	if !ok {
		return Reel{}, false
	}
	return *r, true
}

// GetCurrent returns info about the currently visible reel
func (b *ChromeBackend) GetCurrent() (*ReelInfo, error) {
	cur := b.activeCursor()
	idx, pk, err := cur.Current()
	if err != nil {
		return nil, err
	}
	reel, ok := b.reelByPK(pk)
	if !ok {
		return nil, fmt.Errorf("reel pk=%s not in cache", pk)
	}
	return &ReelInfo{Index: idx, Total: cur.Total(), Reel: reel}, nil
}

// GetReel returns reel info by *1-BASED INDEX* from cache, no browser interaction
func (b *ChromeBackend) GetReel(index int) (*ReelInfo, error) {
	cur := b.activeCursor()
	total := cur.Total()
	if index < 1 || index > total {
		return nil, fmt.Errorf("index %d out of range (1-%d)", index, total)
	}
	pk := cur.PKAt(index)
	if pk == "" {
		return nil, fmt.Errorf("no pk at index %d", index)
	}
	reel, ok := b.reelByPK(pk)
	if !ok {
		return nil, fmt.Errorf("reel pk=%s not in cache", pk)
	}
	return &ReelInfo{Index: index, Total: total, Reel: reel}, nil
}

// updateReelComments appends comments to a reel by PK, or sets them if none exist yet.
func (b *ChromeBackend) updateReelComments(pk string, comments []Comment) {
	b.mutateReelByPK(pk, func(r *Reel) {
		if r.Comments != nil {
			r.Comments = append(r.Comments, comments...)
		} else {
			r.Comments = comments
		}
	})
}

// insertChildComments splices a parent comment's replies into the reel's comment
// list immediately after the parent.
func (b *ChromeBackend) insertChildComments(reelPK, parentPK string, children []Comment) {
	b.mutateReelByPK(reelPK, func(r *Reel) {
		idx := slices.IndexFunc(r.Comments, func(c Comment) bool { return c.PK == parentPK })
		if idx == -1 {
			return
		}
		r.Comments = slices.Insert(r.Comments, idx+1, children...)
	})
}

// CollapseChildComments removes the loaded replies of the given parent comment
// from the open reel's comment list, then emits EventCommentsCaptured.
func (b *ChromeBackend) CollapseChildComments(parentPK string) {
	reelPK := b.comments.GetReelPK()
	if reelPK == "" || parentPK == "" {
		return
	}
	b.mutateReelByPK(reelPK, func(r *Reel) {
		r.Comments = slices.DeleteFunc(r.Comments, func(c Comment) bool {
			return c.ParentCommentID == parentPK
		})
	})
	b.events <- Event{Type: EventCommentsCaptured}
}

// GetTotal returns total number of captured reels
func (b *ChromeBackend) GetTotal() int {
	return b.activeCursor().Total()
}

// DiscoverNextReel moves the main feed beyond the last reel captured so far
// and waits for the GraphQL response that contains the newly loaded reel.
//
// The number currently in FeedCursor is only a cache boundary, not the end of
// Instagram's infinite feed. Treating it as a hard upper bound made navigation
// appear to stop permanently after enough scrolling.
func (b *ChromeBackend) DiscoverNextReel(afterIndex int) (*ReelInfo, error) {
	if b.IsChatMode() {
		return nil, fmt.Errorf("cannot extend the feed while browsing a chat")
	}
	b.modeMu.RLock()
	profile := b.profile
	b.modeMu.RUnlock()
	if profile != nil {
		b.profileOpMu.Lock()
		defer b.profileOpMu.Unlock()
		return b.discoverNextProfileReel(profile, afterIndex)
	}
	if afterIndex < 1 {
		return nil, fmt.Errorf("invalid feed index %d", afterIndex)
	}

	// A response may already have arrived between the key press and this
	// command starting.
	if info, err := b.GetReel(afterIndex + 1); err == nil {
		return info, nil
	}

	if err := b.feed.scrollDown(); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := b.GetReel(afterIndex + 1); err == nil {
			return info, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for the next reel")
}

// SyncTo navigates the active cursor to the given index. Comments are cleared
// up-front because arrow-key scrolls don't trigger Instagram's auto-close.
func (b *ChromeBackend) SyncTo(index int) error {
	b.ClearComments()
	cur := b.activeCursor()
	if _, ok := cur.(*ProfileCursor); ok {
		b.profileOpMu.Lock()
		defer b.profileOpMu.Unlock()
	}
	return cur.SyncTo(index)
}

// IsSyncing returns true if the active cursor is mid-navigation.
func (b *ChromeBackend) IsSyncing() bool {
	return b.activeCursor().IsSyncing()
}

func (b *ChromeBackend) ReactToCurrent(emoji string) error {
	cc := b.activeCursor()
	if dm, ok := cc.(*ChatCursor); ok {
		return dm.ReactToCurrent(emoji)
	}
	return fmt.Errorf("Not in chat mode")
}

// ToggleLike clicks the like button for the current reel
func (b *ChromeBackend) ToggleLike() (bool, error) {
	if b.IsSyncing() {
		return false, fmt.Errorf("Still syncing to reel")
	}

	_, pk, err := b.activeCursor().Current()
	if err != nil {
		return false, err
	}

	// Clear any previous marker, then find the like button for the visible video
	js := `
		(() => {
			// Clear old markers first
			document.querySelectorAll('[data-reels-like-btn]').forEach(el => {
				el.removeAttribute('data-reels-like-btn');
			});

			const videos = document.querySelectorAll('video[playsinline]');
			for (const video of videos) {
				const rect = video.getBoundingClientRect();
				const viewportHeight = window.innerHeight;
				const videoCenter = rect.top + rect.height / 2;
				if (videoCenter > 0 && videoCenter < viewportHeight) {
					let parent = video.parentElement;
					for (let i = 0; i < 15; i++) {
						if (!parent) break;
						const svg = parent.querySelector('svg[aria-label="Like"], svg[aria-label="Unlike"]');
						if (svg) {
							const btn = svg.closest('[role="button"]');
							if (btn) {
								btn.setAttribute('data-reels-like-btn', 'true');
								return true;
							}
						}
						parent = parent.parentElement;
					}
				}
			}
			return false;
		})()
	`
	var found bool
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &found)); err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	if err := chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-like-btn="true"]`, chromedp.ByQuery),
	); err != nil {
		return false, err
	}

	b.mutateReelByPK(pk, func(r *Reel) { r.Liked = !r.Liked })
	return true, nil
}

// SetCommentLiked likes or unlikes one comment by its Instagram PK.
//
// Matching a visible DOM node by username and the first few words can click
// the wrong comment when an author posts twice, and breaks whenever Instagram
// rearranges its comment markup. The web endpoint accepts the stable PK that
// the comments GraphQL response already gives us.
func (b *ChromeBackend) SetCommentLiked(comment Comment, liked bool) error {
	if b.IsSyncing() {
		return fmt.Errorf("still syncing to reel")
	}
	if comment.PK == "" {
		return fmt.Errorf("missing comment PK")
	}

	action := "unlike"
	if liked {
		action = "like"
	}
	pkJSON, _ := json.Marshal(comment.PK)
	actionJSON, _ := json.Marshal(action)
	usernameJSON, _ := json.Marshal(comment.Username)
	textJSON, _ := json.Marshal(strings.Join(strings.Fields(comment.Text), " "))

	js := fmt.Sprintf(`
		(async () => {
			const pk = %s;
			const action = %s;
			const username = %s;
			const wantedText = %s;
			const csrf = document.cookie.split('; ')
				.find(c => c.startsWith('csrftoken='))
				?.split('=').slice(1).join('=') || '';

			const endpoint = '/api/v1/web/comments/' +
				encodeURIComponent(pk) + '/' + action + '/';
			let lastStatus = 0;
			for (let attempt = 0; attempt < 2; attempt++) {
				try {
					const response = await fetch(endpoint, {
						method: 'POST',
						credentials: 'include',
						headers: {
							'content-type': 'application/x-www-form-urlencoded',
							'x-csrftoken': csrf,
							'x-ig-app-id': '%s',
							'x-asbd-id': '129477',
							'x-requested-with': 'XMLHttpRequest'
						},
						body: ''
					});
					lastStatus = response.status;
					if (response.ok) return 'api';
				} catch (_) {
					lastStatus = 0;
				}
				await new Promise(resolve => setTimeout(resolve, 250));
			}

			// Instagram occasionally rejects the web endpoint while its own
			// visible button remains functional. Match the open comment by
			// exact author + normalized text and only click the state needed
			// for this operation. This works for loaded child replies too.
			const normalize = s => (s || '').replace(/\s+/g, ' ').trim();
			const aria = action === 'like' ? 'Like' : 'Unlike';
			const links = document.querySelectorAll(
				'a[href="/' + CSS.escape(username) + '/"],' +
				'a[href^="/' + CSS.escape(username) + '/?"]'
			);
			for (const link of links) {
				let node = link.parentElement;
				for (let depth = 0; depth < 8 && node; depth++, node = node.parentElement) {
					if (!normalize(node.innerText).includes(wantedText)) continue;
					const icon = node.querySelector('svg[aria-label="' + aria + '"]');
					const button = icon?.closest('[role="button"]');
					if (button) {
						button.click();
						return 'dom';
					}
				}
			}
			throw new Error('comment ' + action + ' failed: HTTP ' + lastStatus);
		})()
	`, pkJSON, actionJSON, usernameJSON, textJSON, expectedAppID)

	var route string
	return chromedp.Run(b.ctx, chromedp.Evaluate(js, &route,
		func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}))
}

// ToggleRepost clicks the repost button for the current reel
func (b *ChromeBackend) ToggleRepost() (bool, error) {
	if b.IsSyncing() {
		return false, fmt.Errorf("Still syncing to reel")
	}

	_, pk, err := b.activeCursor().Current()
	if err != nil {
		return false, err
	}

	js := `
		(() => {
			document.querySelectorAll('[data-reels-repost-btn]').forEach(el => {
				el.removeAttribute('data-reels-repost-btn');
			});

			const videos = document.querySelectorAll('video[playsinline]');
			for (const video of videos) {
				const rect = video.getBoundingClientRect();
				const viewportHeight = window.innerHeight;
				const videoCenter = rect.top + rect.height / 2;
				if (videoCenter > 0 && videoCenter < viewportHeight) {
					let parent = video.parentElement;
					for (let i = 0; i < 15; i++) {
						if (!parent) break;
						const svgs = parent.querySelectorAll('svg[aria-label="Repost"]');
						for (const svg of svgs) {
							if (svg.getAttribute('viewBox') !== '0 0 24 24') continue;
							const btn = svg.closest('[role="button"]');
							if (btn) {
								btn.setAttribute('data-reels-repost-btn', 'true');
								return true;
							}
						}
						parent = parent.parentElement;
					}
				}
			}
			return false;
		})()
	`
	var found bool
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &found)); err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	if err := chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-repost-btn="true"]`, chromedp.ByQuery),
	); err != nil {
		return false, err
	}

	b.mutateReelByPK(pk, func(r *Reel) { r.Reposted = !r.Reposted })
	return true, nil
}

// ToggleSave clicks the bookmark/save button for the current reel
func (b *ChromeBackend) ToggleSave() (bool, error) {
	if b.IsSyncing() {
		return false, fmt.Errorf("Still syncing to reel")
	}

	_, pk, err := b.activeCursor().Current()
	if err != nil {
		return false, err
	}

	js := `
		(() => {
			document.querySelectorAll('[data-reels-save-btn]').forEach(el => {
				el.removeAttribute('data-reels-save-btn');
			});

			const videos = document.querySelectorAll('video[playsinline]');
			for (const video of videos) {
				const rect = video.getBoundingClientRect();
				const viewportHeight = window.innerHeight;
				const videoCenter = rect.top + rect.height / 2;
				if (videoCenter > 0 && videoCenter < viewportHeight) {
					let parent = video.parentElement;
					for (let i = 0; i < 15; i++) {
						if (!parent) break;
						const svg = parent.querySelector('svg[aria-label="Save"], svg[aria-label="Remove"]');
						if (svg) {
							const btn = svg.closest('[role="button"]');
							if (btn) {
								btn.setAttribute('data-reels-save-btn', 'true');
								return true;
							}
						}
						parent = parent.parentElement;
					}
				}
			}
			return false;
		})()
	`
	var found bool
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &found)); err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	if err := chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-save-btn="true"]`, chromedp.ByQuery),
	); err != nil {
		return false, err
	}

	b.mutateReelByPK(pk, func(r *Reel) { r.Saved = !r.Saved })
	return true, nil
}

// OpenComments opens the comments panel for the current reel
func (b *ChromeBackend) OpenComments() {
	if b.IsSyncing() {
		return
	}

	_, pk, err := b.activeCursor().Current()
	if err != nil {
		return
	}
	b.comments.Open(pk)
	b.clickCommentsButton()
}

// CloseComments closes the comments panel UI
func (b *ChromeBackend) CloseComments() {
	if b.IsSyncing() {
		return
	}

	b.clickCloseButton()
}

// ClearComments closes the comments panel and clears the cache
// Note: Pagination state is persisted in the Reel struct itself and is not wiped
func (b *ChromeBackend) ClearComments() {
	b.comments.Clear()
	b.clickCloseButton()
}

// getCommentsPagination returns the pagination state for the currently open comments reel.
// Returns nil if no comments are open or the reel isn't found.
func (b *ChromeBackend) getCommentsPagination() *CommentsPagination {
	pk := b.comments.GetReelPK()
	if pk == "" {
		return nil
	}
	reel, ok := b.reelByPK(pk)
	if !ok {
		return nil
	}
	return reel.CommentsPagination
}

// setCommentsPagination updates pagination fields on the currently open comments reel.
func (b *ChromeBackend) setCommentsPagination(cursor string, hasNextPage bool) {
	pk := b.comments.GetReelPK()
	if pk == "" {
		return
	}
	b.mutateReelByPK(pk, func(r *Reel) {
		if r.CommentsPagination == nil {
			r.CommentsPagination = &CommentsPagination{}
		}
		r.CommentsPagination.Cursor = cursor
		r.CommentsPagination.HasNextPage = hasNextPage
	})
}

// enableCommentsPagination stores the request template and marks pagination as validated.
func (b *ChromeBackend) enableCommentsPagination(template string) {
	pk := b.comments.GetReelPK()
	if pk == "" {
		return
	}
	b.mutateReelByPK(pk, func(r *Reel) {
		if r.CommentsPagination == nil {
			r.CommentsPagination = &CommentsPagination{}
		}
		r.CommentsPagination.RequestTemplate = template
		r.CommentsPagination.PaginationEnabled = true
	})
}

// clickCloseButton finds and clicks the Close button in the browser
func (b *ChromeBackend) clickCloseButton() {
	js := `
		(() => {
			document.querySelectorAll('[data-reels-close-btn]').forEach(el => {
				el.removeAttribute('data-reels-close-btn');
			});

			const svg = document.querySelector('svg[aria-label="Close"]');
			if (svg) {
				const btn = svg.closest('[role="button"]') || svg.parentElement;
				if (btn) {
					btn.setAttribute('data-reels-close-btn', 'true');
					return true;
				}
			}
			return false;
		})()
	`
	var found bool
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &found)); err != nil || !found {
		return
	}

	chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-close-btn="true"]`, chromedp.ByQuery),
	)
}

// clickCommentsButton finds and clicks the comments button for the visible video
func (b *ChromeBackend) clickCommentsButton() {
	js := `
		(() => {
			// Clear old markers first
			document.querySelectorAll('[data-reels-comment-btn]').forEach(el => {
				el.removeAttribute('data-reels-comment-btn');
			});

			const videos = document.querySelectorAll('video[playsinline]');
			for (const video of videos) {
				const rect = video.getBoundingClientRect();
				const viewportHeight = window.innerHeight;
				const videoCenter = rect.top + rect.height / 2;
				if (videoCenter > 0 && videoCenter < viewportHeight) {
					let parent = video.parentElement;
					for (let i = 0; i < 15; i++) {
						if (!parent) break;
						const svg = parent.querySelector('svg[aria-label="Comment"]');
						if (svg) {
							const btn = svg.closest('[role="button"]');
							if (btn) {
								btn.setAttribute('data-reels-comment-btn', 'true');
								return true;
							}
						}
						parent = parent.parentElement;
					}
				}
			}
			return false;
		})()
	`
	var found bool
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &found)); err != nil || !found {
		return
	}

	chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-comment-btn="true"]`, chromedp.ByQuery),
	)
}

// OpenSharePanel clicks the share button to open Instagram's share modal,
// then scrapes the friend list from the DOM.
func (b *ChromeBackend) OpenSharePanel() {
	if b.IsSyncing() {
		return
	}

	js := `
		(() => {
			// Clear old markers first
			document.querySelectorAll('[data-reels-share-btn]').forEach(el => {
				el.removeAttribute('data-reels-share-btn');
			});

			const videos = document.querySelectorAll('video[playsinline]');
			for (const video of videos) {
				const rect = video.getBoundingClientRect();
				const viewportHeight = window.innerHeight;
				const videoCenter = rect.top + rect.height / 2;
				if (videoCenter > 0 && videoCenter < viewportHeight) {
					let parent = video.parentElement;
					for (let i = 0; i < 15; i++) {
						if (!parent) break;
						const svg = parent.querySelector('svg[aria-label="Share"]');
						if (svg) {
							const btn = svg.closest('[role="button"]');
							if (btn) {
								btn.setAttribute('data-reels-share-btn', 'true');
								return true;
							}
						}
						parent = parent.parentElement;
					}
				}
			}
			return false;
		})()
	`
	var found bool
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &found)); err != nil || !found {
		return
	}

	chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-share-btn="true"]`, chromedp.ByQuery),
	)

	// Wait for the share modal to render
	chromedp.Run(b.ctx,
		chromedp.WaitVisible(`img[alt="User avatar"]`, chromedp.ByQuery),
	)
	// Scrape friend list from the DOM
	js = `
		(() => {
			const imgs = document.querySelectorAll('img[alt="User avatar"]');
			const results = Array.from(imgs).map((img) => {
				const btn = img.closest('[role="button"][tabindex="0"]');
				if (!btn) return null;

				let name = "";
				const avatarDiv = img.parentElement?.parentElement;
				if (avatarDiv) {
					const siblingDiv = avatarDiv.nextElementSibling || avatarDiv.previousElementSibling;
					if (siblingDiv) {
						const outerSpan = siblingDiv.querySelector('span');
						if (outerSpan) {
							const innerSpan = outerSpan.querySelector('span');
							if (innerSpan) {
								name = innerSpan.textContent.trim();
							}
						}
					}
				}

				return { name, imgSrc: img.src || "" };
			});
			return JSON.stringify(results.filter(r => r !== null));
		})()
	`

	var result string
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &result)); err != nil {
		return
	}

	var raw []struct {
		Name   string `json:"name"`
		ImgSrc string `json:"imgSrc"`
	}
	if err := json.Unmarshal([]byte(result), &raw); err != nil {
		return
	}

	// Download all friend profile pics in parallel
	urls := make([]string, len(raw))
	for i, r := range raw {
		urls[i] = r.ImgSrc
	}

	data := fetchURLsHTTP(urls)

	friends := make([]User, len(raw))
	for i, r := range raw {
		friends[i] = User{Name: r.Name, ImgSrc: r.ImgSrc}
		if i < len(data) && data[i] != nil {
			friends[i].ImgPath = b.cacheSharePfp(fmt.Sprintf("share_pfp_%d.jpg", i), data[i])
		}
	}
	b.shareFriends = friends

	b.events <- Event{Type: EventShareFriendsLoaded, Count: len(friends)}
}

// GetShareFriends returns the friend list scraped from the share modal
func (b *ChromeBackend) GetShareFriends() []User {
	return b.shareFriends
}

// ToggleShareFriend clicks the friend at the given index in the share modal.
// Finds the Nth img[alt="User avatar"], traverses up to its button, and clicks it.
func (b *ChromeBackend) ToggleShareFriend(index int) {
	js := fmt.Sprintf(`
		(() => {
			// Clear old markers
			document.querySelectorAll('[data-reels-share-friend]').forEach(el => {
				el.removeAttribute('data-reels-share-friend');
			});

			const imgs = document.querySelectorAll('img[alt="User avatar"]');
			let btnIndex = 0;
			for (const img of imgs) {
				const btn = img.closest('[role="button"][tabindex="0"]');
				if (btn) {
					if (btnIndex === %d) {
						btn.setAttribute('data-reels-share-friend', 'true');
						return true;
					}
					btnIndex++;
				}
			}
			return false;
		})()
	`, index)

	var found bool
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &found)); err != nil || !found {
		return
	}

	chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-share-friend="true"]`, chromedp.ByQuery),
	)
}

// SendShare clicks the Send button in the share modal.
// Instagram closes the modal immediately on successful send.
func (b *ChromeBackend) SendShare() (bool, error) {
	if b.IsSyncing() {
		return false, fmt.Errorf("syncing")
	}

	js := `
		(() => {
			document.querySelectorAll('[data-reels-send-btn]').forEach(el => {
				el.removeAttribute('data-reels-send-btn');
			});

			const buttons = document.querySelectorAll('div[role="button"]');
			for (const btn of buttons) {
				if (btn.textContent.trim().toLowerCase().includes('send')) {
					btn.setAttribute('data-reels-send-btn', 'true');
					return btn.getAttribute('aria-disabled') === 'true' ? 'disabled' : 'ok';
				}
			}
			return 'notfound';
		})()
	`
	var result string
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &result)); err != nil {
		b.clickCloseButton()
		return false, fmt.Errorf("js error: %w", err)
	}

	switch result {
	case "disabled":
		return false, fmt.Errorf("send button disabled")
	case "notfound":
		b.clickCloseButton()
		return false, nil
	}

	chromedp.Run(b.ctx,
		chromedp.Click(`[data-reels-send-btn="true"]`, chromedp.ByQuery),
	)
	return true, nil
}

// GetCommentsReelPK returns which reel we're fetching comments for
func (b *ChromeBackend) GetCommentsReelPK() string {
	return b.comments.GetReelPK()
}
