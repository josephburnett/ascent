import { test as base, _electron as electron, ElectronApplication, Page } from '@playwright/test';
import * as path from 'node:path';
import * as os from 'node:os';
import * as fs from 'node:fs';
import { spawn, ChildProcess } from 'node:child_process';
import { GridwellDriver } from './driver';
import { setOracleAuth } from './oracle';
import { pluginUUIDs, killTmuxServers } from './homes';
import { parseServingLine } from '../src/main/lines';
import { freePort } from '../src/main/freeport';

// apps/desktop, and the repo root two levels up (where `make build` lays out the
// gridwell sidecar + plugin binaries and the web/ static dir).
const DESKTOP_DIR = path.resolve(__dirname, '..');
const REPO_ROOT = path.resolve(DESKTOP_DIR, '..', '..');

// PluginSpec is one content plugin to declare in the seeded home's server.yaml
// (see seedHome's extra param), such as an fs plugin with no config.root.
export interface PluginSpec {
  kind: string;
  name: string;
  config?: Record<string, string>;
}

// seedHome creates a throwaway Gridwell home: a server.yaml declaring the given
// content plugins. The first serve mints the ids and creates the home store,
// which is the first run a real user gets. Every launch points GRIDWELL_HOME at
// a home seeded this way. `extraYaml` appends raw server.yaml sections, such as
// a connections: list. Returns the home dir; callers remove it on teardown.
export function seedHome(extra: PluginSpec[] = [], extraYaml = ''): string {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'gridwell-e2e-'));
  let yaml = '';
  if (extra.length) {
    yaml += 'plugins:\n';
    for (const p of extra) {
      yaml += `    - kind: ${p.kind}\n      label: ${p.name}\n`;
      const conf = Object.entries(p.config ?? {});
      if (conf.length) {
        yaml += '      config:\n';
        for (const [k, v] of conf) yaml += `        ${k}: ${JSON.stringify(v)}\n`;
      }
    }
  }
  fs.writeFileSync(path.join(home, 'server.yaml'), yaml + extraYaml);
  return home;
}

// FarNode is a second `gridwell serve` a spec reaches through a direct
// connection. A node has exactly one home, so another writable space is
// another node.
export interface FarNode {
  label: string;
  home: string;
  child: ChildProcess;
}

// spawnFarNode boots a fresh node (empty server.yaml; its first serve mints
// its id) on a loopback port and waits for its banner. The connection
// socket lives under its home, which is what the local node's connection
// dials.
async function spawnFarNode(label: string): Promise<FarNode> {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'gridwell-e2e-'));
  fs.writeFileSync(path.join(home, 'server.yaml'), '');
  const bin = path.join(REPO_ROOT, process.env.GRIDWELL_SERVE_BIN || 'gridwell');
  const port = await freePort();
  const child = spawn(bin, ['serve', '--bind', `127.0.0.1:${port}`], {
    env: { ...process.env, GRIDWELL_HOME: home, GRIDWELL_PLUGIN_DIR: REPO_ROOT },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let output = '';
  child.stdout!.on('data', (d) => (output += d));
  child.stderr!.on('data', (d) => (output += d));
  const deadline = Date.now() + 15_000;
  for (;;) {
    if (output.split('\n').some((l) => parseServingLine(l)?.auth)) return { label, home, child };
    if (child.exitCode !== null || Date.now() > deadline) {
      child.kill('SIGKILL');
      throw new Error(`far node ${label} did not announce:\n${output}`);
    }
    await new Promise((r) => setTimeout(r, 50));
  }
}

async function stopFarNode(n: FarNode): Promise<void> {
  if (n.child.exitCode === null) {
    await new Promise<void>((resolve) => {
      const hard = setTimeout(() => {
        n.child.kill('SIGKILL');
        resolve();
      }, 3_000);
      n.child.once('exit', () => {
        clearTimeout(hard);
        resolve();
      });
      n.child.kill('SIGTERM');
    });
  }
  killTmuxServers(pluginUUIDs(n.home));
  fs.rmSync(n.home, { recursive: true, force: true });
}


// homePassword reads the web password serve minted into a home's web-password
// file. The door is never open, and the file exists only once a serve has
// started on that home.
export function homePassword(home: string): string {
  return fs.readFileSync(path.join(home, 'web-password'), 'utf8').trim();
}

// loginToken posts the password to the login form and returns the auth cookie
// value the server issued, the same token the serve banner carries, so oracle
// RPCs and a plain browser page can authenticate.
export async function loginToken(origin: string, password: string): Promise<string> {
  const res = await fetch(origin + '/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ password }).toString(),
    redirect: 'manual',
  });
  const m = /gridwell_auth=([0-9a-f]{64})/.exec(res.headers.get('set-cookie') ?? '');
  if (!m) throw new Error(`login at ${origin} issued no cookie (${res.status})`);
  return m[1];
}

// assertSidecarExited polls briefly that the sidecar process is dead after
// app.close(), so a leak is blamed on the test that caused it rather than on a
// later one hitting a stale port or database lock.
async function assertSidecarExited(pid: number | null): Promise<void> {
  if (pid == null) return;

  // Electron's before-quit handler sends SIGTERM to the sidecar; give it up
  // to 3 s to exit cleanly.
  const deadline = Date.now() + 3_000;
  while (Date.now() < deadline) {
    try {
      process.kill(pid, 0); // throws ESRCH when the process is gone
    } catch (err: unknown) {
      if ((err as NodeJS.ErrnoException).code === 'ESRCH') return; // gone
      return; // EPERM: it exists but is not ours to signal; treat as fine
    }
    await new Promise((r) => setTimeout(r, 100));
  }

  // Still alive after the grace period, so this test is blamed rather than a
  // later one colliding with the stale process.
  throw new Error(
    `e2e teardown leak: sidecar (pid ${pid}) still running after app.close(). ` +
      'This test did not clean up properly (e.g. a live shell tile was left open). ' +
      'The sidecar must exit before the next test starts or port/DB locks will bleed.',
  );
}

type Fixtures = {
  // home is the seeded temp home (server.yaml, DBs) the app launches on;
  // the gw fixture reads its minted web-password to authenticate the oracle.
  home: string;
  electronApp: ElectronApplication;
  window: Page;
  gw: GridwellDriver;
  // extraPlugins is a test option, set with test.use({ extraPlugins: [...] }) in
  // a spec file: content plugins seedHome declares, present from the first
  // launch.
  extraPlugins: PluginSpec[];
  // extraNodes: labels of far nodes to boot and connect directly, one row each
  // in the + menu. This is the second writable space a cross-namespace spec
  // needs.
  extraNodes: string[];
  extraYaml: string;
};

// The e2e fixture launches the same `electron .` entry `make launch` uses.
// apps/desktop/src/main/index.ts spawns the Go sidecar itself, so the whole
// stack runs: renderer, wasm, Connect-RPC, server, SQLite. Each test gets a
// fresh temp home, and GRIDWELL_E2E=1 turns on the renderer's read-only
// introspection hook.
//
// GRIDWELL_HOME is a per-test mkdtemp, and Electron's userData is set to
// <home>/electron two ways: the --user-data-dir command-line flag, which
// Chromium reads before any Node.js module runs, and applyUserDataOverride in
// index.ts, which covers a direct non-Playwright launch with GRIDWELL_HOME set
// and no flag. With both, no test instance shares
// ~/.config/gridwell-desktop with the live app or with a concurrent instance.
//
// After app.close() the fixture kills stray tmux servers and asserts the
// sidecar exited.
export const test = base.extend<Fixtures>({
  extraPlugins: [[], { option: true }],
  extraNodes: [[], { option: true }],
  extraYaml: ['', { option: true }],

  home: async ({ extraPlugins, extraNodes, extraYaml }, use) => {
    // Far nodes come up before the local node, so its boot-time connect learns
    // each landing synchronously. They go down after the app closes, since this
    // fixture's teardown runs after electronApp's.
    const far: FarNode[] = [];
    for (const label of extraNodes) far.push(await spawnFarNode(label));
    let yaml = extraYaml;
    if (far.length) {
      yaml += 'connections:\n';
      for (const n of far) {
        yaml += `    - name: ${n.label}\n      label: ${n.label}\n      addr: ${path.join(n.home, 'federation.sock')}\n`;
      }
    }
    await use(seedHome(extraPlugins, yaml));
    for (const n of far) await stopFarNode(n);
  },

  electronApp: async ({ home }, use) => {
    // The per-test Chromium profile. Created before launch so the flag is valid.
    const electronDir = path.join(home, 'electron');
    fs.mkdirSync(electronDir, { recursive: true });
    const app = await electron.launch({
      // --user-data-dir is a Chromium switch, so Chromium picks up the
      // isolated profile directory before the Node.js main script runs.
      // Playwright intercepts app.isReady(), so app.setPath() in index.ts runs
      // after Chromium has already initialised.
      //
      // The flag must come before the app path ('.') in args, because Electron
      // treats everything after the app path as app arguments. Playwright
      // prepends --inspect=0 and --remote-debugging-port=0, so the final argv
      // is:
      //   electron --inspect=0 --remote-debugging-port=0 --user-data-dir=... .
      args: [`--user-data-dir=${electronDir}`, '.'],
      cwd: DESKTOP_DIR,
      env: {
        // Strip the live app's plugin env vars so they cannot bleed into the
        // test sidecar's plugin subprocess. GRIDWELL_PLUGIN_CONFIG carries the
        // live app's DB path, and go-plugin re-appends os.Environ() at Start(),
        // so it would land as the last duplicate and override the fresh
        // per-test config. GRIDWELL_PLUGIN is a companion var go-plugin sets in
        // the live app's tmux session. The sidecar sets both for each launch.
        ...Object.fromEntries(
          Object.entries(process.env).filter(
            ([k]) => k !== 'GRIDWELL_PLUGIN_CONFIG' && k !== 'GRIDWELL_PLUGIN',
          ),
        ),
        GRIDWELL_E2E: '1',
        GRIDWELL_HOME: home,
        GRIDWELL_SIDECAR: path.join(REPO_ROOT, 'gridwell'),
        GRIDWELL_STATIC: path.join(REPO_ROOT, 'web'),
      },
    });
    await use(app);

    // ── Teardown (runs after every test, pass or fail) ──────────────────────

    // Teardown must complete from any spec end state, including a spec that
    // died mid-body with a live shell still attached. If it hangs, the worker
    // is SIGKILLed at the test timeout, the tmux kill, the home removal and the
    // sidecar assert are all skipped, and the report gains a 90s "Tearing down
    // electronApp" plus an unattributed error that reads as a flake.

    // Capture the sidecar pid before closing. index.ts exposes it under
    // GRIDWELL_E2E=1; it is null if the app never finished booting.
    let sidecarPid: number | null = null;
    try {
      sidecarPid = await app.evaluate(
        () => (globalThis as { __gwSidecarPid?: number }).__gwSidecarPid ?? null,
      );
    } catch {
      // The app already crashed or closed; the pid is unknown.
    }

    // electronApp.close() does not settle when a live shell stream existed at
    // close time. The Electron process exits promptly with code 0 and the exit
    // event, while the Playwright-side promise hangs; only shells wedge it, not
    // a dirty live url view. So close() races a deadline and the process exit
    // is verified here, since nothing downstream depends on close()'s own
    // bookkeeping.
    const proc = app.process();
    const closed = await Promise.race([
      app.close().then(
        () => true,
        () => true,
      ),
      new Promise<boolean>((r) => {
        const t = setTimeout(() => r(false), 10_000);
        t.unref?.();
      }),
    ]);
    if (!closed) {
      // A wedged close is expected only with a live shell, so seeing it on
      // another spec is new information.
      console.warn('[e2e teardown] electronApp.close() did not settle in 10s; proceeding with direct cleanup');
      if (proc.exitCode === null) {
        // The app is still alive, rather than the wedge where it has already
        // exited. Kill it and the sidecar, which would otherwise never receive
        // before-quit's SIGTERM.
        proc.kill('SIGKILL');
        if (sidecarPid != null) {
          try {
            process.kill(sidecarPid, 'SIGKILL');
          } catch {
            // already gone
          }
        }
      }
    }

    // Kill stray tmux servers before removing the home dir. The tmux socket
    // lives in the OS tmpdir (/tmp/tmux-<uid>/gridwell-<uuid>), not under home,
    // so rmSync would not clean it up.
    killTmuxServers(pluginUUIDs(home));

    // Fail here in this test's teardown rather than polluting the next test.
    await assertSidecarExited(sidecarPid);

    fs.rmSync(home, { recursive: true, force: true });
  },

  window: async ({ electronApp }, use) => {
    const win = await electronApp.firstWindow();
    // GRIDWELL_E2E_VERBOSE=1, which CI sets, mirrors the renderer console and
    // the Electron main and sidecar stdio into the worker's stdout, so a
    // runner-only failure leaves its app-side detail in the job log. The trace
    // records gestures, never the console.
    if (process.env.GRIDWELL_E2E_VERBOSE === '1') {
      win.on('console', (msg) => console.log(`[renderer:${msg.type()}] ${msg.text()}`));
      win.on('pageerror', (err) => console.log(`[renderer:pageerror] ${err.message}`));
      const proc = electronApp.process();
      proc.stdout?.on('data', (d: Buffer) => process.stdout.write(`[main] ${d}`));
      proc.stderr?.on('data', (d: Buffer) => process.stdout.write(`[main:err] ${d}`));
    }
    // The sidecar must report ready before the window opens, and the wasm must
    // boot and install the hook. Give the whole chain a generous budget.
    await win.waitForFunction(() => !!(window as any).__gridwellTest, null, { timeout: 30_000 });
    // Boot is not done at hook-install. The focused pane's anchor resolves
    // asynchronously, through Handshake and then HomeGrid, and a spec's first
    // focused() read can catch anchor="" on a slow boot and compare against
    // nothing. Ready means anchored.
    await win.waitForFunction(
      () => {
        const t = (window as any).__gridwellTest;
        try {
          return (t.panes() as Array<{ focused: boolean; anchor: string }>).some(
            (p) => p.focused && p.anchor !== '',
          );
        } catch {
          return false;
        }
      },
      null,
      { timeout: 30_000 },
    );
    await use(win);
  },

  gw: async ({ window, home }, use) => {
    const origin = new URL(window.url()).origin;
    setOracleAuth(origin, await loginToken(origin, homePassword(home)));
    await use(new GridwellDriver(window, origin));
  },
});

// Re-export expect so specs can import from './fixtures' without a second import.
export { expect } from '@playwright/test';
