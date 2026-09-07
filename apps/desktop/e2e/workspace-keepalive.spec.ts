import { test, expect } from './fixtures';
import { tileAt } from './oracle';

// One live surface per content tile, across levels, driven through a real tmux
// session. Entering a fresh pane tile captures the layout, so the captured
// shell pane takes over the session and the outer pane detaches, with tmux
// state riding along. Leaving the view closes its panes and frees the surface,
// and the outer pane re-engages on the same session. One tmux session and one
// attachment at every step.

async function shellText(window: any): Promise<string> {
  return window.evaluate(() => (window as any).__gridwellTest.shellText());
}

test('a captured shell takes over the live session; leaving hands it back', async ({
  gw,
  window,
}) => {
  await gw.enterPlugin('home');
  const f = await gw.focused();
  const grid = f.gridID;
  const cx = Math.round(f.cx);
  const cy = Math.round(f.cy);

  // Split first: splitting a descended pane would ascend it.
  await gw.splitFocusedPaneVertical();
  const panes = (await gw.panes()).slice().sort((a: any, b: any) => a.x - b.x);
  await gw.focusPane(panes[0]);
  await gw.openPalette();
  await gw.dragCreate('shell', cx, cy);
  await gw.descendCell(cx, cy);
  await expect
    .poll(() => window.evaluate(() => (window as any).__gridwellTest.shellRenderer()), {
      timeout: 15_000,
    })
    .toBe('webgl');
  await window.keyboard.type('marker=keepalive-249');
  // The marker renders only after the PTY echoes it back, so poll for that
  // round trip.
  await expect.poll(() => shellText(window), { timeout: 10_000 }).toContain('marker=keepalive-249');

  // Descending a pane tile from the right pane captures the layout with the
  // shell pane still live.
  const right = (await gw.panes()).slice().sort((a: any, b: any) => a.x - b.x)[1];
  await gw.focusPane(right);
  const rf = await gw.focused();
  const px = Math.round(rf.cx) + 2; // clear of the shell tile at (cx, cy)
  const py = Math.round(rf.cy);
  await gw.openPalette();
  await gw.dragCreate('pane', px, py);
  expect(tileAt(await gw.getGrid(grid), 'pane', px, py)).toBeTruthy();
  await gw.descendCell(px, py);
  await expect
    .poll(async () => window.evaluate(() => (window as any).__gridwellTest.workspace().depth))
    .toBe(1);

  // The capture cloned the shell pane and its copy took over the session, since
  // there is one surface per tile. The typed but unentered marker on the
  // terminal is what shows it is the same tmux session.
  const innerShell = (await gw.panes()).find((p: any) => p.textFocus !== '');
  expect(innerShell, 'the captured layout carries the shell descent').toBeTruthy();
  await gw.focusPane(innerShell!);
  await expect.poll(() => shellText(window), { timeout: 15_000 }).toContain('marker=keepalive-249');

  // Leaving closes the view's panes, detaching and freezing, so the surface
  // frees and the outer shell pane re-engages on the same session.
  await gw.leaveWorkspace();
  await expect
    .poll(async () => window.evaluate(() => (window as any).__gridwellTest.workspace().depth))
    .toBe(0);
  const outerShell = (await gw.panes()).find((p: any) => p.textFocus !== '');
  expect(outerShell, 'the session shell pane survives').toBeTruthy();
  await gw.focusPane(outerShell!);
  await expect.poll(() => shellText(window), { timeout: 15_000 }).toContain('marker=keepalive-249');

  // Delete the shell tile so tmux never hangs the harness close.
  await gw.ascendViaCrumb();
  await expect.poll(async () => (await gw.focused()).textFocus, { timeout: 10_000 }).toBe('');
  await gw.deleteTileCell(cx, cy);
});
