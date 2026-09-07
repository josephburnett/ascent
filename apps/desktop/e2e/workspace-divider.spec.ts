import { test, expect } from './fixtures';
import { tileAt } from './oracle';

// Inside a pane-tile workspace, panes lay out into rootLayoutRect, which insets
// by wsOutlinePx once the workspace depth is above 0. dividerGrab has to read
// that same rect. Built from the full window instead, divider midlines drift
// from the real pane edges, the half-pixel adjacency match in
// pane.GrabDividers never fires, and a stacked boundary never arms for resize
// on either button. With no divider found the right-drag classifies as an edge
// split, so both buttons are driven here on a real divider inside a workspace.

async function workspaceState(window: any): Promise<{ depth: number }> {
  return window.evaluate(() => (window as any).__gridwellTest.workspace());
}

// barClick ascends out of the workspace by clicking a bar crumb. See
// leaveWorkspace in driver.ts for which crumb and why.
async function barClick(gw: any): Promise<void> {
  await gw.leaveWorkspace();
}

test('stacked-pane divider left-resizes inside a workspace; right-drag splits', async ({ gw, window }) => {
  await gw.enterPlugin('home');
  const f = await gw.focused();
  const rootGrid = f.gridID;
  const wx = Math.round(f.cx);
  const wy = Math.round(f.cy);

  await gw.openPalette();
  await gw.dragCreate('pane', wx, wy);
  const pt = tileAt(await gw.getGrid(rootGrid), 'pane', wx, wy);
  expect(pt, 'pane tile persisted').toBeTruthy();
  await gw.descendCell(wx, wy);
  await expect.poll(async () => (await workspaceState(window)).depth).toBe(1);

  // The boundary between two stacked panes is a horizontal divider.
  await gw.splitFocusedPaneHorizontal();
  expect((await gw.panes()).length).toBe(2);

  // The left button owns divider resizing, so the pane count stays 2 and the
  // top pane grows.
  const r = await gw.resizeHDivider('left', 60);
  expect((await gw.panes()).length, 'left-drag on the divider must resize, not split').toBe(2);
  expect(r.after, 'left-drag must move the stacked boundary').toBeGreaterThan(r.before + 30);

  // Back up, on the same divider and the same grab resolution.
  const l = await gw.resizeHDivider('left', -60);
  expect((await gw.panes()).length, 'left-drag on the divider must resize, not split').toBe(2);
  expect(l.after, 'left-drag must move the stacked boundary').toBeLessThan(l.before - 30);

  // A right drag from the same divider splits inside a workspace too. The drag
  // pulls away from the border, up into the top pane, so the new pane is drawn
  // out of the edge. Dragging toward the border would cancel.
  await gw.resizeHDivider('right', -80);
  expect((await gw.panes()).length, 'border right-drag split a pane').toBe(3);

  // Leave the workspace so the shared session ends at the root.
  await barClick(gw);
  await expect.poll(async () => (await workspaceState(window)).depth).toBe(0);
});
