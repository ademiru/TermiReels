package backend

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchURLsHTTPContextCancelsInFlightRequest(t *testing.T) {
	original := assetHTTPClient
	started := make(chan struct{})
	assetHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	defer func() { assetHTTPClient = original }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan [][]byte, 1)
	go func() {
		done <- fetchURLsHTTPContext(ctx, []string{"https://example.invalid/video"})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()

	select {
	case got := <-done:
		if len(got) != 1 || got[0] != nil {
			t.Fatalf("cancelled fetch returned data: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled fetch did not return promptly")
	}
}
