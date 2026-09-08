import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { WebContentsView } from 'electron';
import { captureAttempt, CAPTURE_TIMEOUT_MS } from './capture';

// CAPTURE_TIMEOUT_MS is a promise about time — the freeze path detaches only
// after capturePage settles — so these wait on it rather than on a number.

// A view whose capturePage settles after ms, or never.
function viewSettlingAfter(ms: number | null): WebContentsView {
  const image = { isEmpty: () => false, toJPEG: () => Buffer.from('jpeg-bytes') };
  return {
    webContents: {
      capturePage: () =>
        new Promise((resolve) => {
          if (ms !== null) setTimeout(() => resolve(image), ms);
        }),
    },
  } as unknown as WebContentsView;
}

test('a parked renderer times out at the declared bound', async () => {
  const started = Date.now();
  // The race is the failure mode: with no timeout the promise never settles,
  // and a hung test says less than a failed one.
  const attempt = await Promise.race([
    captureAttempt(viewSettlingAfter(null)),
    new Promise<'hung'>((r) => setTimeout(() => r('hung'), CAPTURE_TIMEOUT_MS * 4)),
  ]);
  assert.notEqual(attempt, 'hung', `capturePage never settled and captureAttempt waited past ${CAPTURE_TIMEOUT_MS * 4}ms`);
  assert.deepEqual(attempt, { kind: 'timeout', timeoutMs: CAPTURE_TIMEOUT_MS });
  // Timers may fire a hair early; the point is that the wait was the bound and
  // not something shorter.
  assert.ok(Date.now() - started >= CAPTURE_TIMEOUT_MS - 50, `gave up after ${Date.now() - started}ms, before the ${CAPTURE_TIMEOUT_MS}ms bound`);
});

test('a slow renderer that answers inside the bound still yields its frame', async () => {
  const attempt = await captureAttempt(viewSettlingAfter(CAPTURE_TIMEOUT_MS / 4));
  assert.equal(attempt.kind, 'ok');
});
