import { expect } from '@playwright/test';
import { test, authenticate } from './fixtures';

// A wasm download that ends early must say so. The gzip sidecar carries no
// Content-Length, so a short body ends the stream cleanly and the only symptom
// is a parse error on a black page; index.html's boot script counts decoded
// bytes against X-Uncompressed-Size and retries once with the cache bypassed.
// The route interception is fault injection the real server cannot provide.

test('a short wasm download is named, and one cache-busted retry boots', async ({ serve, page }) => {
  await authenticate(page, serve);

  // A real prefix, so instantiateStreaming consumes the whole stream before it
  // complains, as it does against a raced sidecar.
  const full = await (await page.request.get(serve.origin + '/gridwell.wasm')).body();
  expect(full.length).toBeGreaterThan(1024 * 1024);

  let attempts = 0;
  await page.route('**/gridwell.wasm*', async (route) => {
    attempts++;
    if (attempts > 1) {
      await route.continue(); // the retry gets the whole file
      return;
    }
    await route.fulfill({
      status: 200,
      headers: {
        'Content-Type': 'application/wasm',
        // The declared decoded size stays right while the body is short.
        'X-Uncompressed-Size': String(full.length),
      },
      body: full.subarray(0, Math.floor(full.length / 3)),
    });
  });

  // The retry can remove the overlay before a poll sees it, so record every
  // message the boot script writes and assert against the record.
  await page.addInitScript(() => {
    const seen: string[] = ((window as any).__bootMsgs = []);
    new MutationObserver(() => {
      const el = document.getElementById('gw-boot-msg');
      const t = el && el.textContent;
      if (t && seen[seen.length - 1] !== t) seen.push(t);
    }).observe(document, { childList: true, characterData: true, subtree: true });
  });

  await page.goto(serve.origin + '/?e2e=1');

  await page.waitForFunction(() => !!(window as any).__gridwellTest, null, { timeout: 60_000 });
  expect(attempts).toBe(2);
  await expect(page.locator('#gw-boot')).toHaveCount(0);

  const msgs: string[] = await page.evaluate(() => (window as any).__bootMsgs);
  expect(msgs.join('\n')).toMatch(
    new RegExp(`truncated download \\(got \\d+ of ${full.length} bytes\\) — retrying`),
  );
});
