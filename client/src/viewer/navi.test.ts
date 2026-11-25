import { describe, expect, it } from 'vitest';
import { type NaviRoute, naviAngle, naviMaps, naviNext, naviSummary, parseNaviInfo } from './navi';

function route(over: Partial<NaviRoute>): NaviRoute {
  return {
    map: 'payon',
    x: 0,
    y: 0,
    monster: 0,
    services: 0,
    hideWindow: false,
    found: true,
    steps: [],
    cancel: false,
    ...over,
  };
}

describe('parseNaviInfo', () => {
  it('reads the fields of the guides', () => {
    expect(parseNaviInfo('[Pub]', 'prontera,150,160,0,000,0')).toEqual({
      name: '[Pub]',
      map: 'prontera',
      x: 150,
      y: 160,
      services: 0,
      monster: 0,
      hideWindow: false,
    });
  });

  it('reads the hide field and lowers the map', () => {
    expect(parseNaviInfo('x', 'GEFFEN,1,2,101,1002,1')).toMatchObject({
      map: 'geffen',
      services: 101,
      monster: 1002,
      hideWindow: true,
    });
  });

  it('takes a link with a cell only', () => {
    expect(parseNaviInfo('x', 'alberta,23,238')).toMatchObject({ x: 23, y: 238, services: 0 });
  });

  it('refuses what is not a map and a cell', () => {
    expect(parseNaviInfo('x', 'prontera')).toBeUndefined();
    expect(parseNaviInfo('x', 'pront era,1,2')).toBeUndefined();
    expect(parseNaviInfo('x', 'prontera,1,2,300')).toBeUndefined();
    expect(parseNaviInfo('x', 'prontera,70000,2')).toBeUndefined();
    expect(parseNaviInfo('x', '<b>,1,2')).toBeUndefined();
    expect(parseNaviInfo('x', 'prontera,1,2,0,0,0,0')).toBeUndefined();
  });
});

describe('route helpers', () => {
  const steps = [
    { map: 'prontera', x: 20, y: 20 },
    { map: 'geffen', x: 30, y: 31 },
  ];

  it('lists the maps of a route up to the target', () => {
    expect(naviMaps(route({ steps }))).toEqual(['prontera', 'geffen', 'payon']);
    expect(naviMaps(route({ found: false }))).toEqual(['payon']);
    expect(naviMaps(route({ steps: [] }))).toEqual(['payon']);
  });

  it('points at the first leg, or the target on its map', () => {
    expect(naviNext(route({ steps }))).toEqual([20, 20]);
    expect(naviNext(route({ x: 5, y: 6 }))).toEqual([5, 6]);
    expect(naviNext(route({}))).toBeUndefined();
    expect(naviNext(route({ found: false, x: 5, y: 6 }))).toBeUndefined();
  });

  it('turns the arrow clockwise from up, with y to the north', () => {
    expect(naviAngle([10, 10], [10, 20])).toBeCloseTo(0);
    expect(naviAngle([10, 10], [20, 10])).toBeCloseTo(90);
    expect(naviAngle([10, 10], [10, 0])).toBeCloseTo(180);
    expect(naviAngle([10, 10], [0, 10])).toBeCloseTo(-90);
  });

  it('says what the route does', () => {
    expect(naviSummary(route({ steps }))).toBe('To payon: 2 map changes.');
    expect(naviSummary(route({ steps: steps.slice(0, 1), x: 1, y: 2 }))).toBe(
      'To payon 1,2: 1 map change.',
    );
    expect(naviSummary(route({ found: false }))).toBe('No route to payon.');
    expect(naviSummary(route({}))).toBe('You are on payon.');
  });
});
