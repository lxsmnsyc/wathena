// Chooses what a click on the screen points at, among units and ground
// items.

/** Something a click can point at, at a point on the screen. */
export interface Target {
  id: number;
  item: boolean;
  /** Screen position in pixels. */
  x: number;
  y: number;
  /** How far from the point a click still counts, in pixels. */
  radius: number;
}

/**
 * The target nearest a click, relative to each target's radius. A small
 * item at a unit's feet wins over the unit when the click is on the item,
 * and the unit wins when the click is on its body.
 */
export default function nearest(
  targets: Iterable<Target>,
  x: number,
  y: number,
): Target | undefined {
  let best: Target | undefined;
  let bestScore = 1;
  for (const t of targets) {
    const score = Math.hypot(t.x - x, t.y - y) / t.radius;
    if (score < bestScore) {
      best = t;
      bestScore = score;
    }
  }
  return best;
}
