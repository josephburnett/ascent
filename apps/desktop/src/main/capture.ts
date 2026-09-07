import type { NativeImage, WebContentsView } from 'electron';

// JPEG quality for mirrored and frozen frames. The frozen preview is the
// durable picture of a url tile and is zoomed up in larger panes, so it has to
// stay crisp; the base64 payload over IPC is still modest at this quality.
const JPEG_QUALITY = 92;

// CAPTURE_TIMEOUT_MS time-boxes capturePage. A parked or busy renderer can
// leave the promise pending forever, and the freeze path detaches the view only
// after this resolves, which would strand the native view on top of the pane
// the user just left.
const CAPTURE_TIMEOUT_MS = 1500;

// CaptureAttempt is one capture attempt's outcome. A caller that sees only ''
// cannot tell a wedged renderer from a blank page, which is what a frozen
// preview needs said about it. The kind is capturestreak's AttemptKind; the
// payload beside it is this layer's, because the error objects are Electron's.
export type CaptureAttempt =
  | { kind: 'ok'; jpegBase64: string }
  | { kind: 'empty' }
  | { kind: 'timeout'; timeoutMs: number }
  | { kind: 'rejected'; error: unknown }
  | { kind: 'view-gone'; error: unknown };

// captureAttempt grabs a view's rendered contents as a base64 JPEG and says
// what became of the attempt. capturePage works on the visible attached view,
// so no offscreen-rendering mode is needed. The live pane shows native pixels;
// this capture feeds the other panes' frozen previews and the freeze-on-ascend
// snapshot.
//
// Every way it can fail is a case here, because from the outside they all look
// like a preview that stopped updating.
export async function captureAttempt(
  view: WebContentsView,
  timeoutMs = CAPTURE_TIMEOUT_MS,
): Promise<CaptureAttempt> {
  let pending: Promise<NativeImage>;
  try {
    // Reading webContents off a destroyed view throws synchronously, and so
    // does calling capturePage on a closed one. That is the view being gone.
    pending = view.webContents.capturePage();
  } catch (err) {
    return { kind: 'view-gone', error: err };
  }
  const settled = await settleWithin(pending, timeoutMs);
  if (settled.kind === 'timeout') return { kind: 'timeout', timeoutMs };
  if (settled.kind === 'rejected') return { kind: 'rejected', error: settled.error };
  const image = settled.value;
  if (!image || image.isEmpty()) return { kind: 'empty' };
  const jpegBase64 = image.toJPEG(JPEG_QUALITY).toString('base64');
  // A non-empty image that encodes to nothing is still no frame to show.
  return jpegBase64 ? { kind: 'ok', jpegBase64 } : { kind: 'empty' };
}

// captureJpegBase64 is captureAttempt for the callers that only want the bytes:
// '' means no frame, whatever went wrong. The view-gone arm is rethrown so
// remove() can catch it and report "view crashed while closing". Its getURL()
// read on the same dead webContents usually throws first, but the report must
// not depend on that ordering.
export async function captureJpegBase64(view: WebContentsView, timeoutMs = CAPTURE_TIMEOUT_MS): Promise<string> {
  const attempt = await captureAttempt(view, timeoutMs);
  if (attempt.kind === 'view-gone') throw attempt.error;
  return attempt.kind === 'ok' ? attempt.jpegBase64 : '';
}

// describeAttempt is the text of an attempt, for the report the user reads.
// Only the failing arms are reported; 'ok' is here so the switch is exhaustive
// and a new kind cannot be added with no description.
export function describeAttempt(attempt: CaptureAttempt): string {
  switch (attempt.kind) {
    case 'ok':
      return 'ok';
    case 'empty':
      return 'the renderer produced an empty frame';
    case 'timeout':
      return `capturePage did not answer within ${attempt.timeoutMs}ms`;
    case 'rejected':
      return `capturePage failed: ${String(attempt.error)}`;
    case 'view-gone':
      return `the view is gone: ${String(attempt.error)}`;
  }
}

// Settled distinguishes the three ways the time-boxed promise can end, so a
// timeout and a rejection stay visible downstream.
type Settled<T> =
  | { kind: 'value'; value: T }
  | { kind: 'timeout' }
  | { kind: 'rejected'; error: unknown };

// settleWithin resolves, and never rejects, with what p did, or 'timeout' if p
// had not settled within ms.
function settleWithin<T>(p: Promise<T>, ms: number): Promise<Settled<T>> {
  return new Promise<Settled<T>>((resolve) => {
    let done = false;
    const finish = (s: Settled<T>) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      resolve(s);
    };
    const timer = setTimeout(() => finish({ kind: 'timeout' }), ms);
    p.then(
      (value) => finish({ kind: 'value', value }),
      (error) => finish({ kind: 'rejected', error }),
    );
  });
}

// MirrorPump periodically captures every live pane and pushes frames to a sink,
// so a tile mirrored in a second pane stays fresh. index.ts sweeps
// reg.paneIds() each tick. The cadence is low, because mirrored previews do not
// need frame rate.
export class MirrorPump {
  private timer: NodeJS.Timeout | null = null;
  private readonly intervalMs: number;
  private readonly tick: () => Promise<void>;

  constructor(intervalMs: number, tick: () => Promise<void>) {
    this.intervalMs = intervalMs;
    this.tick = tick;
  }

  start(): void {
    if (this.timer) return;
    const loop = async () => {
      await this.tick().catch(() => {});
      if (this.timer) this.timer = setTimeout(loop, this.intervalMs);
    };
    this.timer = setTimeout(loop, this.intervalMs);
  }

  stop(): void {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }
}
