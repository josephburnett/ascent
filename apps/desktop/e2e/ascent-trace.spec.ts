import { test, expect } from './fixtures';
import { tileAt } from './oracle';

// After an ascent a yellow outline marks the tile the pane just came out of,
// so the user can tell which shell or well they left. It arms when the ascent
// transition lands, fades over about two seconds, and expires. It is view
// state and nothing is persisted.

async function traces(window: any) {
  return window.evaluate(() => (window as any).__gridwellTest.traces());
}

test('ascending arms a fading trace on the tile just left, then it expires', async ({
  gw,
  window,
}) => {
  await gw.enterPlugin('home');
  const home = await gw.focused();
  const cx = Math.round(home.cx);
  const cy = Math.round(home.cy);

  await gw.openPalette();
  await gw.dragCreate('well', cx, cy);
  const well = tileAt(await gw.getGrid(home.gridID), 'well', cx, cy)!;

  await gw.descendCell(cx, cy);
  await expect.poll(async () => (await gw.focused()).gridID).toBe(well.childGridId);
  await gw.middleClickCell(cx, cy);
  await gw.waitIdle();

  const armed = await traces(window);
  expect(armed.length, 'one trace armed after the ascent').toBe(1);
  expect(armed[0].tileId, 'trace points at the well just left').toBe(well.id);
  expect(armed[0].paneId).toBe((await gw.focused()).id);
  expect(armed[0].alpha).toBeGreaterThan(0.3);

  await expect.poll(async () => (await traces(window)).length, { timeout: 5_000 }).toBe(0);

  const after = tileAt(await gw.getGrid(home.gridID), 'well', cx, cy)!;
  expect(after.version, 'no version bump from the trace').toBe(well.version);
});
