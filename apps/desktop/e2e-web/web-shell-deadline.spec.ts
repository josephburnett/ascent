import { test, expect } from './fixtures';

// The web door's deadline rule for the /shell WebSocket after hijack. A
// declared timeout is a fact, and its test is a wait bound to its value. The
// web door declares ReadHeaderTimeout: 10s (server.WebDoorServer); net/http
// clears that deadline before the handler runs, and shell_door.go's
// websocket.Accept (coder/websocket) hijacks the conn and owns its deadlines
// through context. So a live shell must survive a wall-clock wait longer than
// the declared timeout and still carry bytes both ways. No other spec idles a
// live shell past 10s, so a refactor that put the shell back on the door's
// request deadline would only be caught here. web-shell.spec.ts covers the
// plain attach chain.
//
// Scope: because coder/websocket takes the conn's deadlines off net/http,
// adding a ReadTimeout or WriteTimeout to WebDoorServer does not cut this
// WebSocket, verified by the spec staying green with either. It guards the
// survival property, while internal/server/door_deadline_test.go and
// test/connections/door_deadline_test.go fail the moment a door deadline
// returns.
//
// A browser-mode spec cannot import the Go shape, so HOLD_MS is a wall-clock
// constant tied to the 10s ReadHeaderTimeout by this comment, with a second of
// margin. The roughly 11s of real time is the test and must not be shortened.
// This is the browser-mode shell coverage make check-web runs, the only gate
// that exercises the real /shell WebSocket end to end.

const HEADER_TIMEOUT_MS = 10_000; // server.WebDoorServer ReadHeaderTimeout
const HOLD_MS = HEADER_TIMEOUT_MS + 1_000;

const shellText = (window: any): Promise<string> =>
  window.evaluate(() => (window as any).__gridwellTest.shellText());

test('a browser shell outlives the web door header timeout', async ({ gw, window }) => {
  await gw.enterPlugin('home');
  const home = await gw.focused();
  const cx = Math.round(home.cx);
  const cy = Math.round(home.cy);

  await gw.openPalette();
  await gw.dragCreate('shell', cx, cy);
  await gw.descendCell(cx, cy); // the drop lands bare; the descent creates the session
  await expect.poll(async () => (await gw.focused()).textFocus, { timeout: 20_000 }).not.toBe('');

  await window.keyboard.type('echo shell-before-wait');
  await window.keyboard.press('Enter');
  await expect
    .poll(() => shellText(window), { timeout: 20_000 })
    .toContain('shell-before-wait');

  // Hold the hijacked WebSocket idle past the web door's declared header
  // timeout. A conn still on that deadline would be closed here.
  await window.waitForTimeout(HOLD_MS);

  await window.keyboard.type('echo shell-after-wait');
  await window.keyboard.press('Enter');
  await expect
    .poll(() => shellText(window), { timeout: 20_000 })
    .toContain('shell-after-wait');

  // A capability notice about shells needing the desktop app would mean the
  // browser never attached.
  const errs = await window.evaluate(() => (window as any).__gridwellTest.errors());
  expect(errs.notices, 'no capability notice: the browser really attached').toEqual([]);

  // Delete the tile so its tmux session dies before teardown.
  await gw.ascendViaCrumb();
  await expect.poll(async () => (await gw.focused()).textFocus).toBe('');
  await gw.deleteTileCell(cx, cy);
});
