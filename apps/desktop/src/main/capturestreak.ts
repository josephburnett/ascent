// capturestreak owns the whole "is this pane's mirror capture failing, and has
// anyone been told yet?" decision. It is pure: no Electron import, no clock, no
// state of its own. webviews.ts is the executor — it runs the capture, labels
// what came back, keeps the state on the entry and sends the reports — and
// makes no comparison of its own.
//
// The rule: a mirrored preview that stops updating must not be evidence-free,
// and the report fires on the transition, not per frame. The pump captures
// every live pane on a timer, so a per-frame report would bury the log.
//
// Why this is a module and not three lines inside capture(): the streak used to
// live inline there and counted only what threw. Every failure that actually
// happens — a capturePage that rejects, one that never settles, an image that
// comes back empty — resolved to '' rather than throwing, so the counter never
// saw the exact cases the report exists for. The one arm it did see, a
// destroyed view, never captures again, so the flag latched set and the
// recovery report was unreachable.
//
// The state is a count, not a flag beside a count: failing IS failures > 0.
// A latch that could disagree with the count is the bug that was here.

// AttemptKind labels the outcome of one capture attempt. Everything that is not
// 'ok' is a failure and counts toward the streak — a frozen preview looks the
// same to the user however the capture died.
//
//   ok        a frame came back with bytes in it
//   empty     capturePage resolved an empty image, or one that encoded to
//             nothing: a live view that rendered no pixels
//   timeout   capturePage did not settle inside the time box (a parked or
//             wedged renderer)
//   rejected  capturePage rejected
//   view-gone reading the view's webContents threw: the view is destroyed
export type AttemptKind = 'ok' | 'empty' | 'timeout' | 'rejected' | 'view-gone';

// StreakState is everything remembered about one pane's mirror. The executor
// stores it on the entry and hands it back; nothing else reads it.
export interface StreakState {
  // everCaptured is whether this pane has ever produced a frame. It is what
  // separates a mirror that froze from one that has not started: a view is
  // placed, and Chromium answers capturePage with an empty image for the first
  // frames, before it has painted anything. Nothing is frozen there — the pane
  // is still showing its stored preview, which is correct — so those empties
  // count nothing and say nothing. A page that fails to load at all is the
  // did-fail-load and render-process-gone reports' business, not the mirror's.
  everCaptured: boolean;
  // failures is the consecutive-failure count. "Failing" is failures > 0.
  failures: number;
}

// StreakReport is what to tell the user, or null for "nothing to say". Failing
// carries the reason that opened the streak; recovered carries how many
// captures were lost, which is the only measure of how stale the mirror got.
export type StreakReport =
  | { kind: 'failing'; reason: AttemptKind }
  | { kind: 'recovered'; afterFailures: number };

export interface StreakDecision {
  state: StreakState;
  report: StreakReport | null;
}

// FRESH is a pane whose mirror has not captured yet. Frozen because every entry
// starts from this one value and decideStreak only ever returns new ones.
export const FRESH: StreakState = Object.freeze({ everCaptured: false, failures: 0 });

// decideStreak folds one attempt into the streak.
export function decideStreak(prev: StreakState, kind: AttemptKind): StreakDecision {
  if (kind === 'ok') {
    // A good frame ends the streak whatever opened it — including the
    // destroyed-view arm, since a reloaded renderer captures again. Recovery is
    // reported only where a failure was reported, so the two always pair.
    const reportable = prev.everCaptured && prev.failures > 0;
    return {
      state: { everCaptured: true, failures: 0 },
      report: reportable ? { kind: 'recovered', afterFailures: prev.failures } : null,
    };
  }
  return {
    state: { everCaptured: prev.everCaptured, failures: prev.failures + 1 },
    report: prev.everCaptured && prev.failures === 0 ? { kind: 'failing', reason: kind } : null,
  };
}
