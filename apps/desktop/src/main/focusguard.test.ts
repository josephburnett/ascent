import { test } from 'node:test';
import assert from 'node:assert/strict';
import { decideFocus, isPressInput, FOCUS_SETTLE_MS, GuardInput, GuardAction } from './focusguard';

// An unfocused pane whose view just grabbed OS focus with no press behind it,
// which is the steal shape. Each case overrides only what it is about.
function input(over: Partial<GuardInput> = {}): GuardInput {
  return {
    phase: 'focus-event',
    paneFocused: false,
    viewHoldsOSFocus: true,
    pressesAtFocus: 0,
    pressesNow: 0,
    alreadyBounced: false,
    ...over,
  };
}

const TABLE: { name: string; in: GuardInput; want: GuardAction }[] = [
  {
    name: '1. the focused pane keeps its view focused',
    in: input({ paneFocused: true }),
    want: { kind: 'allow' },
  },
  {
    name: '2. a grab on an unfocused pane decides nothing yet',
    // At the focus event the press that may explain it has not been dispatched,
    // so bouncing here would report a steal for the user's own first click.
    in: input(),
    want: { kind: 'wait', settleMs: FOCUS_SETTLE_MS },
  },
  {
    name: '3. still unfocused at the settle with no press: a steal',
    in: input({ phase: 'settle' }),
    want: { kind: 'bounce', settleMs: FOCUS_SETTLE_MS },
  },
  {
    name: '4. the confirmation bounces once and stops chaining',
    in: input({ phase: 'settle', alreadyBounced: true }),
    want: { kind: 'bounce', settleMs: null },
  },
  {
    name: '5. the pane became focused between the grab and the settle',
    in: input({ phase: 'settle', paneFocused: true }),
    want: { kind: 'allow' },
  },
  {
    name: '6. the view no longer holds OS focus at the settle',
    in: input({ phase: 'settle', viewHoldsOSFocus: false }),
    want: { kind: 'allow' },
  },
  {
    name: '7. a press landed between the grab and the settle',
    // The user's click. Chromium focused the widget while routing the press and
    // forwarded the press afterwards, so the press arrives second.
    in: input({ phase: 'settle', pressesAtFocus: 0, pressesNow: 1 }),
    want: { kind: 'allow' },
  },
  {
    name: '8. a press already counted at the focus event (press-first host)',
    in: input({ pressesAtFocus: 3, pressesNow: 4 }),
    want: { kind: 'allow' },
  },
  {
    name: '9. an older press, already consumed by an earlier focus, is not intent',
    // The count is equal and non-zero, so this view was clicked before but not
    // for this focus. A guard keyed on how recently a click happened would let
    // a page steal focus in the shadow of an earlier click.
    in: input({ phase: 'settle', pressesAtFocus: 7, pressesNow: 7 }),
    want: { kind: 'bounce', settleMs: FOCUS_SETTLE_MS },
  },
  {
    name: '10. a press during the confirmation stops the second bounce',
    in: input({ phase: 'settle', alreadyBounced: true, pressesAtFocus: 2, pressesNow: 3 }),
    want: { kind: 'allow' },
  },
];

for (const c of TABLE) {
  test(`decideFocus — ${c.name}`, () => {
    assert.deepEqual(decideFocus(c.in), c.want);
  });
}

test('decideFocus reads no clock: the same world always decides the same way', () => {
  // Nothing in GuardInput is a time, so asking twice cannot disagree.
  for (const c of TABLE) {
    assert.deepEqual(decideFocus(c.in), decideFocus(c.in), c.name);
  }
});

test('isPressInput counts presses only, never the registry own wheel injection', () => {
  for (const t of ['mouseDown', 'touchStart', 'pointerDown']) {
    assert.equal(isPressInput(t), true, t);
  }
  // touchScroll injects mouseWheel through sendInputEvent, which raises
  // input-event just like a real press would. A wheel is not intent to focus,
  // and counting it would let a scrolling page keep stolen focus.
  for (const t of ['mouseWheel', 'mouseMove', 'mouseUp', 'keyDown', 'char', 'touchEnd', 'undefined']) {
    assert.equal(isPressInput(t), false, t);
  }
});

test('FOCUS_SETTLE_MS pins the production settle', () => {
  // focusguard.ts says what the constant is worth.
  assert.equal(FOCUS_SETTLE_MS, 120);
});
