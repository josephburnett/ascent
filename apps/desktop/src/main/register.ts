import { ipcMain, BaseWindow, WebContents } from 'electron';
import {
  CH,
  EV,
  VIEW,
  PlaceArgs,
  SetBoundsArgs,
  SetHiddenArgs,
  SetZoomArgs,
  RemoveArgs,
  PaneRef,
  FreezeResult,
  ViewRightdown,
  ViewTouchScroll,
  ForwardedRightdown,
  ErrorEvent,
  OpenBelowEvent,
  FreezeURLEvent,
  ContextMenuEvent,
  ZoomKeyEvent,
} from './ipc';
import { WebviewRegistry } from './webviews';

// safeSend is the one guard every main-to-renderer push goes through. The
// window can close mid-flight, and calling .send on a destroyed WebContents
// throws.
function safeSend(wc: WebContents, channel: string, payload: unknown): void {
  if (!wc.isDestroyed()) wc.send(channel, payload);
}

// registerWebviewIpc connects the renderer-facing IPC channels to the registry.
// Call once after the root window is created. win's content bounds convert a
// live view's screen-space press into canvas coordinates.
export function registerWebviewIpc(
  registry: WebviewRegistry,
  rootWC: WebContents,
  win: BaseWindow,
): void {
  // A right-button press over a live url view begins a pane gesture. The
  // renderer starts the gesture and parks the view, so the rest of the drag
  // lands on the canvas.
  ipcMain.on(VIEW.rightdown, (_event, p: ViewRightdown): void => {
    const cb = win.getContentBounds();
    safeSend(rootWC, EV.rightForward, { x: p.sx - cb.x, y: p.sy - cb.y });
  });

  // A middle-button press over a live url view is the ascend gesture. The
  // renderer resolves the pane from the canvas coords and ascends.
  ipcMain.on(VIEW.middledown, (_event, p: ViewRightdown): void => {
    const cb = win.getContentBounds();
    safeSend(rootWC, EV.middleForward, { x: p.sx - cb.x, y: p.sy - cb.y });
  });

  // A left-button press over a live url view is a focus-transfer intent. The
  // preload forwards it without suppressing it, so the renderer can call
  // focusToPane and the click still reaches the page.
  ipcMain.on(VIEW.leftdown, (_event, p: ViewRightdown): void => {
    const cb = win.getContentBounds();
    const fwd: ForwardedRightdown = { x: p.sx - cb.x, y: p.sy - cb.y };
    safeSend(rootWC, EV.leftForward, fwd);
  });

  // A single-finger drag over a live url view. The registry injects an
  // equivalent mouseWheel back into that view, because Chromium will not
  // gesture-scroll raw touches there.
  ipcMain.on(VIEW.touchscroll, (event, p: ViewTouchScroll): void => {
    registry.touchScroll(event.sender, p);
  });

  ipcMain.handle(CH.place, (_e, a: PlaceArgs): Promise<void> => {
    return registry.place(a.paneId, a.tileId, a.url, a.bounds, a.contentZoom ?? 0, a.history ?? '', a.durable ?? false, a.hidden ?? false, a.focused ?? false);
  });

  ipcMain.handle(CH.setZoom, (_e, a: SetZoomArgs): void => {
    registry.setZoom(a.paneId, a.zoom);
  });

  ipcMain.handle(CH.setBounds, (_e, a: SetBoundsArgs): void => {
    registry.setBounds(a.paneId, a.bounds);
  });

  ipcMain.handle(CH.setHidden, (_e, a: SetHiddenArgs): void => {
    registry.setHidden(a.paneId, a.hidden, a.focused);
  });

  ipcMain.handle(CH.remove, async (_e, a: RemoveArgs): Promise<FreezeResult> => {
    return registry.remove(a.paneId);
  });

  ipcMain.handle(CH.goBack, (_e, a: PaneRef): void => {
    registry.goBack(a.paneId);
  });

  ipcMain.handle(CH.showMenu, (_e, a: PaneRef): void => {
    registry.showMenu(a.paneId);
  });

}

// makeNavForwarder returns a registry onNav callback that ships nav events
// to the renderer over EV.nav.
export function makeNavForwarder(rootWC: WebContents) {
  return (ev: { paneId: string; tileId: string; url: string; title: string }) => {
    safeSend(rootWC, EV.nav, ev);
  };
}

// makeOpenBelowForwarder relays a live view's new-window link to the renderer
// (EV.openBelow), which splits the pane and opens it as an ephemeral visit.
export function makeOpenBelowForwarder(rootWC: WebContents): (ev: OpenBelowEvent) => void {
  return (ev) => safeSend(rootWC, EV.openBelow, ev);
}

// makeFreezeURLForwarder relays the context menu's explicit freeze gesture to
// the renderer (EV.freezeUrl), where the wasm tears the view down and persists
// the standing frozen intent.
export function makeFreezeURLForwarder(rootWC: WebContents): (ev: FreezeURLEvent) => void {
  return (ev) => safeSend(rootWC, EV.freezeUrl, ev);
}

// makeContextMenuForwarder relays a live view's opening context menu to the
// renderer (EV.menuPane), where focusToPane, the one focus owner, moves focus
// to that pane before the menu can act.
export function makeContextMenuForwarder(rootWC: WebContents): (ev: ContextMenuEvent) => void {
  return (ev) => safeSend(rootWC, EV.menuPane, ev);
}

// makeZoomKeyForwarder relays the content-zoom chord from a focused live view
// to the renderer (EV.zoomKey), where the one zoom owner applies and persists
// it.
export function makeZoomKeyForwarder(rootWC: WebContents): (ev: ZoomKeyEvent) => void {
  return (ev) => safeSend(rootWC, EV.zoomKey, ev);
}

// sendFrame ships a mirror/capture frame to the renderer.
export function sendFrame(rootWC: WebContents, paneId: string, tileId: string, jpegBase64: string): void {
  if (jpegBase64) safeSend(rootWC, EV.frame, { paneId, tileId, jpegBase64 });
}

// sendError is the one main-process entry point onto EV.error. Every failure
// site calls it, so client/errsurface in the wasm client is the single place a
// main-process failure becomes visible.
export function sendError(rootWC: WebContents, source: string, message: string): void {
  // The log line too: a main-process failure must reach the app's log even when
  // the renderer is gone, where safeSend no-ops.
  console.error(`[gridwell] ${source}: ${message}`);
  const ev: ErrorEvent = { source, message };
  safeSend(rootWC, EV.error, ev);
}
