import { test, expect } from './fixtures';

// The + creation menu appears on exactly one pane, whichever is focused.
// client/menu owns the rule: SyncFocus closes the menu when focus leaves its
// pane. This spec proves the wiring end to end through the real app.

test('the + menu closes when focus leaves its pane', async ({ gw }) => {
  await gw.enterPlugin('home');
  await gw.splitFocusedPaneVertical();
  const panes = await gw.panes();
  expect(panes.length, 'split produced two panes').toBe(2);
  const [left, right] = panes.slice().sort((a, b) => a.x - b.x);

  // Open the menu on the right pane: its + button sits at the window edge,
  // clear of the divider, while the left pane's + would overlap the divider's
  // resize band. Focus it first so a single + click toggles it open.
  await gw.focusPane(right);
  await gw.openPalette();
  expect((await gw.palette()).open, 'menu opened on the focused (right) pane').toBe(true);

  // palette() reports the focused pane, so a menu left stranded open on the
  // right pane reads open after focus leaves and comes back.
  await gw.focusPane(left);
  await gw.focusPane(right);
  expect(
    (await gw.palette()).open,
    'the menu must not survive focus leaving and returning to its pane',
  ).toBe(false);
});
