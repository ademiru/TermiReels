package tui

import (
	"fmt"
	"strings"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/charmbracelet/lipgloss"
)

// CommentsPanel encapsulates the comments UI state and rendering
type CommentsPanel struct {
	// Display state
	isOpen   bool
	comments []backend.Comment
	cursor   int  // which comment is highlighted
	scroll   int  // first visible comment index
	loading  bool // true while fetching more comments

	// Which reel these comments belong to
	reelPK string

	// Panel dimensions
	width  int
	height int

	// GIF state
	gifAnims      map[string]*player.GifAnimation
	gifCellHeight int

	// hearts records where each visible comment's like control was drawn,
	// rebuilt on every View so a click can be mapped back to a comment
	hearts []commentHeartHit
}

// NewCommentsPanel creates a new CommentsPanel instance
func NewCommentsPanel() *CommentsPanel {
	return &CommentsPanel{
		comments:      make([]backend.Comment, 0),
		gifCellHeight: backend.GetSettings().GifCellHeight,
	}
}

// IsOpen returns whether the comments panel is open
func (cp *CommentsPanel) IsOpen() bool {
	return cp.isOpen
}

// Open opens the comments panel for the given reel
func (cp *CommentsPanel) Open(reelPK string) {
	cp.isOpen = true
	cp.cursor = 0
	cp.scroll = 0

	// If opening a different reel, clear comments
	// If reopening same reel, preserve cached comments
	if cp.reelPK != reelPK {
		cp.comments = make([]backend.Comment, 0)
		cp.gifAnims = nil
	}

	cp.reelPK = reelPK
}

// Close closes the comments panel
// Preserves reelPK and comments for potential reopening
func (cp *CommentsPanel) Close() {
	cp.isOpen = false
	cp.cursor = 0
	cp.scroll = 0
	// Note: we intentionally keep reelPK, comments, and gifAnims
	// so they can be restored if the user reopens for the same reel
}

// Clear clears all comments state (call when changing reels)
func (cp *CommentsPanel) Clear() {
	cp.isOpen = false
	cp.comments = make([]backend.Comment, 0)
	cp.cursor = 0
	cp.scroll = 0
	cp.reelPK = ""
	cp.gifAnims = nil
}

// loadGifs loads GIF animations from disk for comments that have a GifPath
func (cp *CommentsPanel) loadGifs() {
	if cp.gifAnims == nil {
		cp.gifAnims = make(map[string]*player.GifAnimation)
	}

	_, rows, _, termH, err := player.GetTerminalSize()
	if err != nil || rows == 0 || termH == 0 {
		return
	}
	cellH := termH / rows
	gifHeightPx := cp.gifCellHeight * cellH

	for _, c := range cp.comments {
		if c.GifPath == "" {
			continue
		}
		if _, ok := cp.gifAnims[c.PK]; ok {
			continue
		}
		anim, err := player.LoadGif(c.GifPath, gifHeightPx)
		if err != nil {
			continue
		}
		cp.gifAnims[c.PK] = anim
	}
}

// ResizeGifs re-decodes cached comment GIFs for the current terminal cell size.
func (cp *CommentsPanel) ResizeGifs() {
	if !cp.isOpen || len(cp.comments) == 0 {
		return
	}
	cp.gifAnims = nil
	cp.loadGifs()
}

// commentLines returns how many terminal lines comment i occupies: one line for
// the username plus either the reserved GIF rows or the wrapped text lines.
func (cp *CommentsPanel) commentLines(i int) int {
	comment := cp.comments[i]
	lines := 1 // username
	if _, ok := cp.gifAnims[comment.PK]; ok {
		lines += cp.gifCellHeight
	} else {
		layout := commentLayoutFor(cp.width, comment.ParentCommentID != "")
		lines += len(wrapByWidth(strings.ReplaceAll(comment.Text, "\n", " "), layout.wrapWidth))
	}
	if cp.showsReplyHint(i) {
		lines++ // "↳ N replies" hint
	}
	return lines
}

// firstFullyVisible returns the smallest scroll index such that comment `end` is
// the last comment fully visible in the panel. Walking up from `end`, we stop
// before the comment that would overflow, so the panel never leaves empty space
// below `end`.
func (cp *CommentsPanel) firstFullyVisible(end int) int {
	availableLines := cp.height - 2
	if availableLines < 1 || len(cp.comments) == 0 {
		return 0
	}

	lines := 0
	for i := end; i >= 0; i-- {
		lines += cp.commentLines(i)
		if lines == availableLines {
			return i
		}
		if lines > availableLines {
			return i + 1
		}
	}
	return 0
}

// MoveCursor moves the cursor by delta, auto-scrolling to keep it fully visible.
func (cp *CommentsPanel) MoveCursor(delta int) {
	if len(cp.comments) == 0 {
		return
	}
	cp.cursor += delta

	cp.clampCursor()
	cp.clampScroll()
}

// SetComments sets the comments to display
// Returns true if the comments were accepted (belong to current reel)
func (cp *CommentsPanel) SetComments(reelPK string, comments []backend.Comment) bool {
	if !cp.isOpen || cp.reelPK != reelPK {
		return false
	}

	var cursorPK, scrollPK string
	if len(cp.comments) > 0 {
		cursorPK = cp.comments[cp.cursor].PK
		scrollPK = cp.comments[cp.scroll].PK
	}

	cp.comments = comments
	cp.loadGifs()

	// Follow each anchor to its new position.
	if i, ok := indexOfPK(comments, cursorPK); ok {
		cp.cursor = i
	}
	if i, ok := indexOfPK(comments, scrollPK); ok {
		cp.scroll = i
	}

	cp.clampCursor()
	cp.clampScroll()

	return true
}

// indexOfPK returns the index of the comment with the given PK and whether it
// was found.
func indexOfPK(comments []backend.Comment, pk string) (int, bool) {
	if pk == "" {
		return 0, false
	}
	for i := range comments {
		if comments[i].PK == pk {
			return i, true
		}
	}
	return 0, false
}

// clampCursor pulls cursor into [0, len-1], or 0 when there are no comments.
func (cp *CommentsPanel) clampCursor() {
	if cp.cursor > len(cp.comments)-1 {
		cp.cursor = len(cp.comments) - 1
	}
	if cp.cursor < 0 {
		cp.cursor = 0
	}
}

// clampScroll pulls scroll into [firstFullyVisible(cursor), cursor] so the
// cursor's comment is always fully on screen.
func (cp *CommentsPanel) clampScroll() {
	if cp.cursor < cp.scroll {
		cp.scroll = cp.cursor
	}
	if minScroll := cp.firstFullyVisible(cp.cursor); cp.scroll < minScroll {
		cp.scroll = minScroll
	}
}

// CursorIndex returns the index of the comment currently under the cursor.
func (cp *CommentsPanel) CursorIndex() int {
	return cp.cursor
}

// CursorComment returns the comment currently under the cursor, or false if the
// list is empty.
func (cp *CommentsPanel) CursorComment() (backend.Comment, bool) {
	if cp.cursor < 0 || cp.cursor >= len(cp.comments) {
		return backend.Comment{}, false
	}
	return cp.comments[cp.cursor], true
}

// RepliesLoaded reports whether the given parent comment's replies are currently
// spliced into the list.
func (cp *CommentsPanel) RepliesLoaded(parentPK string) bool {
	for i := range cp.comments {
		if cp.comments[i].ParentCommentID == parentPK {
			return true
		}
	}
	return false
}

// showsReplyHint reports whether comment i should render a "↳ N replies" hint:
// it's a top-level comment with replies that haven't been loaded yet. Loaded
// replies are always contiguous right after their parent.
func (cp *CommentsPanel) showsReplyHint(i int) bool {
	c := cp.comments[i]
	if c.ParentCommentID != "" || c.ChildCommentCount == 0 {
		return false
	}
	if i+1 < len(cp.comments) && cp.comments[i+1].ParentCommentID == c.PK {
		return false
	}
	return true
}

// commentGutterCols is the width of the cursor bar drawn to the left of every
// line of a comment.
const commentGutterCols = 2

// commentHeartHit records where one comment's like control was drawn, so a
// click can be resolved back to the comment. row counts from the panel's first
// content line; start and width are columns from the panel's left edge.
type commentHeartHit struct {
	index int
	row   int
	start int
	width int
}

// HeartAt maps a click inside the panel to the comment whose like control it
// landed on, or -1. row counts from the first content line, col from the
// panel's left edge.
func (cp *CommentsPanel) HeartAt(row, col int) int {
	for _, h := range cp.hearts {
		if h.row == row && col >= h.start && col < h.start+h.width {
			return h.index
		}
	}
	return -1
}

// CommentAt returns the index of the comment stored at the given hit-box index.
func (cp *CommentsPanel) CommentAt(i int) (backend.Comment, bool) {
	if i < 0 || i >= len(cp.comments) {
		return backend.Comment{}, false
	}
	return cp.comments[i], true
}

// SetCommentLiked flips a comment's like state optimistically, the way the
// reel's own like does, so the UI answers immediately.
func (cp *CommentsPanel) SetCommentLiked(i int, liked bool) {
	if i < 0 || i >= len(cp.comments) {
		return
	}
	c := &cp.comments[i]
	if c.HasLikedComment == liked {
		return
	}
	c.HasLikedComment = liked
	if liked {
		c.CommentLikeCount++
	} else if c.CommentLikeCount > 0 {
		c.CommentLikeCount--
	}
}

// SetCommentLikedByPK updates a comment after an asynchronous backend result.
// Replies can be inserted while that request is in flight, so an old numeric
// index is not a safe identity for rollback.
func (cp *CommentsPanel) SetCommentLikedByPK(pk string, liked bool) {
	if i, ok := indexOfPK(cp.comments, pk); ok {
		cp.SetCommentLiked(i, liked)
	}
}

// commentLayout describes how one comment's lines are laid out: the indent for
// its username line, for its text lines, the width text wraps at, and the
// column a GIF is drawn at, counted from the panel's left edge.
//
// View and VisibleGifSlots both derive from this. They used to compute indents
// separately, so changing one shifted the blank rows View reserves for a GIF
// out from under the GIF itself.
type commentLayout struct {
	// userCols and textCols are how many columns sit before a comment's
	// username line and its text lines. They are widths, not strings, because
	// a reply's indent is drawn as branch glyphs whose shape depends on
	// whether it is the last of its group — but never its width.
	userCols  int
	textCols  int
	wrapWidth int
	gifCol    int
}

// replyBranchCols is the width of a reply's branch indent: the corner glyph,
// a dash, and a space.
const replyBranchCols = 3

func commentLayoutFor(width int, isReply bool) commentLayout {
	if isReply {
		return commentLayout{
			userCols:  replyBranchCols,
			textCols:  replyBranchCols,
			wrapWidth: width - replyBranchCols - commentGutterCols,
			gifCol:    commentGutterCols + replyBranchCols,
		}
	}
	return commentLayout{
		userCols:  0,
		textCols:  2,
		wrapWidth: width - 2 - commentGutterCols,
		gifCol:    commentGutterCols + 2,
	}
}

// commentGutter renders the bar that marks the comment under the cursor. It is
// the same width whether or not it's drawn, so nothing shifts as the cursor
// moves.
func commentGutter(atCursor bool) string {
	if atCursor {
		return yellow500.Render("▌ ")
	}
	return strings.Repeat(" ", commentGutterCols)
}

// Reply threading
//
// Replies are drawn as a tree: a tee for one with siblings below it, an elbow
// for the last of a group, and a trailing rule under everything but the last.
// All three are replyBranchCols wide, so switching between them never reflows
// the text or moves a GIF.

// replyUserIndent is the branch drawn beside a reply's author line.
func replyUserIndent(isLast bool) string {
	if isLast {
		return gray700.Render("╰─") + " "
	}
	return gray700.Render("├─") + " "
}

// replyTextIndent is what continues under a reply's author line: a rule while
// more replies follow, blank once the group has ended.
func replyTextIndent(isLast bool) string {
	if isLast {
		return strings.Repeat(" ", replyBranchCols)
	}
	return gray700.Render("│") + strings.Repeat(" ", replyBranchCols-1)
}

// isLastReply reports whether comment i is the final reply of its parent.
// Replies are spliced in contiguously right after the comment they answer, so
// the group ends as soon as the next comment has a different parent.
func (cp *CommentsPanel) isLastReply(i int) bool {
	parent := cp.comments[i].ParentCommentID
	if parent == "" {
		return false
	}
	next := i + 1
	return next >= len(cp.comments) || cp.comments[next].ParentCommentID != parent
}

// replyHintText renders the "↳ N replies" hint label for a parent comment.
func replyHintText(n int) string {
	if n == 1 {
		return "↳ 1 reply"
	}
	return fmt.Sprintf("↳ %d replies", n)
}

// View renders the comments panel
// width: available width in characters
// height: available height in lines
// padding: left padding string for alignment
//
// Renders TUI text for the comments section. Reserves space for gifs, which are handled separately
func (cp *CommentsPanel) View(width, height int, padding string) string {
	if !cp.isOpen {
		return ""
	}

	if len(cp.comments) == 0 {
		var b strings.Builder
		b.WriteString(padding + purple400.Bold(true).Render("Comments") + "\n")
		if cp.loading {
			// A small skeleton keeps the card visually stable and communicates
			// progress without a noisy spinner.
			barWidth := max(min(width-8, 18), 4)
			for i := 0; i < min(max(height-2, 1), 4); i++ {
				w := max(barWidth-i*3, 4)
				b.WriteString(padding + gray800.Render(strings.Repeat("━", w)) + "\n")
			}
		} else {
			b.WriteString(padding + gray500.Render("No comments yet") + "\n")
		}
		return b.String()
	}

	cp.width = width
	cp.height = height
	cp.hearts = cp.hearts[:0]

	var b strings.Builder

	// Header shows both position and total. The fixed-width card no longer
	// feels like an unlabelled slice of a longer list.
	header := purple400.Bold(true).Render("Comments")
	position := min(cp.cursor+1, len(cp.comments))
	header += gray600.Render(fmt.Sprintf("  %d/%d", position, len(cp.comments)))
	header += gray800.Render("  ↑↓")
	if cp.loading {
		header += yellow500.Render("  …")
	}
	b.WriteString(padding + header + "\n")
	availableLines := height - 2
	if availableLines < 1 {
		availableLines = 0
	}

	// Render comments starting from scroll position
	linesUsed := 0
	for i := cp.scroll; i < len(cp.comments) && linesUsed < availableLines; i++ {
		comment := cp.comments[i]
		isReply := comment.ParentCommentID != ""
		layout := commentLayoutFor(width, isReply)
		gutter := commentGutter(i == cp.cursor)

		userIndent := gutter + strings.Repeat(" ", layout.userCols)
		textIndent := gutter + strings.Repeat(" ", layout.textCols)
		if isReply {
			last := cp.isLastReply(i)
			userIndent = gutter + replyUserIndent(last)
			textIndent = gutter + replyTextIndent(last)
		}
		wrapWidth := layout.wrapWidth

		// The heart is drawn on every comment, liked or not, so there is always
		// something to click. Build it first so the username and metadata can
		// yield space to it instead of expanding the fixed side card.
		heartLabel := "♥"
		if comment.CommentLikeCount > 0 {
			heartLabel += " " + formatLikeCount(comment.CommentLikeCount)
		}
		heartWidth := lipgloss.Width(heartLabel)

		lineWidth := max(width-commentGutterCols-layout.userCols, 1)
		separator := "  "
		usernameBudget := max(lineWidth-heartWidth-lipgloss.Width(separator), 1)

		badge := ""
		if comment.IsVerified {
			badge = " ✓"
		}
		age := ""
		if renderedAge := relativeTime(comment.CreatedAt); renderedAge != "" {
			age = "  " + renderedAge
		}

		// Age is useful but least important; the verified badge and identity
		// survive first when a narrow panel cannot fit everything.
		suffix := badge + age
		if lipgloss.Width(suffix) >= usernameBudget {
			age = ""
			suffix = badge
		}
		if lipgloss.Width(suffix) >= usernameBudget {
			badge = ""
			suffix = ""
		}
		nameBudget := max(usernameBudget-lipgloss.Width(suffix), 1)
		username := truncateByWidth("@"+comment.Username, nameBudget)

		usernameStyle := pink200.Bold(true)
		if i == cp.cursor {
			usernameStyle = yellow500.Bold(true).Underline(true)
		}
		userPart := usernameStyle.Render(username)
		if badge != "" {
			userPart += " " + blue500.Render("✓")
		}
		if age != "" {
			userPart += gray600.Render(age)
		}
		userPart += separator
		heartStart := commentGutterCols + layout.userCols + lipgloss.Width(userPart)
		if comment.HasLikedComment {
			userPart += red500.Render(heartLabel)
		} else {
			userPart += gray600.Render(heartLabel)
		}
		cp.hearts = append(cp.hearts, commentHeartHit{
			index: i,
			row:   linesUsed,
			start: heartStart,
			width: lipgloss.Width(heartLabel),
		})

		// For GIF comments, require room for username + full cp.gifCellHeight
		if _, ok := cp.gifAnims[comment.PK]; ok {
			if linesUsed+1+cp.gifCellHeight > availableLines {
				break
			}
		} else if linesUsed+1 > availableLines {
			break
		}

		// Write username
		b.WriteString(padding + userIndent + userPart + "\n")
		linesUsed++

		// GIF comment: reserve rows for the animation while continuing the
		// reply branch beside it. Completely blank rows used to cut a thread's
		// vertical rule in half whenever one of its replies was a GIF.
		if _, ok := cp.gifAnims[comment.PK]; ok {
			for range cp.gifCellHeight {
				b.WriteString(padding + textIndent + "\n")
				linesUsed++
			}
		} else {
			// Write comment text lines
			commentLines := wrapByWidth(strings.ReplaceAll(comment.Text, "\n", " "), wrapWidth)
			for _, line := range commentLines {
				if linesUsed >= availableLines {
					break
				}
				b.WriteString(padding + textIndent + renderWithMentions(line, gray50) + "\n")
				linesUsed++
			}
		}

		// Reply hint under a top-level comment whose replies aren't loaded yet
		if cp.showsReplyHint(i) && linesUsed < availableLines {
			hint := blue400.Render(replyHintText(comment.ChildCommentCount))
			if i == cp.cursor {
				hint += gray600.Render("  space to open")
			}
			b.WriteString(padding + textIndent + "  " + hint + "\n")
			linesUsed++
		}
	}

	return b.String()
}

// VisibleGifSlots computes GIF slots with absolute terminal cell positions.
// This simulates the View() layout logic, then computes the row and col positions
// for each gif that will fill in the blank space that View() leaves in for gif comments.
func (cp *CommentsPanel) VisibleGifSlots(width, height, baseRow, baseCol int) []player.GifSlot {
	if !cp.isOpen || len(cp.comments) == 0 || len(cp.gifAnims) == 0 {
		return nil
	}

	availableLines := height - 2
	if availableLines < 1 {
		return nil
	}

	var slots []player.GifSlot
	linesUsed := 0
	currentRow := baseRow + 1 // +1 for header line

	for i := cp.scroll; i < len(cp.comments) && linesUsed < availableLines; i++ {
		comment := cp.comments[i]

		// Same layout View used, so the blank rows it reserved line up with
		// where the GIF actually lands.
		layout := commentLayoutFor(width, comment.ParentCommentID != "")
		wrapWidth := layout.wrapWidth
		gifCol := baseCol + layout.gifCol

		// For GIF comments, require room for username + full cp.gifCellHeight
		if _, ok := cp.gifAnims[comment.PK]; ok {
			if linesUsed+1+cp.gifCellHeight > availableLines {
				break
			}
		} else if linesUsed+1 > availableLines {
			break
		}

		// Username takes 1 line
		linesUsed++
		currentRow++

		if anim, ok := cp.gifAnims[comment.PK]; ok {
			// GIF starts right under the username, indented under the text
			slots = append(slots, player.GifSlot{
				Anim: anim,
				Row:  currentRow,
				Col:  gifCol,
			})
			linesUsed += cp.gifCellHeight
			currentRow += cp.gifCellHeight
		} else {
			// Advance past text lines
			commentLines := wrapByWidth(strings.ReplaceAll(comment.Text, "\n", " "), wrapWidth)
			for range commentLines {
				if linesUsed >= availableLines {
					break
				}
				linesUsed++
				currentRow++
			}
		}

		// Reply hint occupies one line, matching View.
		if cp.showsReplyHint(i) && linesUsed < availableLines {
			linesUsed++
			currentRow++
		}
	}

	return slots
}

// SetLoading sets the loading state for the comments panel
func (cp *CommentsPanel) SetLoading(loading bool) {
	cp.loading = loading
}

// ShouldFetchMore returns true if the cursor is near the end of the loaded comments.
func (cp *CommentsPanel) ShouldFetchMore() bool {
	return len(cp.comments) > 0 && cp.cursor >= len(cp.comments)-5
}

// CanAccept returns true if the panel can accept comments for the given reel
func (cp *CommentsPanel) CanAccept(reelPK string) bool {
	return cp.isOpen && cp.reelPK == reelPK
}
