import { DigitsKind } from '../gen/wathena/v1/event_pb';

// The digital clock at the top of the screen, as rAthena's showdigit
// (clif_showdigit): a value shown for 5 seconds, or a counter that moves
// once a second.

/** How long a value of DigitsKind.UNSPECIFIED shows, in milliseconds. */
const SHOW_MS = 5000;

/**
 * Seconds as DD:HH:MM:SS, HH:MM:SS, MM:SS or SS: the parts on the left
 * that are 0 are left out, as the client shows them.
 */
export function clockText(seconds: number): string {
  const s = Math.max(Math.trunc(seconds), 0);
  const parts = [Math.floor(s / 86400), Math.floor(s / 3600) % 24, Math.floor(s / 60) % 60, s % 60];
  while (parts.length > 1 && parts[0] === 0) {
    parts.shift();
  }
  return parts.map((p) => String(p).padStart(2, '0')).join(':');
}

/**
 * What the clock shows elapsedMs after a DigitsShow came, or undefined
 * once it is gone:
 * - UNSPECIFIED shows the value for 5 seconds.
 * - COUNT_UP counts up from -value.
 * - COUNT_DOWN counts down from -value, and goes at 0.
 * - COUNT_DOWN_TO_ZERO counts down from value with two digits, stops at
 *   0, and goes a second later.
 */
export function digitsText(kind: DigitsKind, value: number, elapsedMs: number): string | undefined {
  const ticks = Math.floor(Math.max(elapsedMs, 0) / 1000);
  switch (kind) {
    case DigitsKind.COUNT_UP:
      return clockText(-value + ticks);
    case DigitsKind.COUNT_DOWN: {
      const left = -value - ticks;
      return left < 0 ? undefined : clockText(left);
    }
    case DigitsKind.COUNT_DOWN_TO_ZERO: {
      const left = value - ticks;
      return left < -1 ? undefined : String(Math.max(left, 0)).padStart(2, '0');
    }
    default:
      return elapsedMs >= SHOW_MS ? undefined : clockText(value);
  }
}
