// capturestreak decides whether a pane's mirror capture is failing and whether
// anyone has been told. It holds no state and reads no clock. webviews.ts is
// the executor: it runs the capture, labels what came back, keeps the state on
// the entry and sends the reports.
//
// A mirrored preview that stops updating must leave evidence, and the report
// fires on the transition. The pump captures every live pane on a timer, so a
// per-frame report would bury the log.

// AttemptKind labels the outcome of one capture attempt. Everything but 'ok'
// counts toward the streak, because a frozen preview looks the same to the user
// however the capture died.
//
//   ok        a frame came back with bytes in it
//   empty     capturePage resolved an empty image, or one that encoded to
//             nothing: a live view that rendered no pixels
//   timeout   capturePage did not settle inside the time box
//   rejected  capturePage rejected
//   view-gone reading the view's webContents threw: the view is destroyed
export type AttemptKind = 'ok' | 'empty' | 'timeout' | 'rejected' | 'view-gone';

// StreakState is everything remembered about one pane's mirror. The executor
// stores it on the entry and hands it back.
export interface StreakState {
  // everCaptured separates a mirror that froze from one that has not started.
  // Chromium answers capturePage with an empty image for the first frames after
  // a view is placed, while the pane still shows its stored preview, so those
  // empties say nothing. A page that fails to load is the did-fail-load and
  // render-process-gone reports' business.
  everCaptured: boolean;
  // failures is the consecutive-failure count. Failing means failures > 0.
  failures: number;
}

// StreakReport is what to tell the user, or null for nothing. Failing carries
// the reason that opened the streak; recovered carries how many captures were
// lost, which is the measure of how stale the mirror got.
export type StreakReport =
  | { kind: 'failing'; reason: AttemptKind }
  | { kind: 'recovered'; afterFailures: number };

export interface StreakDecision {
  state: StreakState;
  report: StreakReport | null;
}

// FRESH is a pane whose mirror has not captured yet. Frozen because every entry
// starts from this one value and decideStreak only returns new ones.
export const FRESH: StreakState = Object.freeze({ everCaptured: false, failures: 0 });

// decideStreak folds one attempt into the streak.
export function decideStreak(prev: StreakState, kind: AttemptKind): StreakDecision {
  if (kind === 'ok') {
    // A good frame ends the streak whatever opened it, including a destroyed
    // view, since a reloaded renderer captures again. Recovery is reported only
    // where a failure was reported, so the two always pair.
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
