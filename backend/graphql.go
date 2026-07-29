package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const (
	initialCommentsDocID        = "37622913243966296"
	initialCommentsFriendlyName = "PolarisPostCommentsContainerQuery"

	paginationDocID        = "27544211905201475"
	paginationFriendlyName = "PolarisPostCommentsPaginationQuery"

	childCommentsDocID        = "37441967968750542"
	childCommentsFriendlyName = "PolarisPostChildCommentsQuery"

	clipsDocID        = "28115468621393196"
	clipsFriendlyName = "PolarisClipsTabDesktopPaginationQuery"

	reactionDocID        = "24374451552236906"
	reactionFriendlyName = "IGDirectReactionSendMutation"

	profileDocID        = "26672929172408668"
	profileFriendlyName = "PolarisProfilePageContentQuery"

	expectedAppID = "936619743392459"
)

type Endpoint = int

const (
	readEndpoint   Endpoint = iota // clips, comments
	mutateEndpoint                 // reel reactions
)

// reelMedia is the Media payload inside one clip edge.
type reelMedia struct {
	PK               string `json:"pk"`
	Code             string `json:"code"`
	HasLiked         bool   `json:"has_liked"`
	HasViewerSaved   bool   `json:"has_viewer_saved"`
	CommentsDisabled bool   `json:"comments_disabled"`
	LikeCount        int    `json:"like_count"`
	CommentCount     int    `json:"comment_count"`
	MediaRepostCount int    `json:"media_repost_count"`
	VideoVersions    []struct {
		URL string `json:"url"`
	} `json:"video_versions"`
	User struct {
		Username      string `json:"username"`
		IsVerified    bool   `json:"is_verified"`
		ProfilePicUrl string `json:"profile_pic_url"`
	} `json:"user"`
	ClipsMetadata struct {
		MusicInfo *struct {
			MusicAssetInfo struct {
				Title                    string `json:"title"`
				DisplayArtist            string `json:"display_artist"`
				CoverArtworkThumbnailUri string `json:"cover_artwork_thumbnail_uri"`
				IsExplicit               bool   `json:"is_explicit"`
			} `json:"music_asset_info"`
		} `json:"music_info"`
	} `json:"clips_metadata"`
	Caption *struct {
		Text string `json:"text"`
	} `json:"caption"`
	CanViewerReshare     bool `json:"can_viewer_reshare"`
	FloatingContextItems []struct {
		Type string `json:"floating_context_item_type"`
		User struct {
			Username      string `json:"username"`
			ProfilePicUrl string `json:"profile_pic_url"`
		} `json:"user"`
		MediaNote *struct {
			Text string `json:"text"`
		} `json:"media_note"`
		Comment *struct {
			Text string `json:"text"`
		} `json:"comment"`
	} `json:"floating_context_items"`
}

// reelResponse represents the xdt_api__v1__clips__home__connection_v2 GraphQL response structure
type reelResponse struct {
	Data struct {
		Connection struct {
			Edges []struct {
				Node struct {
					Media reelMedia `json:"media"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"xdt_api__v1__clips__home__connection_v2"`
	} `json:"data"`
}

func (r reelResponse) mediaByCode(code string) (reelMedia, bool) {
	for _, edge := range r.Data.Connection.Edges {
		media := edge.Node.Media
		if media.Code == code && media.PK != "" {
			return media, true
		}
	}
	return reelMedia{}, false
}

// extractReelMedia handles profile-Reels payloads whose root field changes
// independently from the home clips connection. Array order is retained.
func extractReelMedia(body string) []reelMedia {
	var root any
	if json.Unmarshal([]byte(body), &root) != nil {
		return nil
	}
	seen := make(map[string]struct{})
	var result []reelMedia
	var walk func(any)
	walk = func(value any) {
		switch node := value.(type) {
		case []any:
			for _, child := range node {
				walk(child)
			}
		case map[string]any:
			code, _ := node["code"].(string)
			pk, _ := node["pk"].(string)
			if code != "" && pk != "" {
				raw, err := json.Marshal(node)
				if err == nil {
					var media reelMedia
					if json.Unmarshal(raw, &media) == nil && media.Code != "" && media.PK != "" {
						if _, exists := seen[media.Code]; !exists {
							seen[media.Code] = struct{}{}
							result = append(result, media)
						}
					}
				}
				return
			}
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(root)
	return result
}

func (b *ChromeBackend) processAuthoritativeProfileResponse(body string, profile *ProfileCursor) int {
	count := 0
	for _, media := range extractReelMedia(body) {
		if media.User.Username == "" {
			if profile.captureCandidate(media.Code) {
				log.Printf(
					"creator profile media candidate: target=@%s code=%s",
					profile.Username(), media.Code,
				)
				count++
			}
			continue
		}
		if !strings.EqualFold(media.User.Username, profile.Username()) {
			log.Printf(
				"creator profile media rejected: target=@%s code=%s owner=@%s",
				profile.Username(), media.Code, media.User.Username,
			)
			continue
		}
		resolvedPK := ""
		if len(media.VideoVersions) > 0 {
			b.reelsMu.Lock()
			if _, exists := b.reels[media.PK]; !exists {
				b.reels[media.PK] = buildReel(media)
			}
			b.reelsMu.Unlock()
			resolvedPK = media.PK
		}
		if profile.captureAuthoritative(media.Code, resolvedPK) {
			log.Printf(
				"creator profile media accepted: target=@%s code=%s owner=@%s",
				profile.Username(), media.Code, media.User.Username,
			)
			count++
		}
	}
	return count
}

// buildReel converts a parsed reelMedia into our Reel domain type. It can be
// called from any path that has a reelMedia in hand.
func buildReel(media reelMedia) *Reel {
	var videoURL string
	if len(media.VideoVersions) > 0 {
		videoURL = strings.ReplaceAll(media.VideoVersions[0].URL, "\\u0026", "&")
	}

	caption := ""
	if media.Caption != nil {
		caption = media.Caption.Text
	}

	var music *MusicInfo
	if media.ClipsMetadata.MusicInfo != nil {
		info := media.ClipsMetadata.MusicInfo.MusicAssetInfo
		music = &MusicInfo{
			Title:      info.Title,
			Artist:     info.DisplayArtist,
			IsExplicit: info.IsExplicit,
		}
	}

	var floatingItems []FloatingContextItem
	for _, item := range media.FloatingContextItems {
		fi := FloatingContextItem{
			Type:          item.Type,
			Username:      item.User.Username,
			ProfilePicUrl: strings.ReplaceAll(item.User.ProfilePicUrl, "\\u0026", "&"),
		}
		if item.MediaNote != nil {
			fi.Text = item.MediaNote.Text
		} else if item.Comment != nil {
			fi.Text = item.Comment.Text
		}
		floatingItems = append(floatingItems, fi)
	}

	return &Reel{
		PK:                   media.PK,
		Code:                 media.Code,
		VideoURL:             videoURL,
		ProfilePicUrl:        media.User.ProfilePicUrl,
		Username:             media.User.Username,
		Caption:              caption,
		Liked:                media.HasLiked,
		Saved:                media.HasViewerSaved,
		LikeCount:            media.LikeCount,
		RepostCount:          media.MediaRepostCount,
		IsVerified:           media.User.IsVerified,
		CommentCount:         media.CommentCount,
		CommentsDisabled:     media.CommentsDisabled,
		Music:                music,
		CanViewerReshare:     media.CanViewerReshare,
		FloatingContextItems: floatingItems,
	}
}

// jsonStringForJS converts a Go string to a JS string literal
func jsonStringForJS(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// graphqlRequest describes one replay of a captured Instagram GraphQL request.
// The template is a previously captured x-www-form-urlencoded POST body that
// carries the session tokens (lsd, fb_dtsg, csrf, …); execGraphQL swaps in the
// doc_id / friendly name / variables and reuses everything else.
type graphQLRequest struct {
	ctx          context.Context // page context whose window runs the fetch
	template     string          // captured token-bearing urlencoded request body
	docID        string
	friendlyName string
	endpoint     string
	variables    any
	valid        bool // used by factory
}

func newGraphQLRequest(ctx context.Context, template string, docID string, friendlyName string, endpointEnum Endpoint, variables any) (graphQLRequest, error) {
	if template == "" {
		return graphQLRequest{valid: false}, fmt.Errorf("execGraphQL: empty request template")
	}

	req := graphQLRequest{
		ctx:          ctx,
		template:     template,
		docID:        docID,
		friendlyName: friendlyName,
		variables:    variables,
		valid:        true,
	}

	switch endpointEnum {
	case readEndpoint:
		req.endpoint = "https://www.instagram.com/graphql/query"
	case mutateEndpoint:
		req.endpoint = "https://www.instagram.com/api/graphql"
	default:
		return graphQLRequest{valid: false}, fmt.Errorf("Not a valid endpoint")
	}

	return req, nil
}

// execGraphQL replays a captured GraphQL request as an in-page fetch() so the
// browser attaches the real cookies/CSRF and the tokens in the template match a
// genuine client. The x-fb-lsd header is taken from the template's lsd param.
// Returns the raw response body.
func execGraphQL(req graphQLRequest) (string, error) {
	if req.valid == false {
		return "", fmt.Errorf("invalid graphQLRequest struct")
	}

	params, err := url.ParseQuery(req.template)
	if err != nil {
		return "", err
	}

	varsJSON, err := json.Marshal(req.variables)
	if err != nil {
		return "", err
	}

	params.Set("doc_id", req.docID)
	params.Set("fb_api_req_friendly_name", req.friendlyName)
	params.Set("variables", string(varsJSON))
	postBody := params.Encode()

	endpoint := req.endpoint

	js := fmt.Sprintf(`
		(async () => {
			const ac = new AbortController();
			const tid = setTimeout(() => ac.abort(), 10000);
			try {
				const csrftoken = document.cookie.split('; ')
					.find(c => c.startsWith('csrftoken='))
					?.split('=')[1] || '';
				const r = await fetch(%s, {
					method: "POST",
					headers: {
						"content-type": "application/x-www-form-urlencoded",
						"x-csrftoken": csrftoken,
						"x-fb-friendly-name": %s,
						"x-fb-lsd": %s,
						"x-ig-app-id": %s,
					},
					body: %s,
					credentials: "include",
					signal: ac.signal
				});
				return await r.text();
			} finally {
				clearTimeout(tid);
			}
		})()
	`, jsonStringForJS(endpoint), jsonStringForJS(req.friendlyName), jsonStringForJS(params.Get("lsd")), expectedAppID, jsonStringForJS(postBody))

	var result string
	err = chromedp.Run(req.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(js, &result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}).Do(ctx)
		}),
	)
	if err != nil {
		return "", err
	}
	return result, nil
}

// processReelResponse extracts reels from a GraphQL response. Reel storage is
// global, but source membership is not: profile reels go only to the active
// ProfileCursor and can never reorder or contaminate the main feed.
func (b *ChromeBackend) processReelResponse(body string, profile *ProfileCursor) {
	var resp reelResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return
	}

	for _, edge := range resp.Data.Connection.Edges {
		media := edge.Node.Media
		if media.PK == "" {
			continue
		}

		b.reelsMu.Lock()
		if _, exists := b.reels[media.PK]; !exists {
			b.reels[media.PK] = buildReel(media)
		}
		ownedProfileMedia := profile != nil &&
			strings.EqualFold(media.User.Username, profile.Username())
		if profile == nil || !ownedProfileMedia || !profile.capture(media.Code, media.PK) {
			// The main feed is the only fallback destination. A profile
			// payload whose code is not in its grid is recommendation context,
			// not part of that creator's reel sequence.
			if profile == nil && b.feed.indexOf(media.PK) == 0 {
				b.feed.append(media.PK)
			}
		}
		b.reelsMu.Unlock()
	}
}

// decodePostData reassembles the (base64-chunked) POST body of an intercepted
// request into a plain string.
func decodePostData(e *fetch.EventRequestPaused) string {
	var raw []byte
	for _, entry := range e.Request.PostDataEntries {
		if decoded, err := base64.StdEncoding.DecodeString(entry.Bytes); err == nil {
			raw = append(raw, decoded...)
		}
	}
	return string(raw)
}

// processFeedGraphQLBody is the fetch interception router for the dm browser.
func (b *ChromeBackend) processDMGraphQLBody(ctx context.Context, e *fetch.EventRequestPaused) {
	var body []byte
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			data, err := fetch.GetResponseBody(e.RequestID).Do(c)
			if err != nil {
				return err
			}
			body = data
			return nil
		}),
	)
	if err != nil {
		return
	}
	bodyStr := string(body)
	switch {
	case strings.Contains(bodyStr, "get_slide_thread_nullable"):
		b.dm.CaptureTemplate(decodePostData(e))
		b.processThreadResponse(bodyStr)

	case strings.Contains(bodyStr, "xdt_api__v1__media__media_id__comments__connection"):
		postData := decodePostData(e)
		// Skip pagination responses, FetchMoreComments handles those directly
		if !strings.Contains(postData, paginationFriendlyName) {
			b.processCommentsResponse(bodyStr, postData, e)
		}

	}
	chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return fetch.ContinueRequest(e.RequestID).Do(c)
		}),
	)
}

// processFeedGraphQLBody is the fetch interception router for the regular reel browser.
func (b *ChromeBackend) processFeedGraphQLBody(ctx context.Context, e *fetch.EventRequestPaused) {
	var body []byte
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			data, err := fetch.GetResponseBody(e.RequestID).Do(c)
			if err != nil {
				return err
			}
			body = data
			return nil
		}),
	)
	if err != nil {
		return
	}
	bodyStr := string(body)
	postData := decodePostData(e)
	b.modeMu.RLock()
	profile := b.profile
	profileCtx := b.profileCtx
	b.modeMu.RUnlock()
	if profileCtx != ctx {
		profile = nil
	}
	lowerPost := strings.ToLower(postData)
	lowerURL := strings.ToLower(e.Request.URL)
	targetedProfileReels := profile != nil &&
		(strings.Contains(lowerPost, "target_user_id") || strings.Contains(lowerURL, "target_user_id")) &&
		(strings.Contains(lowerPost, "clip") || strings.Contains(lowerURL, "clip") ||
			strings.Contains(lowerPost, "reel") || strings.Contains(lowerURL, "reel") ||
			strings.Contains(lowerPost, "include_feed_video") ||
			strings.Contains(lowerURL, "include_feed_video"))

	switch {
	case targetedProfileReels:
		if count := b.processAuthoritativeProfileResponse(bodyStr, profile); count > 0 {
			log.Printf("creator profile API captured: @%s %d reels", profile.Username(), count)
		}
	case strings.Contains(bodyStr, "xdt_api__v1__clips__home__connection_v2"):
		b.dm.CaptureTemplate(postData)
		if profile == nil {
			b.processReelResponse(bodyStr, profile)
		}
	case strings.Contains(bodyStr, "xdt_api__v1__media__media_id__comments__connection"):
		// Skip pagination responses, FetchMoreComments handles those directly
		if !strings.Contains(postData, paginationFriendlyName) {
			b.processCommentsResponse(bodyStr, postData, e)
		}
	}

	chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return fetch.ContinueRequest(e.RequestID).Do(c)
		}),
	)
}
