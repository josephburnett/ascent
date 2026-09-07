import { test, expect } from './fixtures';
import { tileAt } from './oracle';
import type { GridwellDriver } from './driver';

// Two panes text-descended into two different text tiles share one textarea
// singleton, bound to whichever tile was descended into last. Every bulk flush
// path (pane collapse, workspace enter and leave, which run
// flushDroppedSubtree into saveTextBeforeAscent) must check that binding
// (lastTextareaTileID) before posting the singleton's value as the flushed
// tile's content. Without it the save claims the flushed tile's own valid
// basis, so the server's version check waves through a total content
// replacement. The debounced-keystroke path has the guard already
// (textedit.ShouldDebouncedSaveFire). Only a spec holding two text descents
// alive at once, then bulk-flushing the unbound one, sees this.

// seedTile creates a markdown tile at (cx, cy), types seed into it, ascends,
// and waits for the content to persist. The pane must be at grid level.
async function seedTile(
  gw: GridwellDriver,
  gridID: string,
  cx: number,
  cy: number,
  seed: string,
): Promise<string> {
  await gw.openPalette();
  await gw.dragCreate('markdown', cx, cy);
  const created = tileAt(await gw.getGrid(gridID), 'text', cx, cy)!;
  expect(created, `markdown tile created at (${cx},${cy})`).toBeTruthy();
  await gw.descendCell(cx, cy);
  await gw.typeText(seed);
  await gw.ascendViaCrumb(); // bar-crumb ascent out of the text descent
  await expect
    .poll(async () => gw.getTileContent(created.id), { timeout: 10_000 })
    .toBe(seed);
  return created.id;
}

test('collapsing a text-descended pane never saves another tile\'s buffer into it', async ({ gw }) => {
  await gw.enterPlugin('home');
  const f = await gw.focused();
  const cx = Math.round(f.cx);
  const cy = Math.round(f.cy);

  // Two text tiles, either side of the viewport center; the center cell stays
  // empty so focus clicks land on no tile.
  const alphaID = await seedTile(gw, f.gridID, cx - 1, cy, 'alpha alpha');
  const bravoID = await seedTile(gw, f.gridID, cx + 1, cy, 'bravo bravo');

  // Descend left into alpha and right into bravo, so the textarea singleton
  // ends bound to bravo, the last text descent.
  await gw.splitFocusedPaneVertical();
  const panes = (await gw.panes()).slice().sort((a, b) => a.x - b.x);
  expect(panes.length, 'split produced two panes').toBe(2);

  await gw.focusPane(panes[0]);
  await gw.descendCell(cx - 1, cy);
  await expect
    .poll(async () => gw.textareaValue(), { timeout: 10_000 })
    .toBe('alpha alpha');

  const panesAfter = (await gw.panes()).slice().sort((a, b) => a.x - b.x);
  await gw.focusPane(panesAfter[1]);
  await gw.descendCell(cx + 1, cy);
  await expect
    .poll(async () => gw.textareaValue(), { timeout: 10_000 })
    .toBe('bravo bravo');

  // Collapse the left pane, the one the textarea singleton is not bound to.
  // Its flush must persist alpha's own content and never the singleton's bravo
  // bytes. Grab the divider from the right side so focus, and with it the
  // binding, stays on bravo: a right-press inside the left pane would transfer
  // focus there first, rebinding the singleton to alpha and masking the seam.
  // Dragging from the right pane's divider band to the left edge collapses the
  // left side (pane.CrushPlan, pane.RemoveSegment).
  const [leftPane, rightPane] = (await gw.panes()).slice().sort((a, b) => a.x - b.x);
  const midY = rightPane.y + rightPane.h / 2;
  await gw.leftDragScreen(rightPane.x + 3, midY, leftPane.x + 6, midY);
  expect((await gw.panes()).length, 'collapsed back to one pane').toBe(1);

  // Server truth: both tiles keep their own words.
  await expect
    .poll(async () => gw.getTileContent(alphaID), { timeout: 10_000 })
    .toBe('alpha alpha');
  expect(await gw.getTileContent(bravoID)).toBe('bravo bravo');
});
