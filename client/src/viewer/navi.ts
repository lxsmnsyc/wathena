// Client navigation (proposal 0006, section 5; decision 0079). An NPC
// dialog holds <NAVI>[name]<INFO>map,x,y,services,monster,hide</INFO></NAVI>
// links. A click sends a NaviCommand, and the server answers with a
// NaviRoute: the legs of a route over the warps, each as the cell to leave
// a map at. The window lists the maps of the route and points at the next
// cell. A route that the server cannot find shows the target alone. Names
// and maps are only ever set with textContent.

import { button, element } from './forms';

/** Where a navigation points: the arguments of rAthena's navigateto. */
export interface NaviTarget {
  map: string;
  x: number;
  y: number;
  monster: number;
  services: number;
  hideWindow: boolean;
}

/** A link of a dialog: its text and its target. */
export interface NaviLink extends NaviTarget {
  name: string;
}

/** One leg of a route: leave map at the cell. */
export interface NaviStep {
  map: string;
  x: number;
  y: number;
}

/** A NaviRoute event. */
export interface NaviRoute extends NaviTarget {
  found: boolean;
  steps: NaviStep[];
  cancel: boolean;
}

/** The longest map name and the largest cell a link may name. */
const MAX_MAP = 64;
const MAX_CELL = 65535;

/** The fields of an <INFO>: map, x, y, services, monster and hide. */
const INFO =
  /^([A-Za-z0-9_@.-]{1,64}),(\d{1,5}),(\d{1,5})(?:,(\d{1,3})(?:,(\d{1,5})(?:,(\d))?)?)?$/;

/**
 * Reads the text between <INFO> and </INFO> of a <NAVI> link, or gives
 * undefined for text that is not a map, a cell and the optional services
 * (0 to 255), monster and hide fields. The hide field is 1 to keep the
 * window closed.
 */
export function parseNaviInfo(name: string, info: string): NaviLink | undefined {
  const m = INFO.exec(info.trim());
  if (m === null) {
    return undefined;
  }
  const x = Number(m[2]);
  const y = Number(m[3]);
  const services = m[4] === undefined ? 0 : Number(m[4]);
  if (x > MAX_CELL || y > MAX_CELL || services > 255 || (m[1] ?? '').length > MAX_MAP) {
    return undefined;
  }
  return {
    name,
    map: (m[1] ?? '').toLowerCase(),
    x,
    y,
    services,
    monster: m[5] === undefined ? 0 : Number(m[5]),
    hideWindow: m[6] === '1',
  };
}

/** The names of the maps of a route, from the map the player is on to the target's. */
export function naviMaps(route: NaviRoute): string[] {
  if (!route.found) {
    return [route.map];
  }
  const maps = route.steps.map((s) => s.map);
  if (maps.at(-1) !== route.map) {
    maps.push(route.map);
  }
  return maps;
}

/**
 * The cell the player heads for now: the first leg's warp, or the
 * target's cell when the player is on its map. It is undefined when the
 * route is not found, or the target has no cell.
 */
export function naviNext(route: NaviRoute): [number, number] | undefined {
  const first = route.steps[0];
  if (route.found && first !== undefined) {
    return [first.x, first.y];
  }
  if (route.found && route.x > 0 && route.y > 0) {
    return [route.x, route.y];
  }
  return undefined;
}

/**
 * The angle in degrees, clockwise from up, from one cell to another. Cell
 * y grows to the north, as the screen's up.
 */
export function naviAngle(from: [number, number], to: [number, number]): number {
  const dx = to[0] - from[0];
  const dy = to[1] - from[1];
  return (Math.atan2(dx, dy) * 180) / Math.PI;
}

/** The line that says what the route does. */
export function naviSummary(route: NaviRoute): string {
  const target =
    route.x > 0 && route.y > 0 ? `${route.map} ${String(route.x)},${String(route.y)}` : route.map;
  if (!route.found) {
    return `No route to ${target}.`;
  }
  if (route.steps.length === 0) {
    return `You are on ${route.map}.`;
  }
  return `To ${target}: ${String(route.steps.length)} ${route.steps.length === 1 ? 'map change' : 'map changes'}.`;
}

/** What the window sends. */
export interface NaviActions {
  cancel(): void;
}

/** How long a finished route stays on the screen, in milliseconds. */
const ARRIVED_MS = 5000;

/** The navigation window: the route, an arrow toward the next cell, and Cancel. */
export class NaviWindow {
  private readonly summary: HTMLElement;
  private readonly list: HTMLOListElement;
  private readonly arrow: SVGSVGElement;
  private route: NaviRoute | undefined;
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: NaviActions,
  ) {
    this.summary = element('p', 'navi-summary');
    this.list = element('ol', 'navi-maps');
    this.arrow = makeArrow();
    const head = element('div', 'navi-head');
    head.append(this.arrow, this.summary);
    root.append(
      head,
      this.list,
      button('Cancel', () => {
        this.cancel();
      }),
    );
    root.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        this.cancel();
      }
    });
    root.hidden = true;
  }

  /** Whether a route shows. */
  get open(): boolean {
    return !this.root.hidden;
  }

  /** Shows a route the server sent, or ends the navigation when it was cancelled. */
  set(route: NaviRoute): void {
    this.clearTimer();
    if (route.cancel) {
      this.hide();
      return;
    }
    this.route = route;
    this.summary.textContent = naviSummary(route);
    this.list.replaceChildren();
    // A route that asks for a hidden window shows the summary and arrow only.
    this.root.classList.toggle('compact', route.hideWindow);
    for (const map of naviMaps(route)) {
      const li = element('li');
      li.textContent = map;
      this.list.append(li);
    }
    this.root.hidden = false;
    this.arrow.style.visibility = 'hidden';
    if (route.found && route.steps.length === 0) {
      // The player is on the target's map: the route ends here.
      this.timer = setTimeout(() => {
        this.hide();
      }, ARRIVED_MS);
    }
  }

  /** Turns the arrow toward the next cell, as the player walks. */
  setCell(x: number, y: number): void {
    const r = this.route;
    if (r === undefined || this.root.hidden) {
      return;
    }
    const next = naviNext(r);
    if (next === undefined) {
      this.arrow.style.visibility = 'hidden';
      return;
    }
    this.arrow.style.visibility = 'visible';
    this.arrow.style.transform = `rotate(${String(naviAngle([x, y], next))}deg)`;
  }

  /** Closes the window and forgets the route. */
  hide(): void {
    this.clearTimer();
    this.route = undefined;
    this.root.hidden = true;
    this.list.replaceChildren();
  }

  private cancel(): void {
    if (this.root.hidden) {
      return;
    }
    this.hide();
    this.actions.cancel();
  }

  private clearTimer(): void {
    if (this.timer !== undefined) {
      clearTimeout(this.timer);
      this.timer = undefined;
    }
  }
}

/** An arrow that points up, which the window turns. */
function makeArrow(): SVGSVGElement {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('class', 'navi-arrow');
  svg.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS(ns, 'path');
  path.setAttribute('d', 'M12 2 L20 20 L12 15 L4 20 Z');
  path.setAttribute('fill', 'currentColor');
  svg.append(path);
  return svg;
}
