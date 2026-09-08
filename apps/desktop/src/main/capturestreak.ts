// Whether a pane's mirror capture is failing and whether anyone has been told.
// A frozen preview must leave evidence, and the pump captures on a timer, so
// the report fires on the transition, not per frame. webviews.ts runs the
// capture and keeps the state on the entry.

// 'empty' is a frame that encoded to nothing, 'view-gone' a destroyed view.
// Everything but 'ok' counts toward the streak, because a frozen preview looks
// the same to the user however the capture died.
export type AttemptKind = 'ok' | 'empty' | 'timeout' | 'rejected' | 'view-gone';

export interface StreakState {
  // Separates a mirror that froze from one that has not started: Chromium
  // answers capturePage with an empty image for the first frames after a view
  // is placed, and those empties say nothing.
  everCaptured: boolean;
  // Consecutive failures; failing means failures > 0.
  failures: number;
}

// Failing carries the reason that opened the streak; recovered carries how many
// captures were lost.
export type StreakReport =
  | { kind: 'failing'; reason: AttemptKind }
  | { kind: 'recovered'; afterFailures: number };

export interface StreakDecision {
  state: StreakState;
  report: StreakReport | null;
}

// FRESH is frozen because every entry starts from this one value and
// decideStreak only returns new ones.
export const FRESH: StreakState = Object.freeze({ everCaptured: false, failures: 0 });

export function decideStreak(prev: StreakState, kind: AttemptKind): StreakDecision {
  if (kind === 'ok') {
    // A good frame ends the streak whatever opened it, since a reloaded
    // renderer captures again. Recovery is reported only where a failure was,
    // so the two always pair.
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
