package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validCreatorSnapshot() CreatorSnapshot {
	return CreatorSnapshot{
		SourceID: "6ba7b810-9dad-41d1-80b4-00c04fd430c8",
		Username: "creator", InstagramUserID: "123", Revision: 1,
		Items: []CreatorProviderItem{{
			Ordinal: 1, Shortcode: "code", PK: "456", VideoURL: "https://video.example/reel.mp4",
			GridSeen: true, TargetResponseSeen: true,
		}},
	}
}

func TestValidateCreatorSnapshot(t *testing.T) {
	if err := ValidateCreatorSnapshot(validCreatorSnapshot(), "@Creator"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCreatorSnapshotRejectsMissingEvidence(t *testing.T) {
	snapshot := validCreatorSnapshot()
	snapshot.Items[0].GridSeen = false
	if err := ValidateCreatorSnapshot(snapshot, "creator"); err == nil {
		t.Fatal("snapshot without grid evidence was accepted")
	}
}

func TestValidateCreatorSnapshotRejectsWrongCreator(t *testing.T) {
	snapshot := validCreatorSnapshot()
	snapshot.Username = "someone_else"
	if err := ValidateCreatorSnapshot(snapshot, "creator"); err == nil {
		t.Fatal("wrong creator snapshot was accepted")
	}
}

func TestValidateCreatorSnapshotRejectsDuplicateIdentity(t *testing.T) {
	snapshot := validCreatorSnapshot()
	snapshot.Items = append(snapshot.Items, CreatorProviderItem{
		Ordinal: 2, Shortcode: "code", PK: "789", VideoURL: "https://video.example/reel2.mp4",
		GridSeen: true, TargetResponseSeen: true,
	})
	if err := ValidateCreatorSnapshot(snapshot, "creator"); err == nil {
		t.Fatal("duplicate shortcode was accepted")
	}
}

func TestValidateCreatorSnapshotRejectsUnsafePayload(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreatorSnapshot)
	}{
		{"non numeric user id", func(snapshot *CreatorSnapshot) { snapshot.InstagramUserID = "creator" }},
		{"non numeric pk", func(snapshot *CreatorSnapshot) { snapshot.Items[0].PK = "pk" }},
		{"insecure media url", func(snapshot *CreatorSnapshot) { snapshot.Items[0].VideoURL = "http://video.example/a.mp4" }},
		{"invalid source id", func(snapshot *CreatorSnapshot) { snapshot.SourceID = "source" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := validCreatorSnapshot()
			test.mutate(&snapshot)
			if err := ValidateCreatorSnapshot(snapshot, "creator"); err == nil {
				t.Fatal("unsafe snapshot was accepted")
			}
		})
	}
}

func TestCreatorProviderHealthProtocol(t *testing.T) {
	script := writeProviderScript(t, `
read request
printf '%s\n' '{"version":1,"id":"go-1","result":{"status":"ok","protocol":1}}'
sleep 1
`)
	client, err := StartCreatorProvider("/bin/sh", script)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Health(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCreatorProviderProcessDeathUnblocksCall(t *testing.T) {
	script := writeProviderScript(t, "exit 17\n")
	client, err := StartCreatorProvider("/bin/sh", script)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Health(ctx); err == nil {
		t.Fatal("provider death was not reported")
	}
}

func TestCreatorProviderContextCancellation(t *testing.T) {
	script := writeProviderScript(t, "read request\nsleep 2\n")
	client, err := StartCreatorProvider("/bin/sh", script)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.Health(ctx); err != context.DeadlineExceeded {
		t.Fatalf("got %v, want deadline exceeded", err)
	}
}

func writeProviderScript(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.sh")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
