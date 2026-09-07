import { BrowserWindow, Menu, screen } from 'electron';
import * as path from 'node:path';
import { rendererLogLine } from './viewutil';

interface RootWindow {
  win: BrowserWindow;
}

// createRootWindow builds the top-level window that hosts the Gridwell
// renderer, the wasm canvas app served by the sidecar.
//
// It is a BrowserWindow because a BrowserWindow's web contents auto-fills the
// window, so the canvas always matches the window size. A manually-sized
// WebContentsView drifts under tiling and HiDPI window managers such as WSLg,
// leaving the canvas at a stale size with bare window background showing.
//
// Live url tiles are WebContentsView children added to win.contentView and
// paint on top of the canvas. The WebviewRegistry takes this window unchanged.
export function createRootWindow(origin: string): RootWindow {
  // Gridwell is a mouse-only canvas. The default Electron application menu
  // eats space and, on Linux, renders inside the window.
  Menu.setApplicationMenu(null);

  // The primary display's work area, so a fresh launch never opens larger than
  // the screen. It is the fallback if the window manager ignores the maximize
  // request below.
  const { width, height } = screen.getPrimaryDisplay().workAreaSize;

  const win = new BrowserWindow({
    width,
    height,
    backgroundColor: '#0c0d11',
    title: 'Gridwell',
    autoHideMenuBar: true,
    minimizable: true,
    maximizable: true,
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      // sandbox:false lets the preload require its sibling ipc module for the
      // channel constants. The renderer is single-tenant, loopback-only and
      // first-party.
      sandbox: false,
    },
  });

  win.maximize();

  // GRIDWELL_E2E=1, set only by the Playwright e2e fixture, loads the renderer
  // with ?e2e=1 so the wasm client installs its read-only window.__gridwellTest
  // introspection surface. A normal launch never sets it.
  const query = process.env.GRIDWELL_E2E === '1' ? '?e2e=1' : '';
  void win.loadURL(origin + '/' + query);

  // The wasm client logs every surfaced errsurface notice to its own console
  // (reportErr), so forwarding warnings and errors here is what keeps those
  // failures in the app's log after the notice expires off the strip.
  win.webContents.on('console-message', (_e, level, message) => {
    const line = rendererLogLine(level, message);
    if (line) console.error(line);
  });

  // A fullscreen window can keep its old bounds when the display geometry
  // changes, leaving the canvas stretched or letterboxed. Re-fitting it to the
  // display's new bounds gives the renderer a resize event. Only while
  // fullscreen, because a normal window is the user's to size.
  screen.on('display-metrics-changed', () => {
    if (win.isDestroyed() || !win.isFullScreen()) return;
    const d = screen.getDisplayMatching(win.getBounds());
    win.setBounds(d.bounds);
  });

  // The removed application menu carried the F11 fullscreen accelerator, so it
  // is bound here, along with Ctrl/Cmd+M to minimize, which a window manager
  // may offer no decoration for. These act when the canvas has focus; a focused
  // url tile's native view handles its own keys, and webviews.ts mirrors F11
  // there.
  win.webContents.on('before-input-event', (event, input) => {
    if (input.type !== 'keyDown') return;
    if (input.key === 'F11') {
      win.setFullScreen(!win.isFullScreen());
      event.preventDefault();
    } else if ((input.control || input.meta) && (input.key === 'm' || input.key === 'M')) {
      win.minimize();
      event.preventDefault();
    }
  });

  return { win };
}
