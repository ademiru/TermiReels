package tui

import (
	"strings"
	"testing"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
)

func TestPlaybackGateRejectsSupersededCommit(t *testing.T) {
	gate := &playbackGate{}
	old := gate.next()
	current := gate.next()

	called := false
	committed, err := gate.commit(old, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("stale commit returned error: %v", err)
	}
	if committed || called {
		t.Fatal("superseded playback reached the player")
	}
	if !gate.isCurrent(current) {
		t.Fatal("newest playback generation was not retained")
	}
}

func TestStaleVideoReadyCannotReplaceCurrentReelAssets(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.status = statusLoading
	m.playbackGate = &playbackGate{}
	oldGeneration := m.playbackGate.next()
	m.playbackGate.next()
	m.currentReel = &backend.ReelInfo{
		Reel: backend.Reel{PK: "new-pk", Code: "new-code", Username: "new-owner"},
	}
	originalPFP := &player.Img{}
	m.reelPFP = originalPFP

	updated, _ := m.Update(videoReadyMsg{
		generation: oldGeneration,
		pk:         "old-pk",
		code:       "old-code",
		pfp:        &player.Img{},
	})
	got := updated.(Model)
	if got.currentReel.Username != "new-owner" {
		t.Fatalf("owner changed to %q", got.currentReel.Username)
	}
	if got.reelPFP != originalPFP {
		t.Fatal("stale profile photo replaced the current reel's photo")
	}
	if got.status != statusLoading {
		t.Fatalf("stale completion changed status to %v", got.status)
	}
}

func TestAdoptReelClearsPreviousOwnerVisuals(t *testing.T) {
	m := testModel()
	m.reelPFP = &player.Img{}
	m.reelFloating = []floatingItem{{pfp: &player.Img{}}}
	m.floating = []floatingItem{{pfp: &player.Img{}}}

	next := &backend.ReelInfo{
		Reel: backend.Reel{PK: "next-pk", Code: "next-code", Username: "next-owner"},
	}
	m.adoptReel(next)

	if m.currentReel != next {
		t.Fatal("next reel metadata was not adopted")
	}
	if m.reelPFP != nil || len(m.reelFloating) != 0 || len(m.floating) != 0 {
		t.Fatal("previous reel imagery survived the identity transition")
	}
	if m.status != statusLoading {
		t.Fatalf("adopt status = %v, want loading", m.status)
	}
}

func TestProfileTransitionHUDExplainsEntryAndExit(t *testing.T) {
	m := testModel()
	m.profileOpening = true
	m.profileTarget = "creator"
	entry := m.viewProfileTransitionHUD(60, 4, "")
	if !strings.Contains(entry, "OPENING @creator") ||
		!strings.Contains(entry, "verifying and preparing") {
		t.Fatalf("entry transition lacks context: %q", entry)
	}

	m.profileOpening = false
	m.profileClosing = true
	exit := m.viewProfileTransitionHUD(60, 4, "")
	if !strings.Contains(exit, "RETURNING TO MAIN FEED") ||
		!strings.Contains(exit, "restoring your exact position") {
		t.Fatalf("exit transition lacks context: %q", exit)
	}
}

func TestFollowTransitionHUDWaitsForConfirmation(t *testing.T) {
	m := testModel()
	m.followTarget = "creator"
	view := m.viewFollowTransitionHUD(60, 4, "")
	if !strings.Contains(view, "UPDATING @creator") ||
		!strings.Contains(view, "waiting for Instagram to confirm") {
		t.Fatalf("follow transition lacks confirmation context: %q", view)
	}
}

func TestSupersededFollowResultCannotClearCurrentOperation(t *testing.T) {
	m := testModel()
	m.followRequest = 2
	m.followTarget = "current-creator"
	m.profileBusy = true

	updated, _ := m.Update(profileFollowedMsg{
		username:  "old-creator",
		following: true,
		request:   1,
	})
	got := updated.(Model)
	if got.followTarget != "current-creator" || !got.profileBusy {
		t.Fatal("superseded follow result altered the current follow operation")
	}
}

func TestSupersededProfileResultCannotReplaceCurrentReel(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.profileRequest = 8
	current := &backend.ReelInfo{
		Reel: backend.Reel{PK: "current-pk", Code: "current-code", Username: "current-owner"},
	}
	m.currentReel = current

	updated, _ := m.Update(profileEnteredMsg{
		request: 7,
		info: &backend.ReelInfo{
			Reel: backend.Reel{PK: "stale-pk", Code: "stale-code", Username: "stale-owner"},
		},
	})
	got := updated.(Model)
	if got.currentReel != current {
		t.Fatal("superseded profile result replaced the current reel")
	}
}

func TestSupersededReelLoadCannotReplaceOwnerMetadata(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.reelLoadGate = &playbackGate{}
	staleGeneration := m.reelLoadGate.next()
	m.reelLoadGate.next()
	current := &backend.ReelInfo{
		Reel: backend.Reel{PK: "current-pk", Code: "current-code", Username: "current-owner"},
	}
	m.currentReel = current

	updated, _ := m.Update(reelLoadedMsg{
		generation: staleGeneration,
		info: &backend.ReelInfo{
			Reel: backend.Reel{PK: "old-pk", Code: "old-code", Username: "old-owner"},
		},
	})
	got := updated.(Model)
	if got.currentReel != current {
		t.Fatal("superseded reel load replaced the current owner metadata")
	}
}
