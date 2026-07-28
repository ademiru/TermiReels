package backend

import (
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// CreatorProfileState is the terminal-facing profile header state.
type CreatorProfileState struct {
	Username  string
	Following bool
	Known     bool
}

// ProfileBackend is an optional Backend extension so test and third-party
// backends do not break when creator browsing is unavailable.
type ProfileBackend interface {
	EnterCreatorProfile(username string) (*ReelInfo, error)
	ExitCreatorProfile() (*ReelInfo, error)
	IsProfileMode() bool
	CreatorProfile() CreatorProfileState
	ToggleCreatorFollow() (bool, error)
}

func (b *ChromeBackend) collectProfileCodes(username string) ([]string, CreatorProfileState, error) {
	target := "https://www.instagram.com/" + username + "/reels/"
	var raw struct {
		Hrefs []string `json:"hrefs"`
		State string   `json:"state"`
	}
	if err := chromedp.Run(b.feedCtx,
		chromedp.Navigate(target),
		chromedp.WaitReady("body"),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`(() => {
			const hrefs = [...document.querySelectorAll('a[href*="/reel/"]')].map(a => a.href);
			const labels = [...document.querySelectorAll('button,[role="button"]')].map(b => (b.innerText || '').trim());
			const state = labels.includes('Following') ? 'following'
				: labels.includes('Requested') ? 'requested'
				: labels.includes('Follow') ? 'not_following' : '';
			return {hrefs, state};
		})()`, &raw),
	); err != nil {
		return nil, CreatorProfileState{}, err
	}

	var codes []string
	seen := make(map[string]struct{})
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
	state := CreatorProfileState{Username: username, Known: raw.State != ""}
	state.Following = raw.State == "following" || raw.State == "requested"
	if len(codes) == 0 {
		return nil, state, fmt.Errorf("@%s has no visible reels", username)
	}
	return codes, state, nil
}

func (b *ChromeBackend) discoverNextProfileReel(pc *ProfileCursor, afterIndex int) (*ReelInfo, error) {
	codes, state, err := b.collectProfileCodes(pc.Username())
	if err != nil {
		_ = pc.SyncTo(afterIndex)
		return nil, err
	}
	b.modeMu.Lock()
	b.profileState = state
	b.modeMu.Unlock()
	total := pc.appendCodes(codes)
	if afterIndex >= total {
		_ = pc.SyncTo(afterIndex)
		return nil, fmt.Errorf("no more visible reels for @%s", pc.Username())
	}
	if err := pc.SyncTo(afterIndex + 1); err != nil {
		return nil, err
	}
	return b.GetCurrent()
}

func (b *ChromeBackend) EnterCreatorProfile(username string) (*ReelInfo, error) {
	b.profileOpMu.Lock()
	defer b.profileOpMu.Unlock()

	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, fmt.Errorf("empty profile username")
	}
	if b.IsChatMode() {
		return nil, fmt.Errorf("exit chat mode before opening a profile")
	}

	returnIndex := 1
	if idx, _, err := b.feed.Current(); err == nil {
		returnIndex = idx
	}
	codes, state, err := b.collectProfileCodes(username)
	if err != nil {
		return nil, err
	}
	pc := NewProfileCursor(b.feedCtx, username, codes)

	b.modeMu.Lock()
	b.profile = pc
	b.profileReturnIndex = returnIndex
	b.profileState = state
	b.active = pc
	b.ctx = b.feedCtx
	b.modeMu.Unlock()

	if err := pc.SyncTo(1); err != nil {
		b.restoreFeedMode()
		_ = chromedp.Run(b.feedCtx, chromedp.Navigate("https://www.instagram.com/reels/"))
		_ = b.feed.SyncTo(returnIndex)
		return nil, err
	}
	return b.GetCurrent()
}

func (b *ChromeBackend) restoreFeedMode() {
	b.modeMu.Lock()
	b.profile = nil
	b.active = b.feed
	b.ctx = b.feedCtx
	b.modeMu.Unlock()
}

func (b *ChromeBackend) ExitCreatorProfile() (*ReelInfo, error) {
	b.profileOpMu.Lock()
	defer b.profileOpMu.Unlock()

	b.modeMu.RLock()
	index := b.profileReturnIndex
	active := b.profile != nil
	b.modeMu.RUnlock()
	if !active {
		return b.GetCurrent()
	}
	b.restoreFeedMode()
	if err := chromedp.Run(b.feedCtx, chromedp.Navigate("https://www.instagram.com/reels/")); err != nil {
		return nil, err
	}
	if err := b.feed.SyncTo(index); err != nil {
		return nil, err
	}
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
	if err := chromedp.Run(b.feedCtx,
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
		if err := chromedp.Run(b.feedCtx,
			chromedp.Sleep(300*time.Millisecond),
			chromedp.Evaluate(`(() => {
				const b = [...document.querySelectorAll('button,[role="button"]')]
					.find(x => (x.innerText || '').trim() === 'Unfollow');
				if (!b) return false;
				b.click();
				return true;
			})()`, &clicked),
		); err != nil || !clicked {
			_ = chromedp.Run(b.feedCtx, chromedp.Navigate(returnURL))
			if err != nil {
				return state.Following, err
			}
			return state.Following, fmt.Errorf("unfollow confirmation unavailable")
		}
		following = false
	case "requested":
		_ = chromedp.Run(b.feedCtx, chromedp.Navigate(returnURL))
		return true, fmt.Errorf("follow request is pending")
	default:
		_ = chromedp.Run(b.feedCtx, chromedp.Navigate(returnURL))
		return state.Following, fmt.Errorf("follow control unavailable")
	}

	b.modeMu.Lock()
	b.profileState.Following = following
	b.profileState.Known = true
	b.modeMu.Unlock()
	if err := chromedp.Run(b.feedCtx, chromedp.Navigate(returnURL)); err != nil {
		return following, err
	}
	return following, nil
}
