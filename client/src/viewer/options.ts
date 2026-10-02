import type { Options } from '../gen/wathena/v1/event_pb';

// How a unit's status options look: a color that multiplies its sprites,
// an opacity, and whether it shows at all. The server sends rAthena's
// opt1 (body), opt2 (health) and option bits (event.proto, Options).

/** A unit's look from its options. */
export interface Appearance {
  /** Multiplies red, green and blue, from 0 to 1. */
  color: [number, number, number];
  /** Multiplies opacity, from 0 to 1. */
  opacity: number;
  /** False when the unit is not drawn at all. */
  visible: boolean;
}

/** How a unit looks with no options. */
export const PLAIN: Appearance = { color: [1, 1, 1], opacity: 1, visible: true };

/** OPT1_ values of the body. */
const BODY_STONE = 1;
const BODY_FREEZE = 2;
const BODY_STUN = 3;
const BODY_SLEEP = 4;
const BODY_STONE_WAIT = 6;
const BODY_IMPRISON = 8;

/** OPT2_ bits of health. */
const HEALTH_POISON = 0x1;
const HEALTH_BLIND = 0x10;
const HEALTH_DEADLY_POISON = 0x80;

/**
 * OPTION_ bits that hide a unit: hiding (0x2), cloaking (0x4), the
 * invisible option of GMs (0x40), and chase walk (0x4000).
 */
const HIDDEN_OPTIONS = 0x2 | 0x4 | 0x40 | 0x4000;

/** The colors of body states. */
const BODY_COLORS = new Map<number, [number, number, number]>([
  // Stone is grey.
  [BODY_STONE, [0.6, 0.6, 0.6]],
  [BODY_STONE_WAIT, [0.6, 0.6, 0.6]],
  // Frozen is light blue.
  [BODY_FREEZE, [0.55, 0.75, 1]],
  // Stun, sleep and imprison are a little darker.
  [BODY_STUN, [0.85, 0.85, 0.85]],
  [BODY_SLEEP, [0.85, 0.85, 0.85]],
  [BODY_IMPRISON, [0.85, 0.85, 0.85]],
]);

/** The color of poison and deadly poison: green, with red a little higher than blue. */
const POISON_COLOR: [number, number, number] = [0.8, 0.95, 0.7];

/** How much blindness darkens a unit. */
const BLIND = 0.6;

/** How opaque the player's own hidden character is. */
const HIDDEN_OPACITY = 0.5;

/**
 * How a unit looks with its options. mine is true for the player's own
 * character, which stays half visible while hidden. Other hidden units
 * do not show.
 */
export function appearance(o: Options | undefined, mine: boolean): Appearance {
  if (o === undefined) {
    return PLAIN;
  }
  let color = BODY_COLORS.get(o.body) ?? PLAIN.color;
  const tint = (c: readonly [number, number, number]): void => {
    color = [color[0] * c[0], color[1] * c[1], color[2] * c[2]];
  };
  if ((o.health & (HEALTH_POISON | HEALTH_DEADLY_POISON)) !== 0) {
    tint(POISON_COLOR);
  }
  if ((o.health & HEALTH_BLIND) !== 0) {
    tint([BLIND, BLIND, BLIND]);
  }
  const hidden = (o.option & HIDDEN_OPTIONS) !== 0;
  return {
    color: [...color],
    opacity: hidden && mine ? HIDDEN_OPACITY : 1,
    visible: !hidden || mine,
  };
}
