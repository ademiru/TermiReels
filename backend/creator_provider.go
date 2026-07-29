package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const creatorProviderProtocol = 1

var (
	creatorSourceIDPattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	creatorUserIDPattern    = regexp.MustCompile(`^[0-9]+$`)
	creatorUsernamePattern  = regexp.MustCompile(`^[a-z0-9._]{1,30}$`)
	creatorShortcodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

type CreatorProviderItem struct {
	Ordinal            int    `json:"ordinal"`
	Shortcode          string `json:"shortcode"`
	PK                 string `json:"pk"`
	OwnerUsername      string `json:"owner_username"`
	VideoURL           string `json:"video_url"`
	ProfilePicURL      string `json:"profile_pic_url"`
	Caption            string `json:"caption"`
	Liked              bool   `json:"liked"`
	Saved              bool   `json:"saved"`
	Reposted           bool   `json:"reposted"`
	LikeCount          int    `json:"like_count"`
	CommentCount       int    `json:"comment_count"`
	RepostCount        int    `json:"repost_count"`
	CommentsDisabled   bool   `json:"comments_disabled"`
	Verified           bool   `json:"verified"`
	CanViewerReshare   bool   `json:"can_viewer_reshare"`
	MusicTitle         string `json:"music_title"`
	MusicArtist        string `json:"music_artist"`
	MusicExplicit      bool   `json:"music_explicit"`
	GridSeen           bool   `json:"grid_seen"`
	TargetResponseSeen bool   `json:"target_response_seen"`
}

type CreatorSnapshot struct {
	SourceID        string                `json:"source_id"`
	Username        string                `json:"username"`
	InstagramUserID string                `json:"instagram_user_id"`
	Revision        int                   `json:"revision"`
	Items           []CreatorProviderItem `json:"items"`
	NextCursor      string                `json:"next_cursor"`
}

type creatorProviderRequest struct {
	Version int         `json:"version"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type creatorProviderResponse struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type creatorProviderReply struct {
	result json.RawMessage
	err    error
}

// CreatorProviderClient supervises the optional Playwright process. A provider
// failure is returned to its caller and never changes the active feed cursor.
type CreatorProviderClient struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	mu           sync.Mutex
	waitMu       sync.Mutex
	wait         map[string]chan creatorProviderReply
	nextID       atomic.Uint64
	done         chan struct{}
	closeOnce    sync.Once
	shutdownOnce sync.Once
}

func StartCreatorProvider(nodePath, scriptPath string) (*CreatorProviderClient, error) {
	if nodePath == "" {
		nodePath = "node"
	}
	cmd := exec.Command(nodePath, scriptPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	client := &CreatorProviderClient{
		cmd: cmd, stdin: stdin, wait: make(map[string]chan creatorProviderReply), done: make(chan struct{}),
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go client.readResponses(stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("creator provider: %s", scanner.Text())
		}
	}()
	go func() {
		err := cmd.Wait()
		client.shutdown(fmt.Errorf("creator provider exited: %w", err))
	}()
	return client, nil
}

func (c *CreatorProviderClient) readResponses(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var frame creatorProviderResponse
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			c.shutdown(fmt.Errorf("invalid creator provider frame: %w", err))
			return
		}
		if frame.Version != creatorProviderProtocol || frame.ID == "" {
			continue
		}
		c.waitMu.Lock()
		ch := c.wait[frame.ID]
		delete(c.wait, frame.ID)
		c.waitMu.Unlock()
		if ch == nil {
			continue
		}
		reply := creatorProviderReply{result: frame.Result}
		if frame.Error != nil {
			reply.err = fmt.Errorf("%s: %s", frame.Error.Code, frame.Error.Message)
		}
		ch <- reply
		close(ch)
	}
	if err := scanner.Err(); err != nil {
		c.shutdown(err)
	}
}

func (c *CreatorProviderClient) call(ctx context.Context, method string, params interface{}, out interface{}) error {
	id := fmt.Sprintf("go-%d", c.nextID.Add(1))
	ch := make(chan creatorProviderReply, 1)
	c.waitMu.Lock()
	c.wait[id] = ch
	c.waitMu.Unlock()

	frame := creatorProviderRequest{Version: creatorProviderProtocol, ID: id, Method: method, Params: params}
	payload, err := json.Marshal(frame)
	if err == nil {
		c.mu.Lock()
		_, err = c.stdin.Write(append(payload, '\n'))
		c.mu.Unlock()
	}
	if err != nil {
		c.removeWaiter(id)
		return err
	}
	select {
	case reply := <-ch:
		if reply.err != nil {
			return reply.err
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(reply.result, out)
	case <-ctx.Done():
		c.removeWaiter(id)
		if method != "creator.cancel" {
			go c.sendCancel(id)
		}
		return ctx.Err()
	case <-c.done:
		c.removeWaiter(id)
		return errors.New("creator provider is not running")
	}
}

func (c *CreatorProviderClient) sendCancel(requestID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var ignored json.RawMessage
	_ = c.call(ctx, "creator.cancel", map[string]string{"request_id": requestID}, &ignored)
}

func (c *CreatorProviderClient) removeWaiter(id string) {
	c.waitMu.Lock()
	delete(c.wait, id)
	c.waitMu.Unlock()
}

func (c *CreatorProviderClient) Health(ctx context.Context) error {
	var result struct {
		Status   string `json:"status"`
		Protocol int    `json:"protocol"`
	}
	if err := c.call(ctx, "health", nil, &result); err != nil {
		return err
	}
	if result.Status != "ok" || result.Protocol != creatorProviderProtocol {
		return fmt.Errorf("creator provider health mismatch")
	}
	return nil
}

func (c *CreatorProviderClient) Warm(ctx context.Context) error {
	var result struct {
		Status string `json:"status"`
	}
	if err := c.call(ctx, "warm", nil, &result); err != nil {
		return err
	}
	if result.Status != "ready" {
		return fmt.Errorf("creator provider warm-up mismatch")
	}
	return nil
}

func (c *CreatorProviderClient) Resolve(ctx context.Context, username string, limit int) (CreatorSnapshot, error) {
	var snapshot CreatorSnapshot
	err := c.call(ctx, "creator.resolve", map[string]interface{}{
		"username": username,
		"limit":    limit,
	}, &snapshot)
	if err != nil {
		return CreatorSnapshot{}, err
	}
	if err := ValidateCreatorSnapshot(snapshot, username); err != nil {
		return CreatorSnapshot{}, err
	}
	return snapshot, nil
}

func ValidateCreatorSnapshot(snapshot CreatorSnapshot, expectedUsername string) error {
	expected := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(expectedUsername), "@"))
	if !creatorSourceIDPattern.MatchString(snapshot.SourceID) || snapshot.Revision != 1 {
		return fmt.Errorf("invalid creator snapshot identity")
	}
	if !creatorUsernamePattern.MatchString(snapshot.Username) || snapshot.Username != expected {
		return fmt.Errorf("creator snapshot username mismatch: got @%s want @%s", snapshot.Username, expected)
	}
	if !creatorUserIDPattern.MatchString(snapshot.InstagramUserID) || len(snapshot.Items) == 0 {
		return fmt.Errorf("creator snapshot is incomplete")
	}
	codes := make(map[string]struct{}, len(snapshot.Items))
	pks := make(map[string]struct{}, len(snapshot.Items))
	for i, item := range snapshot.Items {
		if item.Ordinal != i+1 {
			return fmt.Errorf("creator snapshot ordinal %d is not consecutive", item.Ordinal)
		}
		videoURL, urlErr := url.Parse(item.VideoURL)
		if !creatorShortcodePattern.MatchString(item.Shortcode) ||
			!creatorUserIDPattern.MatchString(item.PK) ||
			urlErr != nil || videoURL.Scheme != "https" || videoURL.Host == "" {
			return fmt.Errorf("creator snapshot item %d is not playable", item.Ordinal)
		}
		if item.OwnerUsername != "" && !creatorUsernamePattern.MatchString(strings.ToLower(item.OwnerUsername)) {
			return fmt.Errorf("creator snapshot item %d has invalid owner identity", item.Ordinal)
		}
		if item.LikeCount < 0 || item.CommentCount < 0 || item.RepostCount < 0 {
			return fmt.Errorf("creator snapshot item %d has invalid counters", item.Ordinal)
		}
		if item.ProfilePicURL != "" {
			profileURL, profileErr := url.Parse(item.ProfilePicURL)
			if profileErr != nil || profileURL.Scheme != "https" || profileURL.Host == "" {
				return fmt.Errorf("creator snapshot item %d has invalid profile image", item.Ordinal)
			}
		}
		if !item.GridSeen || !item.TargetResponseSeen {
			return fmt.Errorf("creator snapshot item %d lacks dual evidence", item.Ordinal)
		}
		if _, exists := codes[item.Shortcode]; exists {
			return fmt.Errorf("duplicate creator shortcode %s", item.Shortcode)
		}
		if _, exists := pks[item.PK]; exists {
			return fmt.Errorf("duplicate creator PK %s", item.PK)
		}
		codes[item.Shortcode] = struct{}{}
		pks[item.PK] = struct{}{}
	}
	return nil
}

func (c *CreatorProviderClient) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *CreatorProviderClient) Close() error {
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
	})
	return nil
}

func (c *CreatorProviderClient) shutdown(err error) {
	c.shutdownOnce.Do(func() {
		close(c.done)
		c.waitMu.Lock()
		for id, ch := range c.wait {
			delete(c.wait, id)
			ch <- creatorProviderReply{err: err}
			close(ch)
		}
		c.waitMu.Unlock()
	})
}

type CreatorAuditBackend interface {
	EnableCreatorAudit(scriptPath string)
	AuditCreator(ctx context.Context, username string)
}

type CreatorProviderWarmBackend interface {
	WarmCreatorProvider()
}

func (b *ChromeBackend) EnableCreatorAudit(scriptPath string) {
	b.creatorProviderMu.Lock()
	b.creatorScript = scriptPath
	b.creatorProviderMu.Unlock()
}

func (b *ChromeBackend) EnableCreatorProvider(scriptPath string) {
	b.creatorProviderMu.Lock()
	b.creatorScript = scriptPath
	b.creatorEnabled = scriptPath != ""
	b.creatorProviderMu.Unlock()
}

func (b *ChromeBackend) WarmCreatorProvider() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client, err := b.creatorProviderClient(ctx)
	if err == nil {
		err = client.Warm(ctx)
	}
	if err != nil {
		// Warm-up is opportunistic. Resolve retries through the same supervised
		// path on demand, so startup and the main feed remain independent.
		log.Printf("creator provider warm-up skipped: %v", err)
	}
}

func (b *ChromeBackend) creatorProviderClient(ctx context.Context) (*CreatorProviderClient, error) {
	b.creatorProviderMu.Lock()
	defer b.creatorProviderMu.Unlock()
	if b.creatorScript == "" {
		return nil, fmt.Errorf("creator provider is not configured")
	}
	if b.creatorProvider != nil && b.creatorProvider.alive() {
		return b.creatorProvider, nil
	}
	if b.creatorProvider != nil {
		_ = b.creatorProvider.Close()
		b.creatorProvider = nil
	}
	client, err := StartCreatorProvider("node", b.creatorScript)
	if err != nil {
		return nil, err
	}
	healthCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = client.Health(healthCtx)
	cancel()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	b.creatorProvider = client
	return client, nil
}

func (b *ChromeBackend) resolveCreatorSnapshot(ctx context.Context, username string, limit int) (CreatorSnapshot, error) {
	b.creatorProviderMu.Lock()
	enabled := b.creatorEnabled
	b.creatorProviderMu.Unlock()
	if !enabled {
		return CreatorSnapshot{}, fmt.Errorf("creator reel browsing is not enabled")
	}
	client, err := b.creatorProviderClient(ctx)
	if err != nil {
		return CreatorSnapshot{}, err
	}
	snapshot, err := client.Resolve(ctx, username, limit)
	if err != nil && !client.alive() {
		b.creatorProviderMu.Lock()
		if b.creatorProvider == client {
			b.creatorProvider = nil
		}
		b.creatorProviderMu.Unlock()
	}
	return snapshot, err
}

func (b *ChromeBackend) AuditCreator(ctx context.Context, username string) {
	username = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
	if username == "" {
		return
	}
	b.creatorProviderMu.Lock()
	if b.creatorScript == "" || b.creatorAuditSeen[username] {
		b.creatorProviderMu.Unlock()
		return
	}
	b.creatorAuditSeen[username] = true
	b.creatorProviderMu.Unlock()

	client, err := b.creatorProviderClient(ctx)
	if err != nil {
		b.creatorProviderMu.Lock()
		delete(b.creatorAuditSeen, username)
		b.creatorProviderMu.Unlock()
		log.Printf("creator provider audit unavailable: %v", err)
		return
	}
	snapshot, err := client.Resolve(ctx, username, 12)
	if err != nil {
		b.creatorProviderMu.Lock()
		delete(b.creatorAuditSeen, username)
		if !client.alive() && b.creatorProvider == client {
			b.creatorProvider = nil
		}
		b.creatorProviderMu.Unlock()
		log.Printf("creator provider audit failed: @%s: %v", username, err)
		return
	}
	log.Printf(
		"creator provider audit passed: @%s user_id=%s source=%s items=%d",
		snapshot.Username, snapshot.InstagramUserID, snapshot.SourceID, len(snapshot.Items),
	)
}
