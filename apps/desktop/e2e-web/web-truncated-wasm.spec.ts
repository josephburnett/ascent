import { expect } from '@playwright/test';
import { test, authenticate } from './fixtures';

// The truncated-boot seam: a wasm download that ends early must SAY so.
//
// The gzip sidecar is served without a Content-Length (ServeContent omits it
// under Content-Encoding), so a body that stops early ends the stream cleanly
// and nothing in the transport complains. That is how a sidecar gzipped from a
// half-written build reached a browser as a clean 200 whose only symptom was
// "WebAssembly.Module doesn't parse ... exceeds the module's remaining size" on
// a black page. index.html's boot script counts the decoded bytes against the
// size the server declared in X-Uncompressed-Size and calls the fault by its
// name, then retries once with the cache bypassed.
//
// The route interception below is the fault injection the real server cannot
// provide: the first /gridwell.wasm answers with a genuine PREFIX of the real
// module and the FULL declared size, exactly the shape of the bug; the second
// goes to the server untouched, so the retry is what actually boots the client.

test('a short wasm download is named, and one cache-busted retry boots', async ({ serve, page }) => {
  await authenticate(page, serve);

  // The real module, fetched once through the page's own cookie jar, so the
  // truncated body is a real prefix and instantiateStreaming consumes the whole
  // stream before it complains — the way it does against a raced sidecar.
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
        // The server's declared decoded size, unchanged — the body is short.
        'X-Uncompressed-Size': String(full.length),
      },
      body: full.subarray(0, Math.floor(full.length / 3)),
    });
  });

  // The overlay's message is transient — the retry can boot and remove the
  // whole overlay before a poll sees it — so record every message the boot
  // script writes and assert against the record.
  await page.addInitScript(() => {
    const seen: string[] = ((window as any).__bootMsgs = []);
    new MutationObserver(() => {
      const el = document.getElementById('gw-boot-msg');
      const t = el && el.textContent;
      if (t && seen[seen.length - 1] !== t) seen.push(t);
    }).observe(document, { childList: true, characterData: true, subtree: true });
  });

  await page.goto(serve.origin + '/?e2e=1');

  // The retry lands: the client boots from the untouched second response.
  await page.waitForFunction(() => !!(window as any).__gridwellTest, null, { timeout: 60_000 });
  expect(attempts).toBe(2);
  await expect(page.locator('#gw-boot')).toHaveCount(0);

  // And on the way it named the download, not the parse, counting both sides.
  const msgs: string[] = await page.evaluate(() => (window as any).__bootMsgs);
  expect(msgs.join('\n')).toMatch(
    new RegExp(`truncated download \\(got \\d+ of ${full.length} bytes\\) — retrying`),
  );
});
