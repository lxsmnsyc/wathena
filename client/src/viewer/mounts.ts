import { jobs } from '../data/tables';

// What a player's option bits show besides its body: a companion, a cart,
// and a mount. The server sends rAthena's option bits (event.proto,
// Options). The bits are OPTION_ values in rAthena's status.hpp.

const OPTION_CART1 = 0x8;
const OPTION_FALCON = 0x10;
const OPTION_RIDING = 0x20;
const OPTION_CART2 = 0x80;
const OPTION_CART3 = 0x100;
const OPTION_CART4 = 0x200;
const OPTION_CART5 = 0x400;
const OPTION_WUG = 0x100000;
const OPTION_WUGRIDER = 0x200000;
const OPTION_MADOGEAR = 0x400000;
/** OPTION_DRAGON1 to OPTION_DRAGON5, one bit per dragon color. */
const OPTION_DRAGON = 0x80000 | 0x800000 | 0x1000000 | 0x2000000 | 0x4000000;

/** Cart bits in order of the cart look, 1 to 5. */
const CARTS = [OPTION_CART1, OPTION_CART2, OPTION_CART3, OPTION_CART4, OPTION_CART5];

/** Bits that put a player on a mount. */
const MOUNTS = OPTION_RIDING | OPTION_DRAGON | OPTION_WUGRIDER | OPTION_MADOGEAR;

/** Mounted third class jobs by the option bit and the job on foot. */
const MOUNTED: [number, ReadonlyMap<number, number>][] = [
  // Royal Guard, transcendent and baby, on a gryphon.
  [
    OPTION_RIDING,
    new Map([
      [4066, 4082],
      [4073, 4083],
      [4102, 4110],
    ]),
  ],
  // Rune Knight, transcendent and baby, on a dragon.
  [
    OPTION_DRAGON,
    new Map([
      [4054, 4080],
      [4060, 4081],
      [4096, 4109],
    ]),
  ],
  // Ranger, transcendent and baby, on a warg.
  [
    OPTION_WUGRIDER,
    new Map([
      [4056, 4084],
      [4062, 4085],
      [4098, 4111],
    ]),
  ],
  // Mechanic, transcendent and baby, in a Mado Gear.
  [
    OPTION_MADOGEAR,
    new Map([
      [4058, 4086],
      [4064, 4087],
      [4100, 4112],
    ]),
  ],
];

/**
 * Jobs whose body sprite shows a mount. The Peco Peco riders come from
 * the server, which swaps the job as rAthena's status_set_viewdata does.
 */
const MOUNTED_JOBS: ReadonlySet<number> = new Set([
  13,
  21,
  4014,
  4022,
  4036,
  4044,
  ...MOUNTED.flatMap(([, m]) => [...m.values()]),
]);

/** The companion drawn beside a player, if any. */
export function companionLook(option: number): 'falcon' | 'warg' | undefined {
  if ((option & OPTION_FALCON) !== 0) {
    return 'falcon';
  }
  if ((option & OPTION_WUG) !== 0) {
    return 'warg';
  }
  return undefined;
}

/** The cart drawn behind a player, 1 to 5, if any. */
export function cartLook(option: number): number | undefined {
  const i = CARTS.findIndex((bit) => (option & bit) !== 0);
  return i < 0 ? undefined : i + 1;
}

/**
 * The job whose body sprite a player shows. A mounted third class shows
 * its mounted job, as the official client does. Other jobs are unchanged.
 */
export function mountedBody(job: number, option: number): number {
  for (const [bit, byJob] of MOUNTED) {
    if ((option & bit) !== 0) {
      const mounted = byJob.get(job);
      if (mounted !== undefined) {
        return mounted;
      }
    }
  }
  return job;
}

/**
 * The look name of a player's body. A mounted player whose job has no
 * mounted sprite shows the job's costume mount look from jobs.yml, when
 * it has one. Unknown jobs show the novice.
 */
export function bodyLook(job: number, option: number): string {
  const body = mountedBody(job, option);
  const info = jobs.get(body) ?? jobs.get(0);
  const look = info?.look ?? 'novice';
  if ((option & MOUNTS) === 0 || MOUNTED_JOBS.has(body)) {
    return look;
  }
  return info?.mount ?? look;
}
