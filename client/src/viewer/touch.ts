// Touch on the page around the game view (review decision 39):
//
// - InputMode follows the pointer last used. The page's body has the
//   class `touch` while touch is used, which shows the on-screen
//   controls. A mouse or pen switches it back.
// - A long press on a UI element does what a right click does. On an
//   item, skill or other draggable row, a long press starts a drag
//   instead: the row follows the finger, and lifting it over a window
//   drops it there, through the same dragstart, dragover and drop
//   handlers that a mouse drag uses. A long press lifted without moving
//   does the right click.
// - Two taps on one element do what a double click does.
// - Times come from the events' timeStamp, when the finger moved, not
//   from when the page handles them. A busy page may handle the two
//   events of a quick tap a long frame apart.
//
// The browser's own long press menu, drag and double tap are turned off
// for touch, so each action happens once. Text fields keep theirs. The
// game view has its own gestures (gestures.ts), so this leaves it alone.

import { LONG_PRESS_MS, SLOP } from './gestures';

export type InputKind = 'mouse' | 'touch';

/** The kind of pointer last used, and the page's `touch` class. */
export class InputMode {
  private current: InputKind;
  private readonly listeners: ((kind: InputKind) => void)[] = [];

  constructor(private readonly body: HTMLElement) {
    this.current = matchMedia('(hover: none) and (pointer: coarse)').matches ? 'touch' : 'mouse';
    this.apply();
    addEventListener(
      'pointerdown',
      (e) => {
        this.set(e.pointerType === 'touch' ? 'touch' : 'mouse');
      },
      { capture: true },
    );
  }

  get kind(): InputKind {
    return this.current;
  }

  get touch(): boolean {
    return this.current === 'touch';
  }

  /** Calls f with each change. */
  onChange(f: (kind: InputKind) => void): void {
    this.listeners.push(f);
  }

  set(kind: InputKind): void {
    if (kind === this.current) {
      return;
    }
    this.current = kind;
    this.apply();
    for (const f of this.listeners) {
      f(kind);
    }
  }

  private apply(): void {
    this.body.classList.toggle('touch', this.current === 'touch');
  }
}

/** Two taps within this time make a double tap, in milliseconds. */
const DOUBLE_TAP_MS = 450;
/** And within this distance, in CSS pixels. */
const DOUBLE_TAP_SLOP = 30;

/** Whether an element takes typed text, so it keeps the browser's own touch menu. */
function isTextField(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) {
    return false;
  }
  if (t.isContentEditable || t instanceof HTMLTextAreaElement || t instanceof HTMLSelectElement) {
    return true;
  }
  if (t instanceof HTMLInputElement) {
    return !['button', 'checkbox', 'radio', 'submit', 'reset', 'range', 'color'].includes(t.type);
  }
  return false;
}

/** A drag event from a finger, with the given data. */
function dragEvent(type: string, data: DataTransfer, x: number, y: number): Event {
  const e = new Event(type, { bubbles: true, cancelable: true, composed: true });
  Object.defineProperties(e, {
    dataTransfer: { value: data },
    clientX: { value: x },
    clientY: { value: y },
  });
  return e;
}

/** A right click, double click or click at a point, as a mouse would make it. */
function mouseEvent(type: string, x: number, y: number): MouseEvent {
  return new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    composed: true,
    clientX: x,
    clientY: y,
    button: type === 'contextmenu' ? 2 : 0,
    buttons: 0,
    detail: type === 'dblclick' ? 2 : 1,
  });
}

/**
 * A DataTransfer for a finger drag. Browsers since 2021 make one with
 * new DataTransfer(). The fallback holds strings only, which is all the
 * game's drags carry.
 */
function newDataTransfer(): DataTransfer {
  try {
    return new DataTransfer();
  } catch {
    const data = new Map<string, string>();
    const shim = {
      dropEffect: 'none',
      effectAllowed: 'all',
      files: [],
      items: [],
      get types() {
        return [...data.keys()];
      },
      getData: (t: string) => data.get(t) ?? '',
      setData: (t: string, v: string) => {
        data.set(t, v);
      },
      clearData: (t?: string) => {
        if (t === undefined) {
          data.clear();
        } else {
          data.delete(t);
        }
      },
      setDragImage: () => undefined,
    };
    // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a stand-in with the methods the game's drags use.
    return shim as unknown as DataTransfer;
  }
}

interface Press {
  id: number;
  /** When the finger went down, as the event's timeStamp. */
  at: number;
  target: Element;
  /** The draggable element under the finger, if any. */
  source: HTMLElement | undefined;
  x: number;
  y: number;
  timer: ReturnType<typeof setTimeout>;
  /** Set once the long press time passed. */
  fired: boolean;
  drag:
    | {
        data: DataTransfer;
        ghost: HTMLElement;
        offsetX: number;
        offsetY: number;
        moved: boolean;
        over: Element | undefined;
      }
    | undefined;
}

/**
 * Long presses, finger drags and double taps on the page. view is the
 * game view, which this leaves alone.
 */
export function installTouchUI(mode: InputMode, view: HTMLElement): void {
  let press: Press | undefined;
  /** Clicks until this time are the end of a long press, and do nothing. */
  let quietUntil = 0;
  let lastTap: { target: Element; x: number; y: number; at: number } | undefined;

  const end = (now: number): void => {
    if (press === undefined) {
      return;
    }
    clearTimeout(press.timer);
    press.drag?.ghost.remove();
    if (press.fired) {
      quietUntil = now + 400;
    }
    press = undefined;
  };

  const startDrag = (p: Press, source: HTMLElement): void => {
    const data = newDataTransfer();
    if (!source.dispatchEvent(dragEvent('dragstart', data, p.x, p.y))) {
      // The row refused the drag.
      return;
    }
    const rect = source.getBoundingClientRect();
    // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- cloneNode of an HTMLElement is one.
    const ghost = source.cloneNode(true) as HTMLElement;
    ghost.removeAttribute('id');
    ghost.classList.add('touch-ghost');
    ghost.style.width = `${rect.width}px`;
    ghost.style.left = `${rect.left}px`;
    ghost.style.top = `${rect.top}px`;
    document.body.append(ghost);
    p.drag = {
      data,
      ghost,
      offsetX: p.x - rect.left,
      offsetY: p.y - rect.top,
      moved: false,
      over: undefined,
    };
    source.classList.add('touch-dragging');
  };

  /** Ends the long press time: a right click, or a drag of a draggable row. */
  const fire = (p: Press): void => {
    if (p.fired) {
      return;
    }
    p.fired = true;
    if (p.source === undefined) {
      p.target.dispatchEvent(mouseEvent('contextmenu', p.x, p.y));
    } else {
      startDrag(p, p.source);
    }
  };

  /**
   * Fires a press that is due by the event's time, before the event
   * counts. A busy page may handle a finger's events after the long press
   * time without its timer having run.
   */
  const due = (p: Press, e: PointerEvent): void => {
    if (!p.fired && e.timeStamp - p.at >= LONG_PRESS_MS) {
      fire(p);
    }
  };

  /** Sends dragover to what is under a point. Returns the element, if it takes the drop. */
  const over = (p: Press, x: number, y: number): Element | undefined => {
    const d = p.drag;
    if (d === undefined) {
      return undefined;
    }
    const under = document.elementFromPoint(x, y);
    if (under === null) {
      d.ghost.classList.remove('can-drop');
      return undefined;
    }
    const takes = !under.dispatchEvent(dragEvent('dragover', d.data, x, y));
    d.ghost.classList.toggle('can-drop', takes);
    d.over = takes ? under : undefined;
    return d.over;
  };

  document.addEventListener(
    'pointerdown',
    (e) => {
      if (e.pointerType !== 'touch') {
        return;
      }
      if (press !== undefined) {
        // A second finger ends a press or drag.
        if (press.drag !== undefined) {
          press.source?.dispatchEvent(dragEvent('dragend', press.drag.data, e.clientX, e.clientY));
          press.source?.classList.remove('touch-dragging');
        }
        end(e.timeStamp);
        return;
      }
      const target = e.target;
      if (!(target instanceof Element) || view.contains(target) || isTextField(target)) {
        return;
      }
      const source = target.closest<HTMLElement>('[draggable="true"]') ?? undefined;
      const p: Press = {
        id: e.pointerId,
        at: e.timeStamp,
        target,
        source,
        x: e.clientX,
        y: e.clientY,
        fired: false,
        drag: undefined,
        timer: setTimeout(() => {
          if (press === p) {
            fire(p);
          }
        }, LONG_PRESS_MS),
      };
      press = p;
    },
    { capture: true },
  );

  document.addEventListener(
    'pointermove',
    (e) => {
      const p = press;
      if (p === undefined || e.pointerId !== p.id) {
        return;
      }
      due(p, e);
      const d = p.drag;
      if (d === undefined) {
        if (!p.fired && Math.hypot(e.clientX - p.x, e.clientY - p.y) > SLOP) {
          // A swipe or a scroll, not a press.
          end(e.timeStamp);
        }
        return;
      }
      e.preventDefault();
      d.ghost.style.left = `${e.clientX - d.offsetX}px`;
      d.ghost.style.top = `${e.clientY - d.offsetY}px`;
      if (Math.hypot(e.clientX - p.x, e.clientY - p.y) > SLOP) {
        d.moved = true;
      }
      over(p, e.clientX, e.clientY);
    },
    { capture: true },
  );

  // While a finger drags, the page must not scroll. Only touchmove can
  // stop that, and only from a listener that is not passive.
  document.addEventListener(
    'touchmove',
    (e) => {
      if (press?.drag !== undefined) {
        e.preventDefault();
      }
    },
    { passive: false },
  );

  document.addEventListener(
    'pointerup',
    (e) => {
      const p = press;
      if (p === undefined || e.pointerId !== p.id) {
        return;
      }
      due(p, e);
      const d = p.drag;
      if (d !== undefined && p.source !== undefined) {
        if (d.moved) {
          const target = over(p, e.clientX, e.clientY);
          target?.dispatchEvent(dragEvent('drop', d.data, e.clientX, e.clientY));
        } else {
          // Lifted where it was pressed: the right click.
          p.source.dispatchEvent(mouseEvent('contextmenu', p.x, p.y));
        }
        p.source.dispatchEvent(dragEvent('dragend', d.data, e.clientX, e.clientY));
        p.source.classList.remove('touch-dragging');
      }
      end(e.timeStamp);
    },
    { capture: true },
  );

  document.addEventListener(
    'pointercancel',
    (e) => {
      const p = press;
      if (p === undefined || e.pointerId !== p.id) {
        return;
      }
      if (p.drag !== undefined) {
        p.source?.dispatchEvent(dragEvent('dragend', p.drag.data, e.clientX, e.clientY));
        p.source?.classList.remove('touch-dragging');
      }
      end(e.timeStamp);
    },
    { capture: true },
  );

  // The browser's long press menu and double tap would act a second time.
  addEventListener(
    'contextmenu',
    (e) => {
      if (e.isTrusted && mode.touch && !isTextField(e.target)) {
        e.preventDefault();
        e.stopImmediatePropagation();
      }
    },
    { capture: true },
  );
  addEventListener(
    'dblclick',
    (e) => {
      if (e.isTrusted && mode.touch && !isTextField(e.target)) {
        e.stopImmediatePropagation();
      }
    },
    { capture: true },
  );
  // A touch drag would start the browser's own drag on some phones.
  addEventListener(
    'dragstart',
    (e) => {
      if (e.isTrusted && mode.touch) {
        e.preventDefault();
        e.stopImmediatePropagation();
      }
    },
    { capture: true },
  );

  addEventListener(
    'click',
    (e) => {
      if (!e.isTrusted || !mode.touch) {
        return;
      }
      // The time of the tap, not of this handler: a busy page may handle
      // two quick taps far apart.
      const now = e.timeStamp;
      if (now < quietUntil) {
        e.preventDefault();
        e.stopImmediatePropagation();
        return;
      }
      const target = e.target;
      if (!(target instanceof Element) || view.contains(target) || isTextField(target)) {
        lastTap = undefined;
        return;
      }
      // Taps on two parts of one row or button count as one place. A row
      // that the first tap drew again counts as the same row.
      const key = target.closest('li, tr, button, [draggable="true"]') ?? target;
      const t = lastTap;
      if (
        t !== undefined &&
        (t.target === key || !t.target.isConnected) &&
        now - t.at <= DOUBLE_TAP_MS &&
        Math.hypot(e.clientX - t.x, e.clientY - t.y) <= DOUBLE_TAP_SLOP
      ) {
        lastTap = undefined;
        const x = e.clientX;
        const y = e.clientY;
        // After this click's own handlers, as a browser sends it.
        setTimeout(() => {
          target.dispatchEvent(mouseEvent('dblclick', x, y));
        }, 0);
        return;
      }
      lastTap = { target: key, x: e.clientX, y: e.clientY, at: now };
    },
    { capture: true },
  );
}
