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
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	b.processReelResponse(string(body))
	if got := profile.PKAt(1); got != "123" {
		t.Fatalf("profile PK = %q, want 123", got)
	}
	if got := feed.Total(); got != 0 {
		t.Fatalf("main feed received %d profile reels", got)
	}
}
