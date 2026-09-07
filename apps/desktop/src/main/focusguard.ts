// focusguard decides whether a live url view may keep OS keyboard focus. It
// holds no state, reads no clock and runs no timer. webviews.ts is the
// executor: it subscribes to the events, counts the presses, runs the timer and
// calls onFocusStolen.
//
// A live url view may hold OS keyboard focus only when its pane is the focused
// pane, or when the user just pressed into it. Anything else is a page-initiated
// steal, such as a self-refresh or a scripted reload focusing the new document's
// widget, and focus goes back to the root window, where the canvas and every
// shell overlay live.
//
// The verdict is deferred rather than taken in the focus handler because
// Chromium focuses the widget while it routes the press and forwards the press
// afterwards. When the `focus` event fires the press has not arrived: the
// browser-process `input-event` follows 0.1-3.0 ms later and the preload's IPC
// stamp 0.7-3.8 ms later. Deciding there would bounce the user's own first
// click. After one settle the guard can ask whether a press landed in this view
// after this focus.

// FOCUS_SETTLE_MS is how long the guard waits after a focus grab before it
// decides, and again after a bounce before it confirms.
//
// Chromium emits no event for a widget-focus commit, so there is nothing to
// wait on. A `rootWC.focus()` called from inside the focus handler is swallowed
// by the in-flight commit and moves nothing; a bounce 121 ms later moves focus
// back. 120 ms also covers the 3 ms the press correlation needs, and is short
// enough that leaked keystrokes stay negligible.
export const FOCUS_SETTLE_MS = 120;

// isPressInput reports whether an Electron InputEvent type is a press into the
// view, which is the one legitimate way a live view acquires OS focus. The
// registry's own touchScroll injects `mouseWheel` through sendInputEvent, which
// also raises `input-event`, so a wheel is excluded. Touch and pen presses
// count, because a tap on a touch host is the same gesture as a click.
export function isPressInput(type: string): boolean {
  return type === 'mouseDown' || type === 'touchStart' || type === 'pointerDown';
}

// GuardPhase is which step of the decision this is. 'focus-event' is the raw
// grab, where nothing is knowable yet; 'settle' is the deferred verdict, and
// the confirmation after a bounce.
export type GuardPhase = 'focus-event' | 'settle';

export interface GuardInput {
  phase: GuardPhase;
  // paneFocused is Entry.focused: whether the renderer says this pane is the
  // focused pane. The renderer owns it and carries it on place and setHidden.
  paneFocused: boolean;
  // viewHoldsOSFocus is webContents.isFocused(). At 'focus-event' the event
  // itself is the evidence, so the executor passes true.
  viewHoldsOSFocus: boolean;
  // pressesAtFocus is the view's press count when the focus event fired;
  // pressesNow is the count now. A press between the two is the user's click
  // arriving, which is what makes this focus legitimate. The count is monotonic
  // so there is no clock to step backwards and no window to tune.
  pressesAtFocus: number;
  pressesNow: number;
  // alreadyBounced is whether this focus has been bounced once already, so the
  // confirmation does not chain forever.
  alreadyBounced: boolean;
}

export type GuardAction =
  // allow: this view may keep OS focus. Nothing to do, nothing to schedule.
  | { kind: 'allow' }
  // wait: nothing is knowable yet. Do not bounce; ask again in settleMs.
  | { kind: 'wait'; settleMs: number }
  // bounce: report the steal. settleMs schedules the confirmation, or null when
  // this was already the confirmation.
  | { kind: 'bounce'; settleMs: number | null };

// decideFocus is the whole guard. Every arm reads a fact someone owns: the
// renderer's paneFocused, Chromium's viewHoldsOSFocus, and main's press count.
export function decideFocus(i: GuardInput): GuardAction {
  // The renderer says this pane is focused: its view is entitled to OS focus.
  if (i.paneFocused) return { kind: 'allow' };
  // Chromium says the view no longer holds focus — a bounce landed, or the
  // user moved on. There is nothing left to take back.
  if (!i.viewHoldsOSFocus) return { kind: 'allow' };
  // A press landed in this view after the focus: the user's own click, whose
  // widget focus Chromium applied before forwarding the press.
  if (i.pressesNow > i.pressesAtFocus) return { kind: 'allow' };
  // Nothing is knowable at the focus event itself; the press, if there is one,
  // is still in flight. Defer.
  if (i.phase === 'focus-event') return { kind: 'wait', settleMs: FOCUS_SETTLE_MS };
  // Unfocused pane, view holds focus, no press explains it: a steal.
  return { kind: 'bounce', settleMs: i.alreadyBounced ? null : FOCUS_SETTLE_MS };
}
