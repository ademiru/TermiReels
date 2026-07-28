package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Settings struct {
	ShowNavbar  bool
	RetinaScale int
	// ReelFit scales the reel to the terminal on every resize instead of
	// holding ReelWidth/ReelHeight fixed.
	ReelFit          bool
	ReelWidth        int
	ReelHeight       int
	ReelSizeStep     int
	Volume           float64
	GifCellHeight    int
	PanelShrinkSteps int

	KeysNext         []string
	KeysPrevious     []string
	KeysMute         []string
	KeysPause        []string
	KeysLike         []string
	KeysRepost       []string
	KeysNavbar       []string
	KeysReelSizeInc  []string
	KeysReelSizeDec  []string
	KeysVolUp        []string
	KeysVolDown      []string
	KeysQuit         []string
	KeysCopyLink     []string
	KeysSave         []string
	KeysSeekForward  []string
	KeysSeekBackward []string
	KeysSelect       []string

	KeysShareOpen  []string
	KeysShareClose []string

	KeysCommentsOpen  []string
	KeysCommentsClose []string

	KeysHelpOpen  []string
	KeysHelpClose []string

	KeysChatsOpen  []string
	KeysChatsClose []string

	KeysReactOpen  []string
	KeysReactClose []string

	KeysProfileOpen   []string
	KeysProfileBack   []string
	KeysProfileFollow []string
}

var Config Settings

// confToKey maps key names in reels.conf to bubbletea KeyMsg.String() values.
var ConfToKey = map[string]string{
	"space":  " ",
	"escape": "esc",
}

// KeyToConf maps bubbletea KeyMsg.String() values to key names in reels.conf.
var KeyToConf = map[string]string{
	" ":   "space",
	"esc": "escape",
}

// GetSettings returns a snapshot copy of the current settings.
func GetSettings() Settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return Config
}

// fifoCache is a bounded FIFO that evicts the oldest entry (and its file) when full.
type fifoCache struct {
	mu   sync.Mutex
	list []string
	set  map[string]bool
	max  int
}

func newFIFOCache(max int) *fifoCache {
	return &fifoCache{
		set: make(map[string]bool),
		max: max,
	}
}

func (c *fifoCache) has(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.set[path]
}

func (c *fifoCache) add(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.set[path] {
		return
	}
	c.list = append(c.list, path)
	c.set[path] = true
	for len(c.list) > c.max {
		os.Remove(c.list[0])
		delete(c.set, c.list[0])
		c.list = c.list[1:]
	}
}

var (
	videoCache    *fifoCache
	reelPfpCache  *fifoCache
	sharePfpCache *fifoCache
	gifCache      *fifoCache
	dmPfpCache    *fifoCache

	cacheMu sync.Mutex
	// inProgress tracks downloads currently in flight; channel is closed when done
	inProgress map[string]chan struct{}

	liked map[string]bool

	settingsMu sync.RWMutex
)

func (b *ChromeBackend) initStorage() error {
	videoCache = newFIFOCache(ReelCacheSize)
	reelPfpCache = newFIFOCache(ReelCacheSize)
	sharePfpCache = newFIFOCache(SharePfpCacheSize)
	gifCache = newFIFOCache(GifCacheSize)
	dmPfpCache = newFIFOCache(DMPfpCacheSize)
	inProgress = make(map[string]chan struct{})
	liked = make(map[string]bool)

	// clear cache on startup
	if err := os.RemoveAll(b.cacheDir); err != nil {
		return fmt.Errorf("could not delete old cache directory")
	}
	if err := os.MkdirAll(b.cacheDir, 0755); err != nil {
		return fmt.Errorf("could not create new cache directory")
	}

	// ensure config directory exists
	if err := os.MkdirAll(b.configDir, 0755); err != nil {
		return fmt.Errorf("could not create config directory")
	}

	// Write the config back out. On a first run this creates the file from
	// defaults; on later runs it appends any keys added since the file was
	// written, leaving the user's own values and comments alone.
	settingsPath := filepath.Join(b.configDir, "reels.conf")
	writeConf(settingsPath, GetSettings())

	return nil
}

// cacheGif writes GIF data to the cache directory with FIFO eviction.
func (b *ChromeBackend) cacheGif(pk string, data []byte) string {
	path := filepath.Join(b.cacheDir, fmt.Sprintf("gif_%s.gif", pk))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}
	gifCache.add(path)
	return path
}

// cacheReelPfp writes a reel profile picture to the cache directory with FIFO eviction.
func (b *ChromeBackend) cacheReelPfp(name string, data []byte) string {
	path := filepath.Join(b.cacheDir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}
	reelPfpCache.add(path)
	return path
}

// cacheDMPfp writes a DM sender profile picture to the cache directory with
// FIFO eviction. The cap is large enough that entries effectively live for
// the whole session.
func (b *ChromeBackend) cacheDMPfp(name string, data []byte) string {
	path := filepath.Join(b.cacheDir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}
	dmPfpCache.add(path)
	return path
}

// cacheSharePfp writes a share panel avatar to the cache directory with FIFO eviction.
func (b *ChromeBackend) cacheSharePfp(name string, data []byte) string {
	path := filepath.Join(b.cacheDir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}
	sharePfpCache.add(path)
	return path
}

// SetReelSize updates the reel bounding box dimensions and persists to disk.
func (b *ChromeBackend) SetReelSize(width, height int) error {
	settingsMu.Lock()
	Config.ReelWidth = width
	Config.ReelHeight = height
	snapshot := Config
	settingsMu.Unlock()

	path := filepath.Join(b.configDir, "reels.conf")
	queueConfWrite(path, snapshot)
	return nil
}

// SetReelFit turns terminal-fitted reel sizing on or off and persists it.
func (b *ChromeBackend) SetReelFit(fit bool) error {
	settingsMu.Lock()
	Config.ReelFit = fit
	snapshot := Config
	settingsMu.Unlock()

	path := filepath.Join(b.configDir, "reels.conf")
	queueConfWrite(path, snapshot)
	return nil
}

// ToggleNavbar updates navbar state to !state, persists to disk, and returns the new state of the navbar
func (b *ChromeBackend) ToggleNavbar() bool {
	settingsMu.Lock()
	Config.ShowNavbar = !Config.ShowNavbar
	showNavbar := Config.ShowNavbar
	snapshot := Config
	settingsMu.Unlock()

	path := filepath.Join(b.configDir, "reels.conf")
	queueConfWrite(path, snapshot)
	return showNavbar
}

// SetVolume updates volume and persists to disk
func (b *ChromeBackend) SetVolume(vol float64) error {
	vol = min(max(vol, 0), 1)
	settingsMu.Lock()
	Config.Volume = vol
	snapshot := Config
	settingsMu.Unlock()

	path := filepath.Join(b.configDir, "reels.conf")
	queueConfWrite(path, snapshot)
	return nil
}

// fetchURLsHTTP fetches multiple URLs in parallel via plain Go HTTP.
// Used for signed CDN URLs that are blocked by CORS when fetched
// from the instagram page context.
// Returns nil for each failed URL
func fetchURLsHTTP(urls []string) [][]byte {
	return fetchURLsHTTPContext(context.Background(), urls)
}

var assetHTTPClient = &http.Client{Timeout: 10 * time.Second}

func fetchURLsHTTPContext(ctx context.Context, urls []string) [][]byte {
	if len(urls) == 0 {
		return nil
	}
	data := make([][]byte, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		if u == "" {
			continue
		}
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return
			}
			resp, err := assetHTTPClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}
			data[i] = b
		}(i, u)
	}
	wg.Wait()
	return data
}

// fetchURLsJS fetches multiple URLs in parallel from the chrome page context.
// Used for URLs that require cookies.
// Returns nil for each failed URL
//
// Currently unused. All our fetched URLs are signed CDN URLs that work over
// plain HTTP (see fetchURLsHTTP).
//
// Note: cross-origin URLs that the page can't reach via fetch() (e.g.
// cdn.fbsbx.com from instagram.com) will silently return empty bytes here
// because of CORS.
/*
func (b *ChromeBackend) fetchURLsJS(urls []string) [][]byte {
	if len(urls) == 0 {
		return nil
	}

	urlsJSON, _ := json.Marshal(urls)

	js := fmt.Sprintf(`
		(async () => {
			const urls = %s;
			const results = await Promise.all(urls.map(async (url) => {
				if (!url) return "";
				try {
					const r = await fetch(url);
					const buf = await r.arrayBuffer();
					const bytes = new Uint8Array(buf);
					let binary = '';
					for (let i = 0; i < bytes.length; i += 8192) {
						binary += String.fromCharCode(...bytes.subarray(i, i + 8192));
					}
					return btoa(binary);
				} catch(e) { return ""; }
			}));
			return JSON.stringify(results);
		})()
	`, string(urlsJSON))

	var result string
	if err := chromedp.Run(b.ctx, chromedp.Evaluate(js, &result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	})); err != nil {
		return make([][]byte, len(urls))
	}

	var b64s []string
	if err := json.Unmarshal([]byte(result), &b64s); err != nil {
		return make([][]byte, len(urls))
	}

	data := make([][]byte, len(urls))
	for i, s := range b64s {
		if s == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err == nil {
			data[i] = decoded
		}
	}
	return data
}
*/

// Download downloads a reel video and profile picture to the cache directory
func (b *ChromeBackend) Download(index int) (string, string, []FloatingPfpFile, error) {
	return b.DownloadContext(context.Background(), index)
}

// DownloadContext is Download with cancellation support for speculative
// prefetch. Files are only committed to the cache after every required video
// byte has arrived successfully.
func (b *ChromeBackend) DownloadContext(ctx context.Context, index int) (string, string, []FloatingPfpFile, error) {
	if err := ctx.Err(); err != nil {
		return "", "", nil, err
	}
	pk := b.activeCursor().PKAt(index)
	if pk == "" {
		return "", "", nil, fmt.Errorf("index out of range")
	}
	b.reelsMu.RLock()
	r, ok := b.reels[pk]
	if !ok {
		b.reelsMu.RUnlock()
		return "", "", nil, fmt.Errorf("reel pk=%s not in cache", pk)
	}
	reel := *r
	b.reelsMu.RUnlock()

	if reel.VideoURL == "" {
		return "", "", nil, fmt.Errorf("no video URL")
	}

	videoFile := filepath.Join(b.cacheDir, fmt.Sprintf("%03d_%s.mp4", index, reel.Code))
	pfpFile := filepath.Join(b.cacheDir, fmt.Sprintf("%03d_%s_pfp.jpg", index, reel.Code))

	floatingPfpPaths := make([]FloatingPfpFile, len(reel.FloatingContextItems))
	for i, item := range reel.FloatingContextItems {
		floatingPfpPaths[i].Type = item.Type
		if item.ProfilePicUrl == "" {
			continue
		}
		floatingPfpPaths[i].Path = filepath.Join(b.cacheDir, fmt.Sprintf("%03d_%s_fc%d.jpg", index, reel.Code, i))
	}

	// check cache to see if already downloaded
	if videoCache.has(videoFile) {
		return videoFile, pfpFile, floatingPfpPaths, nil
	}

	cacheMu.Lock()
	// re-check under lock
	if videoCache.has(videoFile) {
		cacheMu.Unlock()
		return videoFile, pfpFile, floatingPfpPaths, nil
	}

	// check if in the progress of being downloaded
	if ch, ok := inProgress[videoFile]; ok {
		cacheMu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return "", "", nil, ctx.Err()
		}
		if videoCache.has(videoFile) {
			return videoFile, pfpFile, floatingPfpPaths, nil
		}
		cacheMu.Lock()
	}

	// Mark as in progress
	done := make(chan struct{})
	inProgress[videoFile] = done
	cacheMu.Unlock()
	// cleanup: remove from inProgress and signal waiters when done
	defer func() {
		cacheMu.Lock()
		delete(inProgress, videoFile)
		cacheMu.Unlock()
		close(done)
	}()

	// Download video, creator pfp, and any floating-context pfps in parallel.
	// urls[0] is video, urls[1] is creator pfp (if present), then floating pfps.
	urls := []string{reel.VideoURL}
	hasCreatorPfp := reel.ProfilePicUrl != ""
	if hasCreatorPfp {
		urls = append(urls, reel.ProfilePicUrl)
	}
	floatingStart := len(urls)
	floatingIdx := make([]int, 0, len(reel.FloatingContextItems))
	for i, item := range reel.FloatingContextItems {
		if item.ProfilePicUrl == "" {
			continue
		}
		urls = append(urls, item.ProfilePicUrl)
		floatingIdx = append(floatingIdx, i)
	}

	data := fetchURLsHTTPContext(ctx, urls)
	if err := ctx.Err(); err != nil {
		return "", "", nil, err
	}
	if data[0] == nil {
		return "", "", nil, fmt.Errorf("failed to download video")
	}

	if err := os.WriteFile(videoFile, data[0], 0644); err != nil {
		return "", "", nil, err
	}
	videoCache.add(videoFile)

	if hasCreatorPfp && len(data) > 1 && data[1] != nil {
		b.cacheReelPfp(fmt.Sprintf("%03d_%s_pfp.jpg", index, reel.Code), data[1])
	}

	for k, i := range floatingIdx {
		d := data[floatingStart+k]
		if d == nil {
			continue
		}
		b.cacheReelPfp(fmt.Sprintf("%03d_%s_fc%d.jpg", index, reel.Code, i), d)
	}

	return videoFile, pfpFile, floatingPfpPaths, nil
}
