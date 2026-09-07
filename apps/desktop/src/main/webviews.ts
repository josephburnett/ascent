import { BaseWindow, WebContentsView, Menu, clipboard, session, WebContents } from 'electron';
import type { MenuItemConstructorOptions } from 'electron';
import * as path from 'node:path';
import type { Bounds, FreezeResult, NavEvent, ErrorEvent, OpenBelowEvent, FreezeURLEvent, ContextMenuEvent, ZoomKeyEvent } from './ipc';
import {
  SESSION_PARTITION,
  roundBounds,
  boundsEqual,
  parkedBounds,
  minWidthZoomFactor,
  composeZoom,
  serializeHistory,
  reviveNavigation,
  URL_MIN_LAYOUT_WIDTH,
  shouldSurfaceFailLoad,
  failLoadMessage,
  renderProcessGoneMessage,
  zoomChordKey,
  openBelowUrl,
} from './viewutil';
import { urlContextMenuTemplate } from './contextmenu';
import { captureAttempt, captureJpegBase64, describeAttempt } from './capture';
import { decideStreak, FRESH, StreakState } from './capturestreak';
import { decideFocus, isPressInput, GuardPhase } from './focusguard';

// urlViewPreload is the script injected into every live url view. __dirname is
// dist/main at runtime, so the compiled preload sits one level up.
const urlViewPreload = path.join(__dirname, '..', 'preload', 'urlview-preload.js');

interface Entry {
  view: WebContentsView;
  tileId: string;
  bounds: Bounds;
  hidden: boolean;
  // focused is whether this pane is the focused pane, as the renderer last
  // reported it through setHidden. The focus-steal guard reads it.
  focused: boolean;
  // userZoom is the tile's persisted content zoom, composed with the min-width
  // layout zoom in applyMinWidthZoom. 0 means unset, i.e. 1.0.
  userZoom: number;
  // presses counts the press-shaped input events Chromium has routed into this
  // view (focusguard.isPressInput). Main sees them in the browser process
  // before the renderer receives the press, so no page can delay or suppress
  // the count the focus guard reads.
  presses: number;
  // durable is whether the tile behind this view survives ascent. An ephemeral
  // visit is not durable and has nothing to re-descend into, so the context
  // menu offers no Freeze Page there.
  durable: boolean;
  // focusSettle is the steal guard's pending settle timer, tracked so remove()
  // can cancel it. The closure holds the view, and firing after
  // webContents.close() would throw uncaught in main.
  focusSettle: ReturnType<typeof setTimeout> | null;
  // captureStreak is this pane's mirror-capture state; capturestreak.ts owns
  // its shape and decideStreak is the only thing that moves it.
  captureStreak: StreakState;
}

interface RegistryCallbacks {
  // onNav fires when a hosted view finishes a navigation, changing url or
  // title, so the renderer can update the cached tile address.
  onNav?: (ev: NavEvent) => void;
  // onError fires for every webview failure the registry detects: did-fail-load,
  // render-process-gone, a crash during remove(). index.ts wires it to
  // sendError, the one path onto EV.error. The registry knows nothing of IPC.
  onError?: (ev: ErrorEvent) => void;
  // onOpenBelow fires when a hosted view's page tries to open a new window
  // through target=_blank, window.open, or a ctrl/cmd-click. The renderer
  // splits the pane and opens the url as an ephemeral visit below.
  onOpenBelow?: (ev: OpenBelowEvent) => void;
  // onFreezeURL fires when the user picks "Freeze Page" in a live view's
  // context menu; the renderer freezes and stores the intent.
  onFreezeURL?: (ev: FreezeURLEvent) => void;
  // onContextMenu fires just before a live view's context menu opens, naming the
  // pane it acts in, so the renderer can move focus there first. Both doors into
  // the menu announce through here, the only place that knows a menu is opening.
  onContextMenu?: (ev: ContextMenuEvent) => void;
  // onZoomKey fires when the content-zoom chord (Ctrl/Cmd with +, =, - or 0) is
  // pressed while this view owns OS keyboard focus. The renderer's
  // applyContentZoom, the one owner of the cache and the write, handles it.
  onZoomKey?: (ev: ZoomKeyEvent) => void;
  // onFocusStolen fires when a live view acquired OS keyboard focus with no user
  // action on its pane; focusguard.ts owns that verdict. index.ts hands focus
  // back to the root window's webContents, where the canvas and every shell
  // overlay live.
  onFocusStolen?: (ev: { paneId: string }) => void;
}

// WebviewRegistry owns the live url-tile WebContentsViews parented to the root
// window. One view per paneId, and every view browses on the one host-local
// persistent partition (SESSION_PARTITION). The registry knows nothing of IPC
// or the store: register.ts wires the Electron handlers to these methods, and
// the renderer stays the only thing that talks to the Go backend.
export class WebviewRegistry {
  private readonly win: BaseWindow;
  private readonly cb: RegistryCallbacks;
  private readonly entries = new Map<string, Entry>();
  // Count of zoom chords seen by before-input-event and relayed to the renderer.
  // The e2e reads it through __gwRegistry as a delivery ack: a synthetic
  // sendInputEvent that never bumps it was lost in the input pipeline, an xvfb
  // artifact, while a bump with no zoom effect is a real relay bug.
  zoomChordRelays = 0;

  constructor(win: BaseWindow, cb: RegistryCallbacks = {}) {
    this.win = win;
    this.cb = cb;
  }

  // toggleFullScreen flips the host window's fullscreen state. The F11 handler
  // injected into live url views calls it, because the canvas's own F11 handler
  // cannot see the key while a native view has focus.
  private toggleFullScreen(): void {
    this.win.setFullScreen(!this.win.isFullScreen());
  }

  // showContextMenu pops the live url view's right-click menu. contextmenu.ts
  // owns which items appear and what each does; this binds the actions to the
  // real clipboard and webContents. params is the subset of ContextMenuParams
  // the template reads, which ContextMenuParams satisfies structurally.
  private showContextMenu(
    paneId: string,
    view: WebContentsView,
    params: {
      linkURL: string;
      selectionText: string;
      isEditable: boolean;
      editFlags: { canCut: boolean; canCopy: boolean; canPaste: boolean };
    },
  ): void {
    // Announced before the pop, so this pane is the focused one by the time any
    // item runs, and a menu dismissed without a pick has still moved focus, as
    // a bare left-click does.
    this.cb.onContextMenu?.({ paneId });
    const wc = view.webContents;
    const nav = wc.navigationHistory;
    const template = urlContextMenuTemplate(
      {
        linkURL: params.linkURL,
        selectionText: params.selectionText,
        isEditable: params.isEditable,
        editFlags: {
          canCut: params.editFlags.canCut,
          canCopy: params.editFlags.canCopy,
          canPaste: params.editFlags.canPaste,
        },
        canGoBack: nav.canGoBack(),
        canGoForward: nav.canGoForward(),
        // Only a durable tile can hold the freeze intent; an ephemeral visit
        // has nothing to re-descend into.
        canFreeze: this.entries.get(paneId)?.durable ?? false,
      },
      {
        copyText: (t) => clipboard.writeText(t),
        copyLink: (u) => clipboard.writeText(u),
        openLink: (u) => void wc.loadURL(u),
        cut: () => wc.cut(),
        paste: () => wc.paste(),
        back: () => this.goBack(paneId),
        forward: () => {
          if (nav.canGoForward()) nav.goForward();
        },
        reload: () => wc.reload(),
        freeze: () => this.cb.onFreezeURL?.({ paneId }),
      },
    );
    const menu = Menu.buildFromTemplate(template as MenuItemConstructorOptions[]);
    menu.popup({ window: this.win });
  }

  // showMenu is the bar circle's right-click door onto the same context menu,
  // with no in-page context. A page can hijack contextmenu and make the in-page
  // path unreachable, while the circle sits on the canvas outside the view's
  // rect, so this path always reaches Freeze Page.
  showMenu(paneId: string): void {
    const e = this.entries.get(paneId);
    if (!e) return;
    this.showContextMenu(paneId, e.view, {
      linkURL: '',
      selectionText: '',
      isEditable: false,
      editFlags: { canCut: false, canCopy: false, canPaste: false },
    });
  }

  has(paneId: string): boolean {
    return this.entries.has(paneId);
  }

  paneIds(): string[] {
    return [...this.entries.keys()];
  }

  // tileIdFor returns the tile id hosted in paneId, or undefined.
  tileIdFor(paneId: string): string | undefined {
    return this.entries.get(paneId)?.tileId;
  }

  // focusedFor reports whether the registry believes paneId is the focused pane.
  // The renderer owns the fact and carries it on both place and setHidden.
  focusedFor(paneId: string): boolean | undefined {
    return this.entries.get(paneId)?.focused;
  }

  // webContentsFor is a test-only accessor returning the webContents behind a
  // pane, so a harness can drive real Chromium focus and input against it.
  webContentsFor(paneId: string): WebContents | undefined {
    return this.entries.get(paneId)?.view.webContents;
  }

  // viewBoundsFor is a test-only accessor returning the view's physical bounds
  // as Electron last set them, which says whether the view is parked or at its
  // visible position.
  viewBoundsFor(paneId: string): { x: number; y: number; width: number; height: number } | undefined {
    const e = this.entries.get(paneId);
    if (!e) return undefined;
    // View inherits getBounds() from Electron's View base class.
    return (e.view as unknown as { getBounds(): { x: number; y: number; width: number; height: number } }).getBounds();
  }

  // place creates the view for paneId. The view is a child of the window's
  // contentView, so it paints above the root canvas renderer at the given
  // bounds. Later bounds changes arrive through setBounds, every frame from
  // syncURLViews. A place() for a pane that already holds a view is a renderer
  // bug and is reported: url_stream_client.go returns early for the tile
  // already live in the pane and closes any other view first, so nothing
  // legitimate reaches that branch.
  async place(paneId: string, tileId: string, url: string, bounds: Bounds, contentZoom = 0, history = '', durable = false, hidden = false, focused = false): Promise<void> {
    const rounded = roundBounds(bounds);
    const partition = SESSION_PARTITION;
    const stale = this.entries.get(paneId);
    if (stale) {
      // The renderer closes a pane's live view first, through closeURLStream,
      // the one path that persists a freeze. Reaching here means a view was
      // replaced without its close, so the freeze this remove() returns has no
      // caller to land in and a frame is lost.
      this.cb.onError?.({
        source: 'electron:webview',
        message: `pane ${paneId}: live view replaced (${stale.tileId} → ${tileId}) without a close; its final frame is lost`,
      });
      await this.remove(paneId).catch(() => {});
    }
    const view = new WebContentsView({
      webPreferences: {
        partition,
        contextIsolation: true,
        nodeIntegration: false,
        // Stacked levels keep their views running while parked off-screen, so
        // a hidden call keeps ringing. Chromium would otherwise throttle an
        // occluded page's timers.
        backgroundThrottling: false,
        preload: urlViewPreload,
      },
    });
    // Everything Chromium would open as a new window or tab arrives here, and
    // none of it spawns a detached BrowserWindow. The url goes to the renderer,
    // which splits the pane and opens it as an ephemeral visit below.
    // openBelowUrl filters to web urls only, matching the session's
    // openExternal deny.
    view.webContents.setWindowOpenHandler(({ url: target }) => {
      const below = openBelowUrl(target);
      if (below) {
        this.cb.onOpenBelow?.({ paneId, url: below });
      }
      return { action: 'deny' };
    });
    // window.ts handles F11 on the canvas, but a focused live url view owns OS
    // keyboard focus, so that handler never sees the key. The content-zoom chord
    // is intercepted the same way and relayed to the renderer, where
    // applyContentZoom updates the cache and persists. Calling registry.setZoom
    // from main would move the view and skip both.
    view.webContents.on('before-input-event', (event, input) => {
      if (input.type !== 'keyDown') return;
      if (input.key === 'F11') {
        this.toggleFullScreen();
        event.preventDefault();
        return;
      }
      const key = zoomChordKey(input);
      if (key) {
        this.zoomChordRelays++;
        this.cb.onZoomKey?.({ paneId, key });
        event.preventDefault();
      }
    });
    // A WebContentsView has no default context menu and only emits this event.
    // The injected preload suppresses it for a right-drag, which is a pane
    // gesture, so reaching here means a real click.
    view.webContents.on('context-menu', (_event, params) => this.showContextMenu(paneId, view, params));
    // hidden and focused both start from the renderer's verdict for this frame
    // (PlaceArgs), because the renderer owns both facts. hidden parks a view
    // placed while the palette is open or during a drag gesture instead of
    // landing it on top of the canvas overlay. focused feeds the steal guard
    // from the first frame: addChildView and loadURL hand the new widget OS
    // keyboard focus, and a placement on an unfocused pane, such as a workspace
    // restore walking every leaf, must bounce it straight back. syncURLViews
    // calls setHidden for this pane on the next draw() and reaffirms both.
    const startHidden = hidden;
    const e: Entry = { view, tileId, bounds: rounded, hidden: startHidden, focused, userZoom: contentZoom, presses: 0, durable, focusSettle: null, captureStreak: FRESH };
    this.entries.set(paneId, e);
    this.win.contentView.addChildView(view);
    view.setBounds(startHidden ? parkedBounds(rounded.width, rounded.height) : rounded);
    this.wireNav(paneId, e);
    this.applyMinWidthZoom(e);
    // reviveNavigation owns the tie-break between the persisted back-stack and
    // the tile's user-editable address.
    const nav = reviveNavigation(url, history);
    if (nav.kind === 'restore') {
      void view.webContents.navigationHistory.restore({ entries: nav.history.entries, index: nav.history.index });
    } else {
      void view.webContents.loadURL(url);
    }
  }

  setBounds(paneId: string, bounds: Bounds): void {
    const e = this.entries.get(paneId);
    if (!e) return;
    const rounded = roundBounds(bounds);
    if (boundsEqual(e.bounds, rounded)) return;
    e.bounds = rounded;
    if (!e.hidden) {
      e.view.setBounds(rounded);
    }
    this.applyMinWidthZoom(e);
  }

  // touchScroll injects one step of a single-finger drag as a mouseWheel into
  // the view whose preload forwarded it, because Chromium does not
  // gesture-scroll raw touches inside an embedded WebContentsView (see
  // urlview-preload.ts). The finger's screen position converts to view-local
  // coords so the wheel lands on the scrollable element under the finger. The
  // content follows the finger, which under sendInputEvent's wheel convention
  // is the finger's own delta; the capture harness pins the sign.
  touchScroll(sender: WebContents, p: { sx: number; sy: number; dx: number; dy: number }): void {
    for (const e of this.entries.values()) {
      if (e.view.webContents !== sender) continue;
      const cb = this.win.getContentBounds();
      e.view.webContents.sendInputEvent({
        type: 'mouseWheel',
        x: p.sx - cb.x - e.bounds.x,
        y: p.sy - cb.y - e.bounds.y,
        deltaX: p.dx,
        deltaY: p.dy,
        // Precise, touchpad-style deltas, so the page tracks the finger 1:1
        // instead of running the wheel's animated smoothing.
        hasPreciseScrollingDeltas: true,
      });
      return;
    }
  }

  // applyMinWidthZoom keeps a narrow url pane from reflowing the page to a
  // cramped mobile layout; minWidthZoomFactor owns the arithmetic. zoomFactor
  // resets on cross-origin navigation, so wireNav re-applies it on load.
  private applyMinWidthZoom(e: Entry): void {
    const z = composeZoom(minWidthZoomFactor(e.bounds.width, URL_MIN_LAYOUT_WIDTH), e.userZoom);
    try {
      e.view.webContents.setZoomFactor(z);
    } catch {
      // webContents not ready yet; wireNav re-applies on did-finish-load.
    }
  }

  // setZoom updates the user content zoom for the pane's live view, the tile's
  // content_zoom, and re-applies the composed factor.
  setZoom(paneId: string, zoom: number): void {
    const e = this.entries.get(paneId);
    if (!e) return;
    e.userZoom = zoom;
    this.applyMinWidthZoom(e);
  }

  // setHidden shows or hides the view without destroying it, and tracks whether
  // the pane is focused. `hidden` parks the whole view off-screen during drag
  // gestures and modals, so canvas-drawn overlays such as the palette can paint
  // where the native view would otherwise sit on top. `focused` feeds the
  // focus-steal guard. syncURLViews calls this every frame, so it no-ops when
  // nothing changed.
  setHidden(paneId: string, hidden: boolean, focused: boolean): void {
    const e = this.entries.get(paneId);
    if (!e || (e.hidden === hidden && e.focused === focused)) return;
    const viewChanged = e.hidden !== hidden;
    e.hidden = hidden;
    e.focused = focused;
    if (viewChanged) {
      if (hidden) {
        e.view.setBounds(parkedBounds(e.bounds.width, e.bounds.height));
      } else {
        e.view.setBounds(e.bounds);
      }
    }
  }

  // remove captures a final frame plus the page's url and title, detaches and
  // destroys the view, and returns the freeze payload for persistence.
  async remove(paneId: string): Promise<FreezeResult> {
    const e = this.entries.get(paneId);
    if (!e) return { jpegBase64: '', url: '', title: '', history: '' };
    this.entries.delete(paneId);
    // The settle timer's closure holds this view, and firing after close() would
    // throw uncaught in main.
    if (e.focusSettle) {
      clearTimeout(e.focusSettle);
      e.focusSettle = null;
    }

    // Chromium writes cookies eagerly but flushes localStorage lazily, so an
    // abrupt webContents.close() can drop recent localStorage writes, where a
    // site keeps something like an unsubmitted comment draft. Flushing here is
    // what makes such a draft survive ascend, descend and go-live.
    try {
      session.fromPartition(SESSION_PARTITION).flushStorageData();
    } catch {
      // The durable partition flushes on quit regardless.
    }

    let jpegBase64 = '';
    let url = '';
    let title = '';
    let history = '';
    try {
      url = e.view.webContents.getURL();
      title = e.view.webContents.getTitle();
      // The navigation back-stack, persisted so a revived tile can still go
      // back.
      const nav = e.view.webContents.navigationHistory;
      history = serializeHistory(nav.getAllEntries(), nav.getActiveIndex());
      jpegBase64 = await captureJpegBase64(e.view);
    } catch {
      // A crashed or destroyed view yields an empty freeze, which bridgeRemove
      // in client/wasm/url_stream_client.go drops rather than writing back, so
      // it cannot overwrite a good preview with a blank one. The crash itself
      // must surface, so the user knows why the tile fell back to its last good
      // preview.
      this.cb.onError?.({
        source: 'electron:webview',
        message: 'view crashed while closing — preview not updated',
      });
    } finally {
      // Runs even when the capture above threw or timed out. The renderer has
      // already dropped this pane from its live set, so a view left attached
      // would sit blank on top of the pane the user just ascended out of.
      try {
        this.win.contentView.removeChildView(e.view);
        e.view.webContents.close();
      } catch (err) {
        // The detach failed, so a live view is left sitting on top of the pane
        // the user ascended out of. The blank rectangle comes with its cause.
        this.cb.onError?.({
          source: 'electron:webview',
          message: 'failed to detach live view — ascend may leave a blank overlay: ' + String(err),
        });
      }
    }
    return { jpegBase64, url, title, history };
  }

  // capture grabs a current frame for mirroring to other panes, without tearing
  // the view down. It returns '' for a pane with no live view and for any failed
  // attempt. Every outcome goes to capturestreak, which owns whether this is a
  // new streak, a continuing one or a recovery. A hidden pane is not an attempt,
  // so it neither opens nor closes a streak.
  async capture(paneId: string): Promise<string> {
    const e = this.entries.get(paneId);
    if (!e || e.hidden) return '';
    const attempt = await captureAttempt(e.view);
    const decision = decideStreak(e.captureStreak, attempt.kind);
    e.captureStreak = decision.state;
    const report = decision.report;
    if (report) {
      // A recovery is reported too, or the failing report reads as permanent.
      const message =
        report.kind === 'failing'
          ? `pane ${paneId}: mirror capture failing: ${describeAttempt(attempt)}`
          : `pane ${paneId}: mirror capture recovered after ${report.afterFailures} failed ` +
            `${report.afterFailures === 1 ? 'capture' : 'captures'}`;
      this.cb.onError?.({ source: 'electron:webview', message });
    }
    return attempt.kind === 'ok' ? attempt.jpegBase64 : '';
  }

  // goBack is the one back action for a live view. The bar's back button and the
  // context menu's Back both land here, and it no-ops at the start of history.
  goBack(paneId: string): void {
    const e = this.entries.get(paneId);
    if (!e) return;
    const nav = e.view.webContents.navigationHistory;
    if (nav.canGoBack()) nav.goBack();
  }

  // removeAll tears everything down, on app quit or window close.
  async removeAll(): Promise<void> {
    await Promise.all(this.paneIds().map((id) => this.remove(id)));
  }

  private wireNav(paneId: string, e: Entry): void {
    const emit = () => {
      this.cb.onNav?.({
        paneId,
        tileId: e.tileId,
        url: e.view.webContents.getURL(),
        title: e.view.webContents.getTitle(),
      });
    };
    e.view.webContents.on('did-navigate', emit);
    e.view.webContents.on('did-navigate-in-page', emit);
    e.view.webContents.on('page-title-updated', emit);
    // Every press Chromium routes into this view. isPressInput excludes the
    // registry's own mouseWheel injection.
    e.view.webContents.on('input-event', (_event, input) => {
      if (isPressInput(input.type)) e.presses++;
    });
    // A page-initiated navigation makes Chromium focus the new document's
    // widget, taking OS keyboard focus from whatever the user was typing in.
    // The grab can land, and re-land, asynchronously after any one navigation
    // event, so the guard sits on the focus event itself and asks focusguard,
    // which owns the verdict and why it is deferred.
    //
    // remove() cancels the settle timer, but only for a teardown that went
    // through the registry. A view can also die under it, from a render-process
    // crash or a host-side close, and every read of a destroyed WebContents
    // throws, uncaught inside a timer, which hangs main behind an error dialog.
    // So the settle reads isFocused() inside a catch.
    const step = (phase: GuardPhase, pressesAtFocus: number, alreadyBounced: boolean): void => {
      let viewHoldsOSFocus = true; // at 'focus-event' the event is the evidence
      if (phase === 'settle') {
        try {
          viewHoldsOSFocus = e.view.webContents.isFocused();
        } catch {
          return; // the view died between the grab and this settle
        }
      }
      const act = decideFocus({
        phase,
        paneFocused: e.focused,
        viewHoldsOSFocus,
        pressesAtFocus,
        pressesNow: e.presses,
        alreadyBounced,
      });
      if (act.kind === 'allow') return;
      if (act.kind === 'bounce') this.cb.onFocusStolen?.({ paneId });
      if (act.settleMs === null) return;
      if (e.focusSettle) clearTimeout(e.focusSettle);
      const bounced = act.kind === 'bounce';
      e.focusSettle = setTimeout(() => {
        e.focusSettle = null;
        if (this.entries.get(paneId) !== e) return; // removed meanwhile
        step('settle', pressesAtFocus, bounced);
      }, act.settleMs);
    };
    e.view.webContents.on('focus', () => step('focus-event', e.presses, false));
    // zoomFactor resets across cross-origin navigations, so re-apply the
    // min-width zoom once the new document has loaded.
    e.view.webContents.on('did-finish-load', () => this.applyMinWidthZoom(e));

    // shouldSurfaceFailLoad owns which of these events reach the user.
    e.view.webContents.on(
      'did-fail-load',
      (_event, errorCode, errorDescription, validatedURL, isMainFrame) => {
        if (!shouldSurfaceFailLoad(errorCode, isMainFrame)) return;
        this.cb.onError?.({
          source: 'electron:webview',
          message: failLoadMessage(validatedURL, errorDescription, errorCode),
        });
      },
    );

    // render-process-gone means the renderer process crashed, and unreported the
    // view just sits blank. getURL() after a crash may throw, which must not
    // stop the notice.
    e.view.webContents.on('render-process-gone', (_event, details) => {
      let url = '';
      try {
        url = e.view.webContents.getURL();
      } catch {
        // renderProcessGoneMessage handles an empty url cleanly
      }
      this.cb.onError?.({
        source: 'electron:webview',
        message: renderProcessGoneMessage(url, details.reason),
      });
    });
  }
}

