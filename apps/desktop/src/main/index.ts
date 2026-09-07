import { app, BrowserWindow, dialog, session } from 'electron';
import { startSidecar, Sidecar } from './sidecar';
import { createRootWindow } from './window';
import { WebviewRegistry } from './webviews';
import { registerWebviewIpc, makeNavForwarder, makeOpenBelowForwarder, makeFreezeURLForwarder, makeContextMenuForwarder, makeZoomKeyForwarder, sendFrame, sendError } from './register';
import { MirrorPump } from './capture';
import { sanitizeUserAgent, allowPermission, SESSION_PARTITION } from './viewutil';
import { applyUserDataOverride } from './userdata';
import { sidecarExitMessage } from './sidecar-messages';
import { AUTH_COOKIE_NAME, AUTH_COOKIE_MAX_AGE_S } from './authconst';

// The e2e fixture also passes --user-data-dir=<home>/electron as a Chromium
// command-line switch (fixtures.ts), which is more reliable under Playwright's
// interception of app.isReady. This call covers a non-Playwright launch that
// sets GRIDWELL_HOME without the flag. See userdata.ts.
applyUserDataOverride((name, value) => app.setPath(name as Parameters<typeof app.setPath>[0], value), process.env);

// Chromium 137 dropped the silent SwiftShader fallback, so software WebGL needs
// this switch. Without it a GPU-less display (WSLg, xvfb) has no WebGL2, the
// terminal's WebGL renderer falls back to the canvas renderer, and shell text
// picks up rendering artifacts.
app.commandLine.appendSwitch('enable-unsafe-swiftshader');

// MIRROR_INTERVAL_MS is how often live views are captured and their frames
// pushed to the renderer, so other panes showing the same tile mirror live
// navigation. The live pane renders natively and ignores these frames. See
// MirrorPump in capture.ts for why the cadence is low.
const MIRROR_INTERVAL_MS = 250;

// Gridwell desktop entry. Boot order:
//   1. spawn the Go sidecar and wait for it to listen
//   2. open the root window pointing at the sidecar's loopback origin
//   3. tear the sidecar down on quit

let sidecar: Sidecar | null = null;
let registry: WebviewRegistry | null = null;
let pump: MirrorPump | null = null;
// quitting guards the post-boot sidecar exit listener below. before-quit
// SIGTERMs the sidecar, and that expected exit must not surface as a
// "backend crashed" notice while the window is already closing.
let quitting = false;

async function boot(): Promise<void> {
  // Before any view loads, so every url tile in every partition presents as
  // plain Chrome. userAgentFallback is the UA used when none is set
  // per-webContents.
  app.userAgentFallback = sanitizeUserAgent(app.userAgentFallback, app.getName());
  // allowPermission owns the decision. The deny covers every session, the root
  // renderer's default session included.
  const denyExternal = (ses: Electron.Session) =>
    ses.setPermissionRequestHandler((_wc, permission, callback) => {
      callback(allowPermission(permission));
    });
  denyExternal(session.defaultSession);
  denyExternal(session.fromPartition(SESSION_PARTITION));
  // No webContents may spawn a window. Live views get their own open-below
  // handler when the registry places them, replacing this one; every other
  // webContents denies. Gridwell code never calls window.open, so a call
  // reaching this default is a page trying to leave.
  app.on('web-contents-created', (_ev, wc) => {
    wc.setWindowOpenHandler(() => ({ action: 'deny' }));
  });
  try {
    // --no-server / GRIDWELL_NO_SERVER: never start a server, and discover a
    // separately-run one through `gridwell status`. The server owns the
    // per-home lock and the discovery banner.
    const noServer = process.argv.includes('--no-server') || process.env.GRIDWELL_NO_SERVER === '1';
    sidecar = await startSidecar({ noServer });
  } catch (err) {
    // The window is created below, only after the sidecar is up, so there is no
    // renderer to draw a notice-strip error into. dialog.showErrorBox is the
    // one surface available before boot, and without it a boot failure makes
    // the app vanish with no explanation.
    const message = err instanceof Error ? err.message : String(err);
    console.error('[gridwell] sidecar failed to start:', err);
    dialog.showErrorBox('Gridwell failed to start', message);
    app.exit(1);
    return;
  }
  // The web password gates other browsers on a shared origin, and must never
  // prompt this window. The serve banner carries the derived auth token, set
  // here as the cookie the server checks, before the window's first load.
  // Cookies scope by host and not by port, so it survives ephemeral-port churn
  // across launches.
  if (sidecar.auth) {
    await session.defaultSession.cookies.set({
      url: sidecar.origin,
      name: AUTH_COOKIE_NAME,
      value: sidecar.auth,
      // The server's own cookie shape; see authconst.ts.
      expirationDate: Math.floor(Date.now() / 1000) + AUTH_COOKIE_MAX_AGE_S,
      httpOnly: true,
      sameSite: 'lax',
    });
  }
  const { win } = createRootWindow(sidecar.origin);
  const rootWC = win.webContents;
  const reg = new WebviewRegistry(win, {
    onNav: makeNavForwarder(rootWC),
    onError: (ev) => sendError(rootWC, ev.source, ev.message),
    onOpenBelow: makeOpenBelowForwarder(rootWC),
    onFreezeURL: makeFreezeURLForwarder(rootWC),
    onContextMenu: makeContextMenuForwarder(rootWC),
    onZoomKey: makeZoomKeyForwarder(rootWC),
    // A live view took focus through page-initiated navigation: give it back
    // to the root renderer, where the user was typing.
    onFocusStolen: () => rootWC.focus(),
  });
  registry = reg;
  registerWebviewIpc(reg, rootWC, win);

  // A post-boot exit watch. startSidecar's own exit listener rejects the boot
  // promise only before it settles, so without this second listener a crash
  // after boot leaves the app a zombie: window open, backend gone, every RPC
  // failing with no explanation. An external server's probe child has already
  // exited by design, having only re-emitted the running holder's banner, so
  // watching it would report a phantom crash for a healthy backend.
  if (!sidecar.external) {
    sidecar.child.on('exit', (code, signal) => {
      if (quitting) return;
      sendError(rootWC, 'electron:backend', sidecarExitMessage(code, signal));
    });
  }

  // Under GRIDWELL_E2E=1 only, expose the registry so a Playwright spec running
  // in the main process can place a real live url view and drive the native
  // context-menu path. A WebContentsView is a separate webContents off the main
  // page, which the canvas-only harness cannot reach. __gwSidecarPid lets the
  // fixture assert the sidecar exited after app.close().
  if (process.env.GRIDWELL_E2E === '1') {
    (globalThis as { __gwRegistry?: WebviewRegistry; __gwSidecarPid?: number }).__gwRegistry = reg;
    (globalThis as { __gwSidecarPid?: number }).__gwSidecarPid = sidecar.child.pid;
  }

  // The renderer takes each frame into the tile's preview cache, and so into
  // every frozen pane showing that tile.
  pump = new MirrorPump(MIRROR_INTERVAL_MS, async () => {
    for (const paneId of reg.paneIds()) {
      const jpeg = await reg.capture(paneId);
      const tileId = reg.tileIdFor(paneId);
      if (jpeg && tileId !== undefined) {
        sendFrame(rootWC, paneId, tileId, jpeg);
      }
    }
  });
  pump.start();
}

// One app instance per user, using Electron's own lock alongside the server's
// per-home flock: a second launch hands off to the first and exits, and the
// first focuses its window. The e2e harness skips it and runs many isolated
// instances at once; each has its own home and user-data dir, and the serve
// flock still guards each home.
if (process.env.GRIDWELL_E2E !== '1' && !app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    const [win] = BrowserWindow.getAllWindows();
    if (win) {
      if (win.isMinimized()) win.restore();
      win.focus();
    }
  });
  app.whenReady().then(boot);
}

// On macOS the dock can reopen a window, so the process stays alive with no
// windows. Recreating the window from the dock is not implemented.
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});

// Quit is two-phase, because the renderer's unload flush needs both the views
// and the sidecar still alive. The first before-quit closes the windows and
// waits: each renderer's beforeunload runs its full flush of text, framing and
// url-state, then the views' own teardown flushes DOM storage, and only then
// does the sidecar stop. Tearing the views down or SIGTERMing the sidecar first
// loses the live tile's page, trail and unsaved text. A watchdog caps the wait
// so a wedged renderer cannot hold quit hostage.
let quitFlushed = false;
app.on('before-quit', (e) => {
  quitting = true;
  if (quitFlushed) return;
  e.preventDefault();
  const finish = (): void => {
    if (quitFlushed) return;
    quitFlushed = true;
    if (pump) {
      pump.stop();
      pump = null;
    }
    const reg = registry;
    registry = null;
    const done = (): void => {
      if (sidecar) {
        sidecar.stop();
        sidecar = null;
      }
      app.quit();
    };
    // removeAll runs for its localStorage flush. Its captures are moot, since
    // the renderers already beaconed their state.
    if (reg) void reg.removeAll().then(done, done);
    else done();
  };
  const watchdog = setTimeout(finish, 2000);
  const wins = BrowserWindow.getAllWindows();
  void Promise.all(
    wins.map(
      (w) =>
        new Promise<void>((res) => {
          if (w.isDestroyed()) return res();
          w.once('closed', () => res());
          w.close();
        }),
    ),
  ).then(() => {
    clearTimeout(watchdog);
    finish();
  });
});
