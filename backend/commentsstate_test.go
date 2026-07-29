package backend

import "testing"

func TestCommentsStateClearReleasesInFlightFetch(t *testing.T) {
	state := &CommentsState{}
	state.Open("old-reel")
	if !state.StartFetch() {
		t.Fatal("initial fetch did not start")
	}

	state.Clear()
	state.Open("new-reel")
	if !state.StartFetch() {
		t.Fatal("cleared comments retained the previous reel's fetch lock")
	}
}
