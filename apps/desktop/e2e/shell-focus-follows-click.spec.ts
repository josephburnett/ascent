import { test, expect } from './fixtures';

// The xterm overlay swallows left mousedowns, so its capture listener must
// forward left as well as right. Forwarding only right leaves a click into a
// terminal transferring no pane focus while keystrokes still reach the PTY, so
// the user types in a shell Gridwell considers unfocused and every focus-gated
// affordance stays hidden. The live url view's forward is the same shape.

test('left-click into a live shell transfers pane focus', async ({
  window,
  gw,
}) => {
  await gw.enterPlugin('home');

  await gw.splitFocusedPaneVertical();
  const panes0 = (await gw.panes()).slice().sort((a, b) => a.x - b.x);
  const left = panes0[0];
  const right = panes0[panes0.length - 1];

  await window.mouse.click(left.x + left.w / 2, left.y + left.h / 2);
  await gw.waitIdle();
  const lf = await gw.focused();
  const cx = Math.round(lf.cx);
  const cy = Math.round(lf.cy);
  await gw.openPalette();
  await gw.dragCreate('shell', cx, cy);
  await gw.descendCell(cx, cy); // the drop lands bare; the descent creates the session
  await expect.poll(async () => (await gw.focused()).textFocus, { timeout: 15_000 }).not.toBe('');
  const shellPaneId = (await gw.focused()).id;

  await window.mouse.click(right.x + right.w / 2, right.y + right.h / 2);
  await gw.waitIdle();
  expect((await gw.focused()).id, 'focus moved off the shell pane').toBe(right.id);

  // The overlay swallows the mousedown, so it has to transfer pane focus.
  await window.mouse.click(left.x + left.w / 2, left.y + left.h / 2);
  await expect
    .poll(async () => (await gw.focused()).id, { timeout: 5_000 })
    .toBe(shellPaneId);

  // Delete the shell tile so its tmux session dies before teardown.
  await gw.ascendViaCrumb();
  await expect.poll(async () => (await gw.focused()).textFocus).toBe('');
  await gw.deleteTileCell(cx, cy);
});
