// Injected into every live url WebContentsView, where it tells a right-click
// from a right-drag over live web content. A plain right-click passes through,
// so webviews.ts pops the context menu; a right-drag arms a pane gesture,
// forwarded to main at the press point. Nothing is suppressed on right-down:
// only once the cursor moves past the drag threshold with the right button
// still held is the gesture forwarded and the would-be context menu suppressed.
// The middle button is always ascend. Left button, wheel, keyboard and
// selection stay with the page.
//
// This runs inside arbitrary web pages, so the view stays sandboxed. A
// sandboxed, isolated preload may use electron's ipcRenderer but may not
// require local modules, so the channel names and the thresholds are duplicated
// from ../main/ipc.ts and ../main/viewutil.ts.
import { ipcRenderer } from 'electron';

// Keep in sync with VIEW.rightdown / VIEW.middledown / VIEW.leftdown /
// VIEW.touchscroll in ../main/ipc.ts.
const VIEW_RIGHTDOWN = 'gw:view-rightdown';
const VIEW_MIDDLEDOWN = 'gw:view-middledown';
const VIEW_LEFTDOWN = 'gw:view-leftdown';
const VIEW_TOUCHSCROLL = 'gw:view-touchscroll';
// Keep in sync with RIGHT_DRAG_THRESHOLD, RIGHT_DRAG_TIME_MS and
// RIGHT_DRAG_FAR_THRESHOLD in ../main/viewutil.ts, which owns what each one is
// for. gesture-threshold.test.ts lints the copies.
const RIGHT_DRAG_THRESHOLD = 4;
const RIGHT_DRAG_TIME_MS = 200;
const RIGHT_DRAG_FAR_THRESHOLD = 24;
// MouseEvent.buttons bit for the secondary (right) button.
const RIGHT_BUTTON_MASK = 2;

let rightDown = false;
let rightDragged = false;
let rightStartX = 0;
let rightStartY = 0;
let rightDownTime = 0;

// Capture phase at the window, so this fires before the page's own listeners.
// screenX and screenY are physical screen pixels, unaffected by the page's
// zoomFactor; main converts them to window coords through getContentBounds.
window.addEventListener(
  'mousedown',
  (e: MouseEvent) => {
    if (e.button === 2) {
      // Not suppressed, because a plain right-click must reach the page.
      // mousemove decides whether this becomes a drag gesture.
      rightDown = true;
      rightDragged = false;
      rightStartX = e.screenX;
      rightStartY = e.screenY;
      rightDownTime = Date.now();
    } else if (e.button === 1) {
      e.preventDefault();
      e.stopPropagation();
      ipcRenderer.send(VIEW_MIDDLEDOWN, { sx: e.screenX, sy: e.screenY });
    } else if (e.button === 0) {
      // A focus intent for the renderer. The click is not suppressed, so
      // in-page interaction, selection and links stay with the page.
      ipcRenderer.send(VIEW_LEFTDOWN, { sx: e.screenX, sy: e.screenY });
    }
  },
  true,
);

window.addEventListener(
  'mousemove',
  (e: MouseEvent) => {
    if (!rightDown) return;
    // The right button was released elsewhere, for instance while the view was
    // parked after a prior drag, so a later move must not fake a drag.
    if ((e.buttons & RIGHT_BUTTON_MASK) === 0) {
      rightDown = false;
      return;
    }
    if (rightDragged) return;
    const dx = e.screenX - rightStartX;
    const dy = e.screenY - rightStartY;
    // Mirrors viewutil.classifyRightPress, which a preload cannot import.
    const d2 = dx * dx + dy * dy;
    if (
      d2 > RIGHT_DRAG_FAR_THRESHOLD * RIGHT_DRAG_FAR_THRESHOLD ||
      (d2 > RIGHT_DRAG_THRESHOLD * RIGHT_DRAG_THRESHOLD &&
        Date.now() - rightDownTime >= RIGHT_DRAG_TIME_MS)
    ) {
      // The original press point, so main classifies the gesture where the
      // press began. Main then parks the view and the rest of the drag lands
      // on the canvas.
      rightDragged = true;
      ipcRenderer.send(VIEW_RIGHTDOWN, { sx: rightStartX, sy: rightStartY });
    }
  },
  true,
);

window.addEventListener(
  'mouseup',
  (e: MouseEvent) => {
    if (e.button === 2) rightDown = false;
  },
  true,
);

// Single-finger touch scroll. Chromium does not synthesize scroll gestures from
// raw touches inside an embedded WebContentsView: they reach the page as
// TouchEvents and nothing scrolls. Each move's delta goes to main, which injects
// an equivalent mouseWheel back into this view at the finger's position.
// preventDefault on the claimed moves keeps Chromium's touch-to-mouse
// compatibility from turning the drag into a text selection, and keeps a
// platform with working native touch scrolling from scrolling twice.
// Multi-finger touches are left alone, so pinch zoom stays the page's.
let touchScrolling = false;
let touchLastX = 0;
let touchLastY = 0;

window.addEventListener(
  'touchstart',
  (e: TouchEvent) => {
    if (e.touches.length !== 1) {
      touchScrolling = false;
      return;
    }
    touchScrolling = true;
    touchLastX = e.touches[0].screenX;
    touchLastY = e.touches[0].screenY;
  },
  true,
);

window.addEventListener(
  'touchmove',
  (e: TouchEvent) => {
    if (!touchScrolling || e.touches.length !== 1) return;
    const t = e.touches[0];
    const dx = t.screenX - touchLastX;
    const dy = t.screenY - touchLastY;
    touchLastX = t.screenX;
    touchLastY = t.screenY;
    if (dx === 0 && dy === 0) return;
    e.preventDefault();
    ipcRenderer.send(VIEW_TOUCHSCROLL, { sx: t.screenX, sy: t.screenY, dx, dy });
  },
  // Window touch listeners default to passive, and a passive listener's
  // preventDefault is ignored.
  { capture: true, passive: false },
);

window.addEventListener(
  'touchend',
  (e: TouchEvent) => {
    if (e.touches.length === 0) touchScrolling = false;
  },
  true,
);

// Preventing the page's `contextmenu` event stops Chromium from emitting the
// webContents `context-menu` event, so webviews.ts pops no menu mid-gesture. A
// plain right-click falls through. On Linux and Windows `contextmenu` fires on
// right-button-up, after the drag has been detected.
window.addEventListener(
  'contextmenu',
  (e: Event) => {
    if (rightDragged) e.preventDefault();
  },
  true,
);
