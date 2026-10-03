import { describe, expect, it } from 'vitest';
import { bodyLook, cartLook, companionLook, mountedBody } from './mounts';

const FALCON = 0x10;
const RIDING = 0x20;
const WUG = 0x100000;
const WUGRIDER = 0x200000;
const MADOGEAR = 0x400000;
const DRAGON1 = 0x80000;
const DRAGON5 = 0x4000000;

describe('companionLook', () => {
  it('shows a falcon or a warg', () => {
    expect(companionLook(0)).toBeUndefined();
    expect(companionLook(FALCON)).toBe('falcon');
    expect(companionLook(WUG)).toBe('warg');
    // A warg rider has no warg beside it.
    expect(companionLook(WUGRIDER)).toBeUndefined();
  });
});

describe('cartLook', () => {
  it('shows carts 1 to 5', () => {
    expect(cartLook(0)).toBeUndefined();
    expect(cartLook(0x8)).toBe(1);
    expect(cartLook(0x80)).toBe(2);
    expect(cartLook(0x100)).toBe(3);
    expect(cartLook(0x200)).toBe(4);
    expect(cartLook(0x400 | FALCON)).toBe(5);
  });
});

describe('mountedBody', () => {
  it('mounts third classes, transcendent and baby', () => {
    const cases: [number, number, number][] = [
      [RIDING, 4066, 4082],
      [RIDING, 4073, 4083],
      [RIDING, 4102, 4110],
      [DRAGON1, 4054, 4080],
      [DRAGON5, 4060, 4081],
      [DRAGON1, 4096, 4109],
      [WUGRIDER, 4056, 4084],
      [WUGRIDER, 4062, 4085],
      [WUGRIDER, 4098, 4111],
      [MADOGEAR, 4058, 4086],
      [MADOGEAR, 4064, 4087],
      [MADOGEAR, 4100, 4112],
    ];
    for (const [option, job, mounted] of cases) {
      expect(mountedBody(job, option)).toBe(mounted);
    }
  });

  it('keeps other jobs and jobs without their mount bit', () => {
    expect(mountedBody(4054, 0)).toBe(4054);
    expect(mountedBody(4054, RIDING)).toBe(4054);
    expect(mountedBody(4058, WUGRIDER)).toBe(4058);
    expect(mountedBody(11, RIDING)).toBe(11);
  });
});

describe('bodyLook', () => {
  it('shows the job look on foot', () => {
    expect(bodyLook(11, 0)).toBe('hunter');
    expect(bodyLook(4054, 0)).toBe('rune_knight');
  });

  it('shows the mounted sprite of a mounted job', () => {
    expect(bodyLook(4054, DRAGON1)).toBe('rune_knight2');
    // The server sends Knight 13 for a Peco Peco rider.
    expect(bodyLook(13, RIDING)).toBe('knight2');
    expect(bodyLook(4036, RIDING)).toBe('knight2');
  });

  it('shows the costume mount of a job with no mounted sprite', () => {
    expect(bodyLook(11, RIDING)).toBe('ostrich_hunter');
    expect(bodyLook(24, RIDING)).toBe('peco_gunner');
    expect(bodyLook(7, RIDING)).toBe('lion_knight');
  });
});
