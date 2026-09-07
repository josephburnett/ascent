import { test, expect } from './fixtures';

// Crosses the plugin-classification seam: a plugin whose Info handshake
// succeeds and declares no grid of its own, an fs plugin with no config.root,
// is healthy. A plugin contributes + menu entries and is not itself a place, so
// having nothing to enter is not a failure and must not present as one. A row
// with nothing to offer contributes nothing to the menu.
//
// The two non-healthy statuses are "broken", a recorded failure whatever the
// reason, and "waiting", asked and not yet answered, which only a connection
// row can be in (conn-config.spec.ts covers it). Both still get a swatch,
// because the menu has something to say about them.
//
// A plugin whose Info fails outright is not covered here: a plugin that fails
// to spawn aborts the whole server by design, in internal/plugin's loader, so
// no real app boot reaches it. buildPluginInfo tests in
// internal/server/plugininfo_test.go cover that shape.
test.use({ extraPlugins: [{ kind: 'fs', name: 'noroot' }] });

test('a plugin that declares no doorway is healthy and shows nothing', async ({ gw, window }) => {
  const pls = await gw.plugins();
  const noroot = pls.find((p) => p.label === 'noroot');
  expect(noroot, 'rootless fs plugin configured').toBeTruthy();
  expect(noroot!.rootGridID, 'it declared no grid of its own').toBe('');
  expect(noroot!.infoError, 'and it answered without any Info error').toBe('');
  expect(noroot!.status, 'answered with no doorway is healthy, not broken').toBe('nodoor');

  // Boot landed on home: a node's home is where "/" means, and no plugin
  // competes for it.
  const before = await gw.focused();
  expect(before.anchor, 'boots into the node home').toBe(
    pls.find((p) => p.label === 'home')!.rootGridID,
  );

  await gw.openPalette();
  await gw.expandPlugins();
  const pal = await gw.palette();
  const swatch = pal.items.find((i) => i.isPlugin && i.label === 'noroot');
  expect(swatch, 'a plugin with no doorway contributes no swatch').toBeFalsy();

  const errs = await window.evaluate(() => (window as any).__gridwellTest.errors());
  const notice = errs.notices.find((n: any) => n.source === 'launcher:' + noroot!.uuid);
  expect(notice, 'a healthy plugin reports nothing').toBeFalsy();
});
