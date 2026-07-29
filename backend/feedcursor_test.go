package backend

import (
	"context"
	"testing"
	"time"
)

func TestFeedSyncDirectionRequiresKnownDirectionalEvidence(t *testing.T) {
	tests := []struct {
		name                string
		current, target     int
		currentPK, targetPK string
		want                int
	}{
		{"already there", 4, 4, "target", "target", 0},
		{"forward", 3, 4, "three", "four", 1},
		{"backward", 5, 4, "five", "four", -1},
		{"unknown DOM reel", 0, 4, "", "four", 0},
		{"equal index conflicting PK", 4, 4, "stale", "four", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := feedSyncDirection(
				tt.current, tt.target, tt.currentPK, tt.targetPK,
			); got != tt.want {
				t.Fatalf("direction = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWaitFeedSyncHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := waitFeedSync(ctx, time.Second); err == nil {
		t.Fatal("cancelled wait returned nil")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("cancelled wait took %s", elapsed)
	}
}
