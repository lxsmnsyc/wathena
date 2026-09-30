import type { MapSound } from '../formats/wmap';
import type { SoundPlayer } from './audio';

// Plays a map's placed sounds, as RO does: each repeats every cycle
// seconds, heard only within its range, quieter with distance.

/** A point and the listener's right-hand direction, in map space. */
export interface Listener {
  position: { x: number; y: number; z: number };
  right: { x: number; y: number; z: number };
}

export default class Soundscape {
  private readonly timers: number[];

  constructor(
    private readonly sounds: readonly MapSound[],
    private readonly player: SoundPlayer,
    random: () => number = Math.random,
  ) {
    // Sounds start at random points in their cycles, so a map does not
    // play them all at once.
    this.timers = sounds.map((s) => random() * Math.max(s.cycle, 0.1));
  }

  update(seconds: number, listener: Listener): void {
    this.sounds.forEach((s, i) => {
      let t = this.timers[i]! - seconds;
      if (t > 0) {
        this.timers[i] = t;
        return;
      }
      t += Math.max(s.cycle, 0.1);
      this.timers[i] = Math.max(t, 0);
      const dx = s.position[0] - listener.position.x;
      const dy = s.position[1] - listener.position.y;
      const dz = s.position[2] - listener.position.z;
      const distance = Math.hypot(dx, dy, dz);
      if (s.range <= 0 || distance >= s.range) {
        return;
      }
      const side = dx * listener.right.x + dy * listener.right.y + dz * listener.right.z;
      this.player.play(s.name, s.volume * (1 - distance / s.range), side / s.range);
    });
  }
}
