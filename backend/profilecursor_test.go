package backend

import (
	"context"
	"encoding/json"
	"testing"
)

func TestProfileCodeFromHref(t *testing.T) {
	tests := map[string]string{
		"https://www.instagram.com/reel/ABC_123/": "ABC_123",
		"/reel/shortcode/?utm_source=test":        "shortcode",
		"https://www.instagram.com/p/not-a-reel/": "",
	}
	for href, want := range tests {
		if got := profileCodeFromHref(href); got != want {
			t.Errorf("profileCodeFromHref(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestCreatorReelBrowsingIsDisabled(t *testing.T) {
	b := &ChromeBackend{}
	if _, err := b.EnterCreatorProfile("creator"); err == nil {
		t.Fatal("creator reel browsing unexpectedly remained enabled")
	}
	if b.IsProfileMode() {
		t.Fatal("disabled creator browsing changed the active source")
	}
}

func TestResolvedProfileCursorPinsVerifiedMembership(t *testing.T) {
	pc, err := newResolvedProfileCursor(
		context.Background(),
		"creator",
		"source-id",
		[]string{"first", "collaboration"},
		[]string{"11", "22"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !pc.contains("collaboration", "22") {
		t.Fatal("verified collaboration is missing from the creator cursor")
	}
	if pc.contains("recommendation", "99") {
		t.Fatal("unverified recommendation joined the creator cursor")
	}
	if got := pc.SourceID(); got != "source-id" {
		t.Fatalf("source id = %q", got)
	}
}

func TestResolvedProfileCursorRejectsPartialSnapshot(t *testing.T) {
	if _, err := newResolvedProfileCursor(
		context.Background(), "creator", "source", []string{"one"}, nil,
	); err == nil {
		t.Fatal("partial snapshot was accepted")
	}
}

func TestCreatorProviderReelPreservesPresentationMetadata(t *testing.T) {
	reel := creatorProviderReel(CreatorProviderItem{
		PK: "11", Shortcode: "code", OwnerUsername: "collaborator",
		VideoURL: "https://video.example/reel.mp4", Caption: "caption",
		LikeCount: 10, CommentCount: 2, MusicTitle: "song", MusicArtist: "artist",
	}, "creator")
	if reel.Username != "collaborator" || reel.Caption != "caption" || reel.LikeCount != 10 {
		t.Fatalf("provider metadata was not preserved: %#v", reel)
	}
	if reel.Music == nil || reel.Music.Title != "song" || reel.Music.Artist != "artist" {
		t.Fatalf("provider music metadata was not preserved: %#v", reel.Music)
	}
}

func TestProfileCursorCaptureStaysInItsGrid(t *testing.T) {
	pc := NewProfileCursor(context.Background(), "creator", []string{"one", "two"})
	if pc.capture("recommended", "99") {
		t.Fatal("recommendation payload must not join the creator grid")
	}
	if !pc.capture("two", "22") {
		t.Fatal("creator reel was not captured")
	}
	if got := pc.PKAt(2); got != "22" {
		t.Fatalf("PKAt(2) = %q, want 22", got)
	}
	if got := pc.Total(); got != 2 {
		t.Fatalf("Total() = %d, want 2", got)
	}
}

func TestProfileCursorAppendCodesDeduplicates(t *testing.T) {
	pc := NewProfileCursor(context.Background(), "creator", []string{"one"})
	if got := pc.appendCodes([]string{"one", "two", "two", ""}); got != 2 {
		t.Fatalf("appendCodes total = %d, want 2", got)
	}
}

func TestProfileCursorPrimePrependsOlderSourceReel(t *testing.T) {
	pc := NewProfileCursor(context.Background(), "creator", []string{"newest", "newer"})
	if got := pc.prime("older-source", "99"); got != 1 {
		t.Fatalf("prime index = %d, want 1", got)
	}
	index, pk, err := pc.Current()
	if err != nil {
		t.Fatal(err)
	}
	if index != 1 || pk != "99" {
		t.Fatalf("Current() = (%d, %q), want (1, 99)", index, pk)
	}
	if got := pc.Total(); got != 3 {
		t.Fatalf("Total() = %d, want 3", got)
	}
}

func TestProfileCursorHydrationGuardsDOMActions(t *testing.T) {
	pc := NewProfileCursor(context.Background(), "creator", []string{"one"})
	if pc.IsSyncing() {
		t.Fatal("new profile cursor unexpectedly syncing")
	}
	pc.setHydrating(true)
	if !pc.IsSyncing() {
		t.Fatal("profile hydration must report syncing")
	}
	pc.setHydrating(false)
	if pc.IsSyncing() {
		t.Fatal("profile cursor remained syncing after hydration")
	}
}

func TestProfileCursorInstallsRealGridOrder(t *testing.T) {
	pc := NewProfileCursor(context.Background(), "creator", []string{"source"})
	pc.prime("source", "99")
	if got := pc.installGrid([]string{"newest", "source", "older"}); got != 3 {
		t.Fatalf("installGrid total = %d, want 3", got)
	}
	index, _, err := pc.Current()
	if err == nil {
		t.Fatal("unresolved first grid reel unexpectedly current")
	}
	if index != 0 {
		t.Fatalf("unresolved Current index = %d, want 0", index)
	}
	if got := pc.PKAt(2); got != "99" {
		t.Fatalf("source PK moved without identity: got %q, want 99", got)
	}
	if got := pc.codeAt(1); got != "newest" {
		t.Fatalf("first grid code = %q, want newest", got)
	}
}

func TestProfileCursorDoesNotForceSourceIntoCreatorGrid(t *testing.T) {
	pc := NewProfileCursor(context.Background(), "creator", []string{"main-feed-source"})
	pc.prime("main-feed-source", "99")
	pc.installGrid([]string{"creator-one", "creator-two"})

	if got := pc.Total(); got != 2 {
		t.Fatalf("grid total = %d, want 2", got)
	}
	for i := 1; i <= pc.Total(); i++ {
		if got := pc.codeAt(i); got == "main-feed-source" {
			t.Fatalf("source reel leaked into creator grid at %d", i)
		}
	}
}

func TestProfileCaptureDoesNotContaminateMainFeed(t *testing.T) {
	feed := NewFeedCursor(context.Background())
	profile := NewProfileCursor(context.Background(), "creator", []string{"creator-code"})
	b := &ChromeBackend{
		reels:   make(map[string]*Reel),
		feed:    feed,
		active:  profile,
		profile: profile,
	}

	var response reelResponse
	response.Data.Connection.Edges = append(response.Data.Connection.Edges, struct {
		Node struct {
			Media reelMedia `json:"media"`
		} `json:"node"`
	}{})
	response.Data.Connection.Edges[0].Node.Media.PK = "123"
	response.Data.Connection.Edges[0].Node.Media.Code = "creator-code"
	response.Data.Connection.Edges[0].Node.Media.User.Username = "creator"
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	b.processReelResponse(string(body), profile)
	if got := profile.PKAt(1); got != "123" {
		t.Fatalf("profile PK = %q, want 123", got)
	}
	if got := feed.Total(); got != 0 {
		t.Fatalf("main feed received %d profile reels", got)
	}
}

func TestMainFeedResponseRoutesToFeedWhileProfileExists(t *testing.T) {
	feed := NewFeedCursor(context.Background())
	profile := NewProfileCursor(context.Background(), "creator", []string{"creator-code"})
	b := &ChromeBackend{
		reels:   make(map[string]*Reel),
		feed:    feed,
		active:  profile,
		profile: profile,
	}

	var response reelResponse
	response.Data.Connection.Edges = append(response.Data.Connection.Edges, struct {
		Node struct {
			Media reelMedia `json:"media"`
		} `json:"node"`
	}{})
	response.Data.Connection.Edges[0].Node.Media.PK = "main-123"
	response.Data.Connection.Edges[0].Node.Media.Code = "main-code"
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	b.processReelResponse(string(body), nil)
	if got := feed.PKAt(1); got != "main-123" {
		t.Fatalf("main feed PK = %q, want main-123", got)
	}
	if got := profile.PKAt(1); got != "" {
		t.Fatalf("profile received main feed PK %q", got)
	}
}

func TestProfileResponseRejectsAnotherCreatorsMatchingGridCode(t *testing.T) {
	feed := NewFeedCursor(context.Background())
	profile := NewProfileCursor(context.Background(), "expected_creator", []string{"grid-code"})
	b := &ChromeBackend{
		reels:   make(map[string]*Reel),
		feed:    feed,
		active:  profile,
		profile: profile,
	}

	var response reelResponse
	response.Data.Connection.Edges = append(response.Data.Connection.Edges, struct {
		Node struct {
			Media reelMedia `json:"media"`
		} `json:"node"`
	}{})
	media := &response.Data.Connection.Edges[0].Node.Media
	media.PK = "wrong-owner-pk"
	media.Code = "grid-code"
	media.User.Username = "another_creator"
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	b.processReelResponse(string(body), profile)
	if got := profile.PKAt(1); got != "" {
		t.Fatalf("profile accepted another creator's reel PK %q", got)
	}
	if got := feed.Total(); got != 0 {
		t.Fatalf("rejected profile media leaked into main feed: %d entries", got)
	}
}

func TestReelResponseSelectsRequestedCodeNotFirstRecommendation(t *testing.T) {
	var response reelResponse
	for _, item := range []struct{ code, pk string }{
		{"main-feed-recommendation", "wrong-pk"},
		{"requested-profile-code", "right-pk"},
	} {
		edge := struct {
			Node struct {
				Media reelMedia `json:"media"`
			} `json:"node"`
		}{}
		edge.Node.Media.Code = item.code
		edge.Node.Media.PK = item.pk
		response.Data.Connection.Edges = append(response.Data.Connection.Edges, edge)
	}

	media, ok := response.mediaByCode("requested-profile-code")
	if !ok {
		t.Fatal("requested profile reel was not found")
	}
	if media.PK != "right-pk" {
		t.Fatalf("selected PK = %q, want right-pk", media.PK)
	}
	if _, ok := response.mediaByCode("missing"); ok {
		t.Fatal("missing shortcode matched a recommendation")
	}
}

func TestAuthoritativeProfileResponseUsesTargetPayloadOrder(t *testing.T) {
	body := `{"data":{"profile_reels":{"edges":[
		{"node":{"media":{"pk":"11","code":"creator-first","user":{"username":"creator"}}}},
		{"node":{"media":{"pk":"22","code":"creator-second","user":{"username":"creator"}}}}
	]}}}`
	profile := NewProfileCursor(context.Background(), "creator", nil)
	b := &ChromeBackend{reels: make(map[string]*Reel)}

	if got := b.processAuthoritativeProfileResponse(body, profile); got != 2 {
		t.Fatalf("captured %d profile entries, want 2", got)
	}
	codes := profile.authoritativeCodes()
	if len(codes) != 2 || codes[0] != "creator-first" || codes[1] != "creator-second" {
		t.Fatalf("authoritative codes = %#v", codes)
	}
	if profile.PKAt(1) != "" {
		t.Fatal("metadata-only grid entry must remain unresolved until direct reel fetch")
	}
}
