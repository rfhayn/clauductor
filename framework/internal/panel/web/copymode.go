package web

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/coder/websocket"
)

// PANEL-6: the wheel scrolls a lane's history by putting its pane in tmux's copy
// mode. In copy mode tmux takes every key for itself, so text typed while scrolled
// back never reached claude, silently. The terminal handler therefore watches the
// input of each viewer: after a wheel it asks tmux whether the pane is in copy mode
// (`display-message`, only then) and tells the page, which says "scrolled back";
// the first key that is not a scroll key leaves copy mode and then reaches claude.
// Escape only leaves: claude would take it as an interrupt.

var (
	mouseReports = regexp.MustCompile(`^(\x1b\[<\d+;\d+;\d+[Mm])+$`)
	// Wheel buttons 64-79 (up, down, left, right, with Shift and Meta) and 96-109 (most
	// of those with the motion bit). It misses 80-95, the wheel with Ctrl, which tmux
	// does not act on either: only a plain 64 or 65 changes copy mode (after).
	wheelReport = regexp.MustCompile(`\x1b\[<(6[4-9]|7[0-9]|9[6-9]|10[0-9]);`)
	scrollKeys  = regexp.MustCompile(`^\x1b(\[|O)[ABCDHF]$|^\x1b\[[1-6]~$|^\x1b\[1;\d[ABCDHF]$`)
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

// OPS-34 (#82): tmux reads the wheel from the lane's terminal, and a look reaches it
// on a connection of its own, so nothing orders the two: on a loaded machine tmux
// reads the wheel after any fixed delay, the look saw no copy mode, and the next key
// was lost to it again. The look after a scroll therefore repeats, often at first,
// until tmux shows the outcome it waits for, the program took the wheel, or its time
// has passed. A key typed before tmux has acted on the wheels ahead of it waits the
// same way when they will leave the pane in copy mode: the terminal is read in order,
// so tmux acts on those wheels first, and copy mode would take the key and every key
// after it. Which wheels end in copy mode is simulated (copySim), not assumed: a quick
// scroll up and back down ends on the live screen, and its key must not wait.
const (
	recheckFirst   = 25 * time.Millisecond
	recheckEvery   = 250 * time.Millisecond // the longest wait between two looks
	recheckFor     = 3 * time.Second        // when the wheels end in copy mode
	recheckDownFor = time.Second            // otherwise: copy mode can only end
)

// copySim is what the scrolls written so far leave the pane in, once tmux has read
// them. The zero copySim is the live screen, or nothing known: no copy mode to wait
// for.
type copySim struct {
	in    bool // copy mode
	lines int  // how far it is scrolled back; -1: not known
}

const wheelLines = 5 // what one wheel scrolls in copy mode (tmux's copy-mode table)

// wheel is s after tmux has read one more wheel, as tmux 3.6a acts on it with
// hardenArgs and its default copy-mode table: a wheel-up enters copy mode without
// scrolling (`copy-mode -e`) and, in it, scrolls up wheelLines; a wheel-down scrolls
// down wheelLines, and reaching the bottom leaves copy mode (-e). A wheel-down from
// an offset not known may or may not leave: nothing to wait for.
func (s copySim) wheel(up bool) copySim {
	switch {
	case up && !s.in:
		return copySim{in: true}
	case up && s.lines < 0:
		return s
	case up:
		return copySim{in: true, lines: s.lines + wheelLines}
	case s.in && s.lines > wheelLines:
		return copySim{in: true, lines: s.lines - wheelLines}
	}
	return copySim{}
}

// pageWriter is the viewer's websocket, as the watch writes to it.
type pageWriter interface {
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
}

// copyWatch is one viewer's view of its lane's copy mode. before and after run in
// turn on the viewer's input loop; a recheck runs beside it.
type copyWatch struct {
	lanes *lanes.LaneManager
	id    string
	conn  pageWriter
	clock clock.Clock

	mu    sync.Mutex
	gen   int       // counts inputs: a look for an earlier one gives way
	sim   copySim   // what the scrolls not yet seen acted on leave the pane in
	until time.Time // the looks after the latest scroll stop here
	maybe bool      // a wheel was sent: the pane may be in copy mode
	back  bool      // what the page was last told

	page sync.Mutex // orders the page's messages: taken before mu is released
}

// tellAndUnlock tells the page {"type":"scroll","back":…} when it changed, and
// releases w.mu, which the caller holds. The decision is taken under w.mu, the write
// after it is released; w.page, taken in between, keeps the messages in the order
// they were decided.
func (w *copyWatch) tellAndUnlock(ctx context.Context, back bool) {
	if w.back == back {
		w.mu.Unlock()
		return
	}
	w.back = back
	w.page.Lock()
	w.mu.Unlock()
	defer w.page.Unlock()
	msg := `{"type":"scroll","back":false}`
	if back {
		msg = `{"type":"scroll","back":true}`
	}
	_ = w.conn.Write(ctx, websocket.MessageText, []byte(msg))
}

// current reports whether no input came after the one numbered gen.
func (w *copyWatch) current(gen int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gen == gen
}

// await looks at the pane until tmux has acted on the scrolls written so far. in:
// they end in copy mode, unless the program takes the wheel; otherwise they can only
// end it. It stops at that outcome, at an error, or once until has passed (a zero
// until: one look), and gives up (ok false) when the context ends or an input after
// gen makes the answer moot. The first look comes after first; 0 looks at once.
func (w *copyWatch) await(ctx context.Context, gen int, in bool, until time.Time, first time.Duration) (inMode, program, ok bool) {
	for wait := first; ; wait = min(max(2*wait, recheckFirst), recheckEvery) {
		if wait > 0 {
			select {
			case <-ctx.Done():
				return false, false, false
			case <-w.clock.After(wait):
			}
		}
		if !w.current(gen) {
			return false, false, false
		}
		inMode, program, err := w.lanes.WheelOutcome(ctx, w.id)
		if err != nil || inMode == in || (in && program) || !w.clock.Now().Before(until) {
			return inMode, program, true
		}
	}
}

// recheck tells the page what tmux made of the scroll numbered gen, once it has.
func (w *copyWatch) recheck(ctx context.Context, gen int, in bool, until time.Time) {
	inMode, program, ok := w.await(ctx, gen, in, until, recheckFirst)
	w.mu.Lock()
	if !ok || w.gen != gen {
		w.mu.Unlock()
		return
	}
	switch {
	case !inMode && program:
		w.sim = copySim{}
		w.maybe = false // certain: the program, not copy mode, took the wheel
	case inMode == in: // tmux has acted on every scroll: simulate on from what it shows
		w.sim = copySim{in: inMode, lines: -1}
	} // otherwise the time ran out: tmux may not have read the scrolls yet, so sim stands
	w.tellAndUnlock(ctx, inMode)
}

// before runs ahead of writing one input message to the lane. It returns false
// when the message must not be written (an Escape that only left copy mode).
func (w *copyWatch) before(ctx context.Context, data string) bool {
	if k := inputKind(data); k == inMouse || k == inScroll {
		return true
	}
	w.mu.Lock()
	w.gen++ // a recheck still looking gives way: this key settles copy mode
	gen, coming, until, maybe := w.gen, w.sim.in, w.until, w.maybe
	w.sim, w.maybe = copySim{}, false
	w.mu.Unlock()
	if !maybe {
		return true
	}
	if !coming {
		until = time.Time{} // the scrolls end on the live screen: one look settles it
	}
	in, program, _ := w.await(ctx, gen, coming, until, 0)
	if in {
		w.lanes.LeaveCopyMode(ctx, w.id)
	}
	w.mu.Lock()
	if coming && !in && !program {
		w.maybe = true // the wait ran out before copy mode came: the next key looks once more
	}
	w.tellAndUnlock(ctx, false)
	return !(in && data == "\x1b")
}

// after runs once one input message was written to the lane: tmux acts on a scroll
// only once it has read it, so the recheck starts here.
func (w *copyWatch) after(ctx context.Context, data string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch inputKind(data) {
	case inMouse:
		b := wheelReport.FindAllStringSubmatch(data, -1)
		if b == nil {
			return
		}
		w.maybe = true
		for _, m := range b {
			// Only a plain wheel (64 up, 65 down) is bound, in root and in copy mode: a
			// wheel with a modifier, or a sideways one, changes nothing tmux shows.
			if n, _ := strconv.Atoi(m[1]); n == 64 || n == 65 {
				w.sim = w.sim.wheel(n == 64)
			}
		}
	case inScroll: // scrolling to the bottom ends copy mode by itself
		if !w.maybe {
			return
		}
		w.sim = copySim{} // what a scroll key does in copy mode is not simulated
	default:
		return
	}
	w.gen++
	w.until = w.clock.Now().Add(recheckDownFor)
	if w.sim.in {
		w.until = w.clock.Now().Add(recheckFor)
	}
	go w.recheck(ctx, w.gen, w.sim.in, w.until)
}
