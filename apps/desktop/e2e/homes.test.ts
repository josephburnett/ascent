import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { sweepLeakedHomes, pluginUUIDs } from './homes';

// The start-of-run sweep. A fake leaked home from an aborted run must be
// removed, and only gridwell-e2e-* prefixed homes are ever touched.

test('sweepLeakedHomes removes leaked e2e homes and nothing else', () => {
  // The sweep derives the two directories it walks from the environment:
  // os.tmpdir() for homes, $TMUX_TMPDIR for sockets. Redirecting both to one
  // mkdtemp root gives this process its own copy of the tree the production
  // code walks, so two runs on one box neither see nor delete each other's
  // fixtures.
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'homes-test-'));
  const saved = {
    TMPDIR: process.env.TMPDIR,
    TMP: process.env.TMP,
    TEMP: process.env.TEMP,
    TMUX_TMPDIR: process.env.TMUX_TMPDIR,
  };
  for (const k of Object.keys(saved)) process.env[k] = root;
  try {
    assert.equal(os.tmpdir(), root, 'the sweep must walk this run\'s own root');
    const leaked = fs.mkdtempSync(path.join(os.tmpdir(), 'gridwell-e2e-'));
    // Both minted id shapes, 32-hex and the 7-char base36 short form. The regex
    // must find each, or that id's tmux server leaks.
    fs.writeFileSync(
      path.join(leaked, 'server.yaml'),
      'plugins:\n    - id: 0123456789abcdef0123456789abcdef\n      kind: localdb\n    - id: k3x9m2q\n      kind: localdb\n',
    );
    const foreign = fs.mkdtempSync(path.join(os.tmpdir(), 'gridwell-real-'));
    // Stale-socket sweep fixtures in tmux's socket dir ($TMUX_TMPDIR, else /tmp).
    // A dead gridwell-* socket is a plain file no server answers on and must be
    // removed; a non-gridwell name must never be touched.
    const sockDir = path.join(process.env.TMUX_TMPDIR || '/tmp', `tmux-${process.getuid?.() ?? ''}`);
    fs.mkdirSync(sockDir, { recursive: true, mode: 0o700 });
    const deadSock = path.join(sockDir, 'gridwell-zz9dead');
    const foreignSock = path.join(sockDir, 'homes-test-foreign');
    fs.writeFileSync(deadSock, '');
    fs.writeFileSync(foreignSock, '');
    assert.deepEqual(pluginUUIDs(leaked), ['0123456789abcdef0123456789abcdef', 'k3x9m2q']);
    sweepLeakedHomes();
    assert.equal(fs.existsSync(leaked), false, 'the leaked e2e home is swept');
    assert.equal(fs.existsSync(foreign), true, 'a non-e2e dir is never touched');
    assert.equal(fs.existsSync(deadSock), false, 'a dead gridwell-* socket is removed');
    assert.equal(fs.existsSync(foreignSock), true, 'a non-gridwell socket is never touched');
  } finally {
    for (const [k, v] of Object.entries(saved)) {
      if (v === undefined) delete process.env[k];
      else process.env[k] = v;
    }
    fs.rmSync(root, { recursive: true, force: true });
  }
});
