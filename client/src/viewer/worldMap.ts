// The world map window: the part of a procedural world's map graph that
// the character has found (proposal 0002, review decision 43). The server
// sends a WorldMap after login and each time the character enters a map
// for the first time. Each one replaces the one before. A classic world
// sends none.
//
// The layout math is pure, so tests can check it:
//
// - Each field sits at the center of its grid cell. North is up.
// - Towns and the home map sit in a row at the top of their seat field's
//   cell.
// - A dungeon's floors stack in a column at the bottom left of its
//   entrance field's cell, floor 1 at the top.
// - Visited maps show their name and level band. Other maps show "?".
//
// User text is only ever set with textContent.

/** One map of the world map, as plain data. */
export interface WorldMapNodeView {
  id: string;
  /** field, town, dungeon or home. */
  kind: string;
  /** The display name, or empty for a map the character has not visited. */
  name: string;
  /** The grid cell of a field, or of the field a map sits near. North is +y. */
  x: number;
  y: number;
  /** A dungeon floor's number, from 1, or 0. */
  floor: number;
  visited: boolean;
  minLevel: number;
  maxLevel: number;
}

/** A warp between two maps. */
export interface WorldMapLinkView {
  a: string;
  b: string;
}

/** The world map, as plain data. */
export interface WorldMapData {
  maps: WorldMapNodeView[];
  links: WorldMapLinkView[];
}

/** The WorldMap event's fields that the client reads. */
export interface WorldMapInput {
  maps: readonly {
    id: string;
    kind: string;
    name: string;
    x: number;
    y: number;
    floor: number;
    visited: boolean;
    minLevel: number;
    maxLevel: number;
  }[];
  links: readonly { a: string; b: string }[];
}

/** Copies a WorldMap event into plain data. */
export function worldMapOf(m: WorldMapInput): WorldMapData {
  return {
    maps: m.maps.map((n) => ({
      id: n.id,
      kind: n.kind,
      name: n.name,
      x: n.x,
      y: n.y,
      floor: n.floor,
      visited: n.visited,
      minLevel: n.minLevel,
      maxLevel: n.maxLevel,
    })),
    links: m.links.map((l) => ({ a: l.a, b: l.b })),
  };
}

/** The size of a grid cell in the drawing's units. */
export const CELL = 160;
/** The size of label text in the drawing's units. */
export const FONT = 9;
/** The space around the drawing. */
const PADDING = 12;
/** How far towns, the home map and dungeons sit from the cell's center. */
const SIDE = 48;
/** Where a dungeon's floor 1 sits below the cell's center. */
const FLOOR_TOP = 34;
/** The room for a dungeon's floors below floor 1. */
const FLOOR_ROOM = 40;
/** The most space between two floors. */
const FLOOR_STEP = 14;

/** A level band as text, such as "1-10", or empty when there is none. */
export function bandText(minLevel: number, maxLevel: number): string {
  if (maxLevel <= 0) {
    return '';
  }
  return minLevel === maxLevel ? String(minLevel) : `${minLevel}-${maxLevel}`;
}

/**
 * A map's label, such as "Valheim 1-10". A map the character has not
 * visited has none.
 */
export function nodeLabel(n: WorldMapNodeView): string {
  if (!n.visited) {
    return '';
  }
  return [n.name === '' ? n.id : n.name, bandText(n.minLevel, n.maxLevel)]
    .filter((s) => s !== '')
    .join(' ');
}

/**
 * The name to show for the current map: the display name of its map
 * JSON, then the world map's name, then the map's ID.
 */
export function mapDisplayName(
  id: string,
  infoName: string | undefined,
  data: WorldMapData | undefined,
): string {
  if (infoName !== undefined && infoName !== '') {
    return infoName;
  }
  const node = data?.maps.find((m) => m.id === id);
  return node !== undefined && node.name !== '' ? node.name : id;
}

/** A map placed in the drawing. */
export interface PlacedNode {
  id: string;
  kind: string;
  /** The center, in the drawing's units. y grows down, as in SVG. */
  cx: number;
  cy: number;
  r: number;
  visited: boolean;
  current: boolean;
  /** The full label, such as "Crypt of Skarvik F2 21-25", or empty. */
  label: string;
  /** The text drawn beside the map: the label, a short one, or "?". */
  text: string;
  /** Where the text goes, and how it is anchored. */
  tx: number;
  ty: number;
  anchor: 'start' | 'middle';
}

/** A warp drawn as a line. */
export interface PlacedLink {
  a: string;
  b: string;
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** The drawing's box, for the SVG's viewBox. */
export interface Bounds {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** The world map laid out for drawing. */
export interface WorldMapLayout {
  nodes: PlacedNode[];
  links: PlacedLink[];
  bounds: Bounds;
}

/** A dungeon's ID from a floor's map ID: gen:dungeon:crypt:2 gives gen:dungeon:crypt. */
function dungeonOf(id: string): string {
  const colon = id.lastIndexOf(':');
  return colon < 0 ? id : id.slice(0, colon);
}

/** Spreads n slots over [-SIDE, SIDE]. One slot sits at single. */
function spread(i: number, n: number, single: number): number {
  return n <= 1 ? single : -SIDE + (2 * SIDE * i) / (n - 1);
}

/** The width of a text in the drawing's units, about. */
function textWidth(text: string): number {
  return text.length * FONT * 0.6;
}

/** A map's circle size: fields are largest, and unvisited maps small. */
function radius(kind: string, visited: boolean): number {
  if (kind === 'dungeon') {
    return visited ? 5 : 4;
  }
  if (!visited) {
    return 6;
  }
  return kind === 'field' ? 10 : 8;
}

/** Lays out the world map. current is the current map's ID. */
export function layoutWorldMap(data: WorldMapData, current = ''): WorldMapLayout {
  // The maps of each cell, by kind.
  const cells = new Map<string, WorldMapNodeView[]>();
  for (const m of data.maps) {
    const key = `${m.x},${m.y}`;
    const list = cells.get(key) ?? [];
    list.push(m);
    cells.set(key, list);
  }
  const offsets = new Map<string, [number, number]>();
  for (const list of cells.values()) {
    // Towns, then the home map, in a row at the top.
    const top = list
      .filter((m) => m.kind === 'town' || m.kind === 'home')
      .toSorted(
        (p, q) => Number(p.kind === 'home') - Number(q.kind === 'home') || p.id.localeCompare(q.id),
      );
    top.forEach((m, i) => {
      offsets.set(m.id, [spread(i, top.length, m.kind === 'home' ? SIDE : -SIDE), -SIDE]);
    });
    // Each dungeon is a column of its floors.
    const dungeons = new Map<string, WorldMapNodeView[]>();
    for (const m of list) {
      if (m.kind === 'dungeon') {
        const d = dungeonOf(m.id);
        dungeons.set(d, [...(dungeons.get(d) ?? []), m]);
      }
    }
    const columns = [...dungeons.keys()].toSorted();
    columns.forEach((d, j) => {
      const floors = dungeons.get(d) ?? [];
      const deepest = Math.max(...floors.map((f) => f.floor), 1);
      const step = Math.min(FLOOR_STEP, FLOOR_ROOM / Math.max(deepest - 1, 1));
      for (const f of floors) {
        const x = spread(j, columns.length, -SIDE);
        offsets.set(f.id, [x, FLOOR_TOP + (Math.max(f.floor, 1) - 1) * step]);
      }
    });
  }
  const nodes = data.maps.map((m): PlacedNode => {
    const [dx, dy] = offsets.get(m.id) ?? [0, 0];
    const cx = m.x * CELL + dx;
    const cy = -m.y * CELL + dy;
    const dungeon = m.kind === 'dungeon';
    const r = radius(m.kind, m.visited);
    const label = nodeLabel(m);
    let text = label;
    let tx = cx;
    let ty = cy + r + FONT + 1;
    let anchor: PlacedNode['anchor'] = 'middle';
    if (!m.visited) {
      // A "?" inside the circle.
      text = '?';
      ty = cy + FONT * 0.35;
    } else if (dungeon) {
      // Floors stack, so their labels go to the right, and only floor 1
      // repeats the dungeon's name.
      if (m.floor > 1) {
        text = [`F${m.floor}`, bandText(m.minLevel, m.maxLevel)].filter((s) => s !== '').join(' ');
      }
      tx = cx + r + 3;
      ty = cy + FONT * 0.35;
      anchor = 'start';
    }
    return {
      id: m.id,
      kind: m.kind,
      cx,
      cy,
      r,
      visited: m.visited,
      current: m.id === current,
      label,
      text,
      tx,
      ty,
      anchor,
    };
  });
  const at = new Map(nodes.map((n) => [n.id, n]));
  const links: PlacedLink[] = [];
  for (const l of data.links) {
    const p = at.get(l.a);
    const q = at.get(l.b);
    if (p !== undefined && q !== undefined) {
      links.push({ a: l.a, b: l.b, x1: p.cx, y1: p.cy, x2: q.cx, y2: q.cy });
    }
  }
  return { nodes, links, bounds: boundsOf(nodes) };
}

/** The box around every map and its text. */
function boundsOf(nodes: readonly PlacedNode[]): Bounds {
  if (nodes.length === 0) {
    return { x: 0, y: 0, width: 1, height: 1 };
  }
  let left = Infinity;
  let top = Infinity;
  let right = -Infinity;
  let bottom = -Infinity;
  for (const n of nodes) {
    left = Math.min(left, n.cx - n.r);
    right = Math.max(right, n.cx + n.r);
    top = Math.min(top, n.cy - n.r);
    bottom = Math.max(bottom, n.cy + n.r);
    if (n.visited && n.text !== '') {
      const w = textWidth(n.text);
      const x0 = n.anchor === 'start' ? n.tx : n.tx - w / 2;
      left = Math.min(left, x0);
      right = Math.max(right, x0 + w);
      top = Math.min(top, n.ty - FONT);
      bottom = Math.max(bottom, n.ty + FONT * 0.3);
    }
  }
  return {
    x: left - PADDING,
    y: top - PADDING,
    width: right - left + 2 * PADDING,
    height: bottom - top + 2 * PADDING,
  };
}

/** A count of the maps found and visited, for the window's summary. */
export function worldMapSummary(data: WorldMapData): string {
  const visited = data.maps.filter((m) => m.visited).length;
  return `Visited ${visited} of the ${data.maps.length} maps found.`;
}

const SVG = 'http://www.w3.org/2000/svg';

/** The world map window. It draws again when it opens and on each change. */
export class WorldMapWindow {
  private data: WorldMapData | undefined;
  private current = '';
  private readonly body = document.createElement('div');

  /**
   * procedural says whether the world has a world map. A classic world
   * never gets one.
   */
  constructor(
    private readonly root: HTMLElement,
    private readonly procedural: boolean,
  ) {
    this.body.className = 'world-map-body';
    root.append(this.body);
    this.render();
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Shows a new world map. It replaces the one before. */
  set(data: WorldMapData): void {
    this.data = data;
    this.render();
  }

  /** Marks the current map. */
  setCurrent(id: string): void {
    this.current = id;
    this.render();
  }

  /** Opens or closes the window. */
  toggle(): void {
    this.root.hidden = !this.root.hidden;
    this.render();
  }

  /** Draws the window, while it is open. */
  render(): void {
    if (!this.open) {
      return;
    }
    const data = this.data;
    if (data === undefined) {
      const p = document.createElement('p');
      p.className = 'world-map-empty';
      p.textContent = this.procedural
        ? 'The world map shows once the server sends it.'
        : 'This world has no world map.';
      this.body.replaceChildren(p);
      return;
    }
    const layout = layoutWorldMap(data, this.current);
    const b = layout.bounds;
    const svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('viewBox', `${b.x} ${b.y} ${b.width} ${b.height}`);
    svg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
    svg.setAttribute('role', 'img');
    svg.setAttribute('aria-label', 'World map');
    svg.classList.add('world-map');
    const lines = document.createElementNS(SVG, 'g');
    lines.setAttribute('class', 'world-map-links');
    for (const l of layout.links) {
      const line = document.createElementNS(SVG, 'line');
      line.setAttribute('x1', String(l.x1));
      line.setAttribute('y1', String(l.y1));
      line.setAttribute('x2', String(l.x2));
      line.setAttribute('y2', String(l.y2));
      lines.append(line);
    }
    const dots = document.createElementNS(SVG, 'g');
    for (const n of layout.nodes) {
      const g = document.createElementNS(SVG, 'g');
      const classes = ['world-map-node', `kind-${n.kind}`, n.visited ? 'visited' : 'unvisited'];
      if (n.current) {
        classes.push('current');
      }
      g.setAttribute('class', classes.join(' '));
      g.dataset.map = n.id;
      if (n.label !== '') {
        const title = document.createElementNS(SVG, 'title');
        title.textContent = n.label;
        g.append(title);
      }
      const c = document.createElementNS(SVG, 'circle');
      c.setAttribute('cx', String(n.cx));
      c.setAttribute('cy', String(n.cy));
      c.setAttribute('r', String(n.r));
      g.append(c);
      if (n.text !== '') {
        const t = document.createElementNS(SVG, 'text');
        t.setAttribute('x', String(n.tx));
        t.setAttribute('y', String(n.ty));
        t.setAttribute('text-anchor', n.anchor);
        t.setAttribute('font-size', String(n.visited ? FONT : FONT - 1));
        t.textContent = n.text;
        g.append(t);
      }
      dots.append(g);
    }
    svg.append(lines, dots);
    const summary = document.createElement('p');
    summary.className = 'world-map-summary';
    summary.textContent = worldMapSummary(data);
    this.body.replaceChildren(svg, summary);
  }
}
