package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// A comment's GIF is drawn into blank rows View leaves for it, at a column
// View never sees. commentLayoutFor is what keeps the two in step, so its
// gifCol has to match where the comment's text actually starts.
func TestCommentGifColumnMatchesRenderedText(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	const width = 44

	for _, isReply := range []bool{false, true} {
		cp := NewCommentsPanel()
		cp.Open("pk")
		comments := []backend.Comment{{PK: "1", Username: "top", Text: "parent text"}}
		if isReply {
			comments = append(comments, backend.Comment{
				PK: "2", Username: "child", Text: "reply text", ParentCommentID: "1",
			})
		}
		cp.SetComments("pk", comments)

		lines := strings.Split(strings.TrimSuffix(cp.View(width, 16, ""), "\n"), "\n")

		want := commentLayoutFor(width, isReply).gifCol
		needle := "parent text"
		if isReply {
			needle = "reply text"
		}

		found := false
		for _, line := range lines {
			if !strings.Contains(line, needle) {
				continue
			}
			found = true
			// The prefix contains the cursor bar and reply guide, not just
			// spaces, so measure it in display columns.
			indent := lipgloss.Width(line[:strings.Index(line, needle)])
			if indent != want {
				t.Errorf("isReply=%v: text drawn at column %d, gifCol says %d\n  %q",
					isReply, indent, want, line)
			}
		}
		if !found {
			t.Errorf("isReply=%v: never rendered %q", isReply, needle)
		}
	}
}

// The cursor bar must not change how much room the text has, or the panel
// would reflow every time the cursor moved.
func TestCommentGutterIsConstantWidth(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	on := commentGutter(true)
	off := commentGutter(false)
	if lipgloss.Width(on) != lipgloss.Width(off) {
		t.Errorf("cursor gutter is %d columns, idle is %d", lipgloss.Width(on), lipgloss.Width(off))
	}
	if lipgloss.Width(off) != commentGutterCols {
		t.Errorf("gutter is %d columns, want %d", lipgloss.Width(off), commentGutterCols)
	}
	if !strings.Contains(on, "▌") {
		t.Errorf("cursor gutter has no bar: %q", on)
	}
}

// An empty list must say so rather than rendering nothing, which looks like
// the panel failed to open.
func TestEmptyCommentsPanelSaysSo(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	cp := NewCommentsPanel()
	cp.Open("pk")

	if out := cp.View(40, 10, ""); !strings.Contains(out, "No comments yet") {
		t.Errorf("empty panel rendered %q", out)
	}

	cp.SetLoading(true)
	if out := cp.View(40, 10, ""); !strings.Contains(out, "━") {
		t.Errorf("loading panel has no skeleton rows: %q", out)
	}
}

// The heart's recorded hit box has to match where it is drawn, or clicking a
// comment's like does nothing.
func TestCommentHeartHitBoxMatchesRenderedPosition(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	cp := NewCommentsPanel()
	cp.Open("pk")
	cp.SetComments("pk", []backend.Comment{
		{PK: "1", Username: "ayse", Text: "first", CommentLikeCount: 12},
		{PK: "2", Username: "mehmet", Text: "second"},
	})

	lines := strings.Split(strings.TrimSuffix(cp.View(44, 16, ""), "\n"), "\n")

	if len(cp.hearts) != 2 {
		t.Fatalf("recorded %d hit boxes, want 2", len(cp.hearts))
	}

	for _, h := range cp.hearts {
		// hearts[].row counts from the first content line, which is rendered
		// line 1 (line 0 is the header).
		line := lines[h.row+1]
		col := lipgloss.Width(line[:strings.Index(line, "♥")])
		if col != h.start {
			t.Errorf("comment %d: heart drawn at column %d, recorded at %d\n  %q",
				h.index, col, h.start, line)
		}
		if got := cp.HeartAt(h.row, h.start); got != h.index {
			t.Errorf("comment %d: HeartAt on its own box returned %d", h.index, got)
		}
		if got := cp.HeartAt(h.row, h.start-1); got == h.index {
			t.Errorf("comment %d: column left of the heart was a hit", h.index)
		}
		if got := cp.HeartAt(h.row, h.start+h.width); got == h.index {
			t.Errorf("comment %d: column right of the heart was a hit", h.index)
		}
	}
}

func TestReplyCommentHasIndependentHeartHitBox(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	cp := NewCommentsPanel()
	cp.Open("reel")
	cp.SetComments("reel", []backend.Comment{
		{PK: "parent", Username: "parent", Text: "top"},
		{PK: "reply", Username: "child", Text: "nested", ParentCommentID: "parent"},
	})
	cp.View(40, 14, "")

	var replyHit *commentHeartHit
	for i := range cp.hearts {
		if cp.hearts[i].index == 1 {
			replyHit = &cp.hearts[i]
			break
		}
	}
	if replyHit == nil {
		t.Fatal("reply comment did not receive a heart hitbox")
	}
	if got := cp.HeartAt(replyHit.row, replyHit.start); got != 1 {
		t.Errorf("reply heart resolved to comment %d, want 1", got)
	}

	cp.SetCommentLiked(1, true)
	reply, _ := cp.CommentAt(1)
	parent, _ := cp.CommentAt(0)
	if !reply.HasLikedComment || reply.CommentLikeCount != 1 {
		t.Errorf("reply like was not applied independently: %+v", reply)
	}
	if parent.HasLikedComment || parent.CommentLikeCount != 0 {
		t.Errorf("reply like changed its parent: %+v", parent)
	}
}

func TestLongCommentHeaderStaysInsideNarrowCard(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	const width = 24
	cp := NewCommentsPanel()
	cp.Open("pk")
	cp.SetComments("pk", []backend.Comment{{
		PK:               "1",
		Username:         "a_very_long_username_that_must_not_resize_the_card",
		Text:             "body",
		IsVerified:       true,
		CreatedAt:        time.Now().Add(-8 * 24 * time.Hour).Unix(),
		CommentLikeCount: 9876543,
	}})

	lines := strings.Split(strings.TrimSuffix(cp.View(width, 10, ""), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("comment header missing: %q", lines)
	}
	if got := lipgloss.Width(lines[1]); got > width {
		t.Errorf("header expanded narrow card to %d columns, want <= %d: %q", got, width, lines[1])
	}
	if !strings.Contains(lines[1], "♥") {
		t.Errorf("heart was sacrificed instead of truncating metadata: %q", lines[1])
	}
}

// An optimistic like has to move the count, and rolling it back has to undo
// both halves.
func TestSetCommentLikedIsReversible(t *testing.T) {
	cp := NewCommentsPanel()
	cp.Open("pk")
	cp.SetComments("pk", []backend.Comment{{PK: "1", Username: "a", Text: "t", CommentLikeCount: 5}})

	cp.SetCommentLiked(0, true)
	c, _ := cp.CommentAt(0)
	if !c.HasLikedComment || c.CommentLikeCount != 6 {
		t.Fatalf("after liking: liked=%v count=%d, want true/6", c.HasLikedComment, c.CommentLikeCount)
	}

	cp.SetCommentLiked(0, false)
	c, _ = cp.CommentAt(0)
	if c.HasLikedComment || c.CommentLikeCount != 5 {
		t.Errorf("after rollback: liked=%v count=%d, want false/5", c.HasLikedComment, c.CommentLikeCount)
	}

	// Setting the state it's already in must not double-count.
	cp.SetCommentLiked(0, false)
	c, _ = cp.CommentAt(0)
	if c.CommentLikeCount != 5 {
		t.Errorf("redundant rollback changed the count to %d", c.CommentLikeCount)
	}
}

func TestCommentLikeRollbackFollowsPKAfterRepliesAreInserted(t *testing.T) {
	cp := NewCommentsPanel()
	cp.Open("reel")
	cp.SetComments("reel", []backend.Comment{
		{PK: "parent", Username: "p", Text: "parent"},
		{PK: "target", Username: "t", Text: "target", HasLikedComment: true, CommentLikeCount: 1},
	})

	// Simulate replies arriving while the like request is in flight. The
	// target moves from index 1 to index 2.
	cp.SetComments("reel", []backend.Comment{
		{PK: "parent", Username: "p", Text: "parent"},
		{PK: "reply", Username: "r", Text: "reply", ParentCommentID: "parent"},
		{PK: "target", Username: "t", Text: "target", HasLikedComment: true, CommentLikeCount: 1},
	})
	cp.SetCommentLikedByPK("target", false)

	target, _ := cp.CommentAt(2)
	if target.HasLikedComment || target.CommentLikeCount != 0 {
		t.Errorf("target was not rolled back by PK: liked=%v count=%d",
			target.HasLikedComment, target.CommentLikeCount)
	}
	reply, _ := cp.CommentAt(1)
	if reply.HasLikedComment || reply.CommentLikeCount != 0 {
		t.Error("rollback changed the newly inserted reply")
	}
}

// Every branch glyph is the same width, so switching between them can never
// reflow a reply's text or move its GIF.
func TestReplyBranchGlyphsAreConstantWidth(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	for _, isLast := range []bool{false, true} {
		if got := lipgloss.Width(replyUserIndent(isLast)); got != replyBranchCols {
			t.Errorf("replyUserIndent(isLast=%v) is %d columns, want %d", isLast, got, replyBranchCols)
		}
		if got := lipgloss.Width(replyTextIndent(isLast)); got != replyBranchCols {
			t.Errorf("replyTextIndent(isLast=%v) is %d columns, want %d", isLast, got, replyBranchCols)
		}
	}
}

// A reply group ends with an elbow and the ones before it get a tee, which is
// what makes the thread read as a tree.
func TestReplyThreadDrawsATree(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	cp := NewCommentsPanel()
	cp.Open("pk")
	cp.SetComments("pk", []backend.Comment{
		{PK: "1", Username: "parent", Text: "top"},
		{PK: "1a", Username: "first", Text: "one", ParentCommentID: "1"},
		{PK: "1b", Username: "last", Text: "two", ParentCommentID: "1"},
		{PK: "2", Username: "other", Text: "unrelated"},
	})

	if cp.isLastReply(1) {
		t.Error("the first of two replies was treated as the last")
	}
	if !cp.isLastReply(2) {
		t.Error("the final reply was not treated as the last")
	}
	if cp.isLastReply(0) || cp.isLastReply(3) {
		t.Error("a top-level comment was treated as a reply")
	}

	out := cp.View(46, 18, "")
	if !strings.Contains(out, "├─ @first") {
		t.Errorf("first reply has no tee:\n%s", out)
	}
	if !strings.Contains(out, "╰─ @last") {
		t.Errorf("last reply has no elbow:\n%s", out)
	}
	// The comment after the group is top-level and must not be branched.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "@other") && (strings.Contains(line, "├") || strings.Contains(line, "╰")) {
			t.Errorf("a top-level comment was branched: %q", line)
		}
	}
}

// A GIF reply still occupies several text rows. The branch rule must continue
// down those reserved rows or the following sibling looks detached.
func TestGifReplyKeepsThreadBranchContinuous(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	cp := NewCommentsPanel()
	cp.gifCellHeight = 3
	cp.Open("pk")
	cp.SetComments("pk", []backend.Comment{
		{PK: "parent", Username: "parent", Text: "top"},
		{PK: "gif", Username: "gif-user", ParentCommentID: "parent", GifPath: "cached.gif"},
		{PK: "text", Username: "next", Text: "sibling", ParentCommentID: "parent"},
	})
	// Loading a real GIF needs terminal pixel metrics; the renderer only needs
	// map membership to reserve its rows.
	cp.gifAnims = map[string]*player.GifAnimation{"gif": {}}

	lines := strings.Split(strings.TrimSuffix(cp.View(46, 20, ""), "\n"), "\n")
	userLine := -1
	for i, line := range lines {
		if strings.Contains(line, "@gif-user") {
			userLine = i
			break
		}
	}
	if userLine < 0 {
		t.Fatal("GIF reply username was not rendered")
	}
	for i := 1; i <= cp.gifCellHeight; i++ {
		if !strings.Contains(lines[userLine+i], "│") {
			t.Errorf("GIF row %d broke the reply branch: %q", i, lines[userLine+i])
		}
	}
}

// A reply's wrapped text lines sit under its author, carrying the rule while
// more replies follow.
func TestReplyContinuationCarriesTheRule(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(restore)

	cp := NewCommentsPanel()
	cp.Open("pk")
	cp.SetComments("pk", []backend.Comment{
		{PK: "1", Username: "parent", Text: "top"},
		{PK: "1a", Username: "wrapper", Text: strings.Repeat("word ", 20), ParentCommentID: "1"},
		{PK: "1b", Username: "last", Text: "short", ParentCommentID: "1"},
	})

	lines := strings.Split(cp.View(40, 20, ""), "\n")

	var wrapped []string
	seen := false
	for _, line := range lines {
		if strings.Contains(line, "@wrapper") {
			seen = true
			continue
		}
		if seen && strings.Contains(line, "@last") {
			break
		}
		if seen && strings.TrimSpace(line) != "" {
			wrapped = append(wrapped, line)
		}
	}

	if len(wrapped) < 2 {
		t.Fatalf("expected the reply to wrap over several lines, got %d", len(wrapped))
	}
	for _, line := range wrapped {
		if !strings.HasPrefix(strings.TrimLeft(line, " "), "│") {
			t.Errorf("continuation line does not carry the rule: %q", line)
		}
	}
}
