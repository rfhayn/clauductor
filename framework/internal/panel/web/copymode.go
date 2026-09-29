package web

import (
	"context"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/coder/websocket"
)

// PANEL-6: the wheel scrolls a lane's history by putting its pane in tmux's copy
// mode. In copy mode tmux takes every key for itself, so text typed while scrolled
// back never reached claude, silently. The terminal handler therefore watches the
// input of each viewer: after a wheel it asks tmux whether the pane is in copy mode
// (one `display-message`, only then) and tells the page, which says "scrolled back";
// the first key that is not a scroll key leaves copy mode and then reaches claude.
// Escape only leaves: claude would take it as an interrupt.

var (
	mouseReports = regexp.MustCompile(`^(\x1b\[<\d+;\d+;\d+[Mm])+$`)
	wheelReport  = regexp.MustCompile(`\x1b\[<(6[4-9]|7[0-9]|9[6-9]|10[0-9]);`) // buttons 64/65 with any modifier
	scrollKeys   = regexp.MustCompile(`^\x1b(\[|O)[ABCDHF]$|^\x1b\[[1-6]~$|^\x1b\[1;\d[ABCDHF]$`)
)

// Input kinds, as copy mode treats them.
const (
	inMouse  = "mouse"  // mouse reports: tmux handles them (the wheel scrolls)
	inScroll = "scroll" // arrows, Page Up/Down, Home, End: they move through history
	inEscape = "escape" // a lone Escape
	inKey    = "key"    // anything else: text for claude
)

// inputKind classifies one input message from the page.
func inputKind(d string) string {
	switch {
	case mouseReports.MatchString(d):
		return inMouse
	case scrollKeys.MatchString(d):
		return inScroll
	case d == "\x1b":
		return inEscape
	}
	return inKey
}

// copyWatch is one viewer's view of its lane's copy mode.
type copyWatch struct {
	lanes *lanes.LaneManager
	id    string
	conn  *websocket.Conn
	maybe atomic.Bool // a wheel was sent: the pane may be in copy mode
	back  atomic.Bool // what the page was last told
	clock clock.Clock
}

// tell sends the page {"type":"scroll","back":…} when it changed.
func (w *copyWatch) tell(ctx context.Context, back bool) {
	if w.back.Swap(back) != back {
		msg := `{"type":"scroll","back":false}`
		if back {
			msg = `{"type":"scroll","back":true}`
		}
		_ = w.conn.Write(ctx, websocket.MessageText, []byte(msg))
	}
}

// recheck asks tmux shortly after a scroll, once tmux has acted on it.
func (w *copyWatch) recheck(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-w.clock.After(120 * time.Millisecond):
		}
		in := w.lanes.InCopyMode(ctx, w.id)
		w.maybe.Store(in)
		w.tell(ctx, in)
	}()
}

// before runs ahead of writing one input message to the lane. It returns false
// when the message must not be written (an Escape that only left copy mode).
func (w *copyWatch) before(ctx context.Context, data string) bool {
	switch inputKind(data) {
	case inMouse:
		if wheelReport.MatchString(data) {
			w.maybe.Store(true)
			w.recheck(ctx)
		}
		return true
	case inScroll:
		if w.maybe.Load() {
			w.recheck(ctx) // scrolling to the bottom ends copy mode by itself
		}
		return true
	}
	if !w.maybe.Swap(false) {
		return true
	}
	in := w.lanes.InCopyMode(ctx, w.id)
	if in {
		w.lanes.LeaveCopyMode(ctx, w.id)
	}
	w.tell(ctx, false)
	return !(in && data == "\x1b")
}
