import { weapons } from '../data/tables';
import type { Look } from '../render/characters';

// Random character looks for the viewer, from names the asset contract
// defines. Missing names draw nothing, so guesses are safe.

/** Looks with a typical weapon type (rAthena W_ ID) and whether they carry a shield. */
const JOBS: { look: string; weapon: number; shield?: boolean; sex?: 'm' | 'f' }[] = [
  { look: 'novice', weapon: 1 },
  { look: 'swordman', weapon: 2, shield: true },
  { look: 'knight', weapon: 3 },
  { look: 'crusader', weapon: 2, shield: true },
  { look: 'archer', weapon: 11 },
  { look: 'hunter', weapon: 11 },
  { look: 'acolyte', weapon: 8 },
  { look: 'priest', weapon: 8, shield: true },
  { look: 'magician', weapon: 10 },
  { look: 'wizard', weapon: 10 },
  { look: 'merchant', weapon: 6 },
  { look: 'blacksmith', weapon: 6 },
  { look: 'thief', weapon: 1 },
  { look: 'assassin', weapon: 16 },
];

export default function randomLook(random: (n: number) => number): Look {
  const job = JOBS[random(JOBS.length)]!;
  const sex = job.sex ?? (random(2) === 0 ? 'm' : 'f');
  const style = 1 + random(20);
  const look: Look = {
    body: `sprite/body/${job.look}/${sex}`,
    head: `sprite/head/human/${sex}/${style}`,
    hairPalette: `palette/hair/human/${sex}/${style}/${random(9)}`,
    weapon: `sprite/weapon/${job.look}/${sex}/${weapons.get(job.weapon)?.view ?? 0}`,
    headgearTop: `sprite/headgear/${sex}/${1 + random(60)}`,
  };
  if (random(3) === 0) {
    look.bodyPalette = `palette/body/${job.look}/${sex}/${1 + random(3)}`;
  }
  if (job.shield === true) {
    look.shield = `sprite/shield/${job.look}/${sex}/${1 + random(3)}`;
  }
  if (random(4) === 0) {
    look.garment = `sprite/garment/${job.look}/${sex}/${1 + random(20)}`;
  }
  return look;
}
