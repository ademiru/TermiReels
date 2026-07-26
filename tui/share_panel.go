package tui

import (
	"fmt"
	"strings"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/ademiru/TermiReels/tui/colors"
	"github.com/charmbracelet/lipgloss"
)

const sharePfpCellHeight = 3

// SharePanel encapsulates the share modal UI state and rendering
type SharePanel struct {
	isOpen   bool
	friends  []backend.User
	cursor   int // which friend is highlighted
	scroll   int // first visible friend index
	selected map[int]bool

	// Image state
	pfps map[int]*player.Img

	// cached for scroll calculations
	visibleCount int

	// column span of the send button on the header line, recorded by View so
	// a click can be resolved back to it
	sendStart int
	sendWidth int
}

// NewSharePanel creates a new SharePanel instance
func NewSharePanel() *SharePanel {
	return &SharePanel{}
}

// IsOpen returns whether the share panel is open
func (sp *SharePanel) IsOpen() bool {
	return sp.isOpen
}

// Open opens the share panel
func (sp *SharePanel) Open() {
	sp.isOpen = true
	sp.cursor = 0
	sp.scroll = 0
	sp.friends = nil
	sp.pfps = nil
	sp.selected = make(map[int]bool)
}

// Close closes the share panel
func (sp *SharePanel) Close() {
	sp.isOpen = false
	sp.cursor = 0
	sp.scroll = 0
	sp.friends = nil
	sp.pfps = nil
	sp.selected = nil
}

// SetFriends sets the friend list and loads their profile pics.
// Friends with any empty fields are filtered out.
func (sp *SharePanel) SetFriends(friends []backend.User) {
	filtered := friends[:0:0]
	for _, f := range friends {
		if f.Name != "" && f.ImgSrc != "" && f.ImgPath != "" {
			filtered = append(filtered, f)
		}
	}
	sp.friends = filtered
	sp.loadPfps()
}

// loadPfps loads profile pic images from disk
func (sp *SharePanel) loadPfps() {
	sp.pfps = make(map[int]*player.Img)

	for i, f := range sp.friends {
		if f.ImgPath == "" {
			continue
		}
		pfp, err := player.LoadPFP(f.ImgPath)
		if err != nil {
			continue
		}
		pfp.ResizeToCells(sharePfpCellHeight)
		sp.pfps[i] = pfp
	}
}

// ResizePfps re-scales loaded share panel pfps for the current terminal cell size.
func (sp *SharePanel) ResizePfps() {
	for _, pfp := range sp.pfps {
		pfp.ResizeToCells(sharePfpCellHeight)
	}
}

// MoveCursor moves the cursor by delta, auto-scrolling to keep cursor visible
func (sp *SharePanel) MoveCursor(delta int) {
	if len(sp.friends) == 0 {
		return
	}
	sp.cursor += delta
	if sp.cursor < 0 {
		sp.cursor = 0
	}
	if sp.cursor >= len(sp.friends) {
		sp.cursor = len(sp.friends) - 1
	}

	// Auto-scroll to keep cursor visible
	if sp.cursor < sp.scroll {
		sp.scroll = sp.cursor
	}
	if sp.visibleCount > 0 && sp.cursor >= sp.scroll+sp.visibleCount {
		sp.scroll = sp.cursor - sp.visibleCount + 1
	}
}

// CursorIndex returns the current cursor position
func (sp *SharePanel) CursorIndex() int {
	return sp.cursor
}

// ToggleSelected toggles the selected state of the friend at the cursor
func (sp *SharePanel) ToggleSelected() {
	if sp.selected == nil {
		sp.selected = make(map[int]bool)
	}
	if sp.selected[sp.cursor] {
		delete(sp.selected, sp.cursor)
	} else {
		sp.selected[sp.cursor] = true
	}
}

// SelectedCount returns how many friends are picked.
func (sp *SharePanel) SelectedCount() int {
	n := 0
	for _, on := range sp.selected {
		if on {
			n++
		}
	}
	return n
}

// SetCursor moves the cursor to i, scrolling it into view. Used when a friend
// is clicked rather than scrolled to.
func (sp *SharePanel) SetCursor(i int) {
	if i < 0 || i >= len(sp.friends) {
		return
	}
	sp.MoveCursor(i - sp.cursor)
}

// FriendCount returns how many friends are listed.
func (sp *SharePanel) FriendCount() int {
	return len(sp.friends)
}

// FriendAtRow maps a row offset from the panel's first friend row to a friend
// index, or -1 when the row is past the end of the list.
func (sp *SharePanel) FriendAtRow(offset int) int {
	if !sp.isOpen || offset < 0 {
		return -1
	}
	i := sp.scroll + offset/sharePfpCellHeight
	if i >= len(sp.friends) || (sp.visibleCount > 0 && i >= sp.scroll+sp.visibleCount) {
		return -1
	}
	return i
}

// View renders the share panel
// Each friend takes sharePfpCellHeight lines: pfp on left, name centered vertically on right
func (sp *SharePanel) View(width, height int, padding string) string {
	if !sp.isOpen {
		return ""
	}

	var b strings.Builder

	b.WriteString(padding + sp.header() + "\n")

	availableLines := height - 2
	if availableLines < 1 {
		return b.String()
	}

	if len(sp.friends) == 0 {
		b.WriteString(padding + gray500.Render("loading friends...") + "\n")
		return b.String()
	}

	pfpPadding := "        " // space for the pfp image (rendered separately)
	linesUsed := 0

	// Cache how many friends fit on screen
	sp.visibleCount = availableLines / sharePfpCellHeight

	for i := sp.scroll; i < len(sp.friends); i++ {
		if linesUsed+sharePfpCellHeight > availableLines {
			break
		}

		friend := sp.friends[i]
		selected := sp.selected[i]
		atCursor := i == sp.cursor

		// Render sharePfpCellHeight lines per friend
		// Name goes on the middle line (line 1 of 0,1,2), centered vertically
		for line := 0; line < sharePfpCellHeight; line++ {
			if line == sharePfpCellHeight/2 {
				// Names come from Instagram and can be long; clip them to the
				// panel so a narrow terminal doesn't wrap the row and break
				// the fixed rows-per-friend the pfp placement counts on.
				name := truncateByWidth(friend.Name, max(width-len(pfpPadding)-shareRowLabelCols, 1))
				b.WriteString(padding + pfpPadding + shareRowLabel(name, selected, atCursor) + "\n")
			} else {
				b.WriteString(padding + pfpPadding + "\n")
			}
			linesUsed++
		}
	}

	return b.String()
}

// header renders the panel's title, the running selection count, a send
// button, and the keys that do the same thing.
//
// It records where the button lands so SendButtonAt can resolve a click. The
// offsets are measured off a plain copy built alongside the styled one, since
// the styled string carries escape sequences.
func (sp *SharePanel) header() string {
	plain := "Share to"
	out := purple400.Bold(true).Render(plain)

	if n := sp.SelectedCount(); n > 0 {
		count := fmt.Sprintf("  %d selected", n)
		plain += count
		out += yellow300.Render(count)
	}

	const gap = "   "
	plain += gap
	out += gap

	// Nothing picked yet means nothing to send, so the button stays quiet
	// rather than inviting a click that only closes the panel.
	const label = "  SEND  "
	sp.sendStart = lipgloss.Width(plain)
	sp.sendWidth = lipgloss.Width(label)

	if sp.SelectedCount() > 0 {
		out += gradientOnBackground(label, brandRamp, colors.WhiteColor, true)
	} else {
		out += lipgloss.NewStyle().
			Foreground(colors.Gray600Color).
			Background(colors.Gray900Color).
			Render(label)
	}

	return out + gray600.Render("  space pick")
}

// SendButtonAt reports whether a column offset into the header line falls on
// the send button.
func (sp *SharePanel) SendButtonAt(offset int) bool {
	return sp.isOpen && sp.sendWidth > 0 &&
		offset >= sp.sendStart && offset < sp.sendStart+sp.sendWidth
}

// shareRowLabel renders one friend's line: a cursor bar, an explicit
// checkbox, and the name.
//
// Colour alone used to carry the selection, which is invisible if you aren't
// looking for it and unreadable to anyone who can't separate the two hues. The
// checkbox states the answer.
// shareRowLabelCols is what shareRowLabel costs before the name: the cursor
// bar, the checkbox, and the space after it.
const shareRowLabelCols = 4

func shareRowLabel(name string, selected, atCursor bool) string {
	bar := "  "
	if atCursor {
		bar = pink500.Render("▌ ")
	}

	box := gray600.Render("○")
	if selected {
		box = yellow300.Render("●")
	}

	nameStyle := pink300
	switch {
	case selected && atCursor:
		nameStyle = yellow300.Bold(true)
	case selected:
		nameStyle = yellow300
	case atCursor:
		nameStyle = pink50.Bold(true)
	}

	return bar + box + " " + nameStyle.Render(name)
}

// VisiblePfpSlots computes image slots with absolute terminal cell positions
func (sp *SharePanel) VisiblePfpSlots(width, height, baseRow, baseCol int) []player.ImageSlot {
	if !sp.isOpen || len(sp.friends) == 0 || len(sp.pfps) == 0 {
		return nil
	}

	availableLines := height - 2
	if availableLines < 1 {
		return nil
	}

	var slots []player.ImageSlot
	linesUsed := 0
	currentRow := baseRow + 1 // +1 for header

	for i := sp.scroll; i < len(sp.friends); i++ {
		if linesUsed+sharePfpCellHeight > availableLines {
			break
		}

		if pfp, ok := sp.pfps[i]; ok {
			slots = append(slots, player.ImageSlot{
				Img: pfp,
				Row: currentRow,
				Col: baseCol,
			})
		}

		linesUsed += sharePfpCellHeight
		currentRow += sharePfpCellHeight
	}

	return slots
}
