import { describe, expect, it } from 'vitest';
import { BaseStat } from '../gen/wathena/v1/command_pb';
import { StatId } from '../gen/wathena/v1/event_pb';
import { detailRows, raiseRows } from './statusWindow';

describe('raiseRows', () => {
  it('waits for the server', () => {
    const rows = raiseRows(new Map());
    expect(rows.map((r) => r.name)).toEqual(['STR', 'AGI', 'VIT', 'INT', 'DEX', 'LUK']);
    expect(rows.every((r) => r.value === '-' && r.cost === '' && !r.canRaise)).toBe(true);
  });

  it('enables a raise the status points pay for', () => {
    const stats = new Map<StatId, bigint>([
      [StatId.STATUS_POINTS, 3n],
      [StatId.STR, 11n],
      [StatId.STR_COST, 3n],
      [StatId.AGI, 21n],
      [StatId.AGI_COST, 4n],
      [StatId.VIT, 99n],
      [StatId.VIT_COST, 0n],
    ]);
    const [str, agi, vit, int] = raiseRows(stats);
    expect(str).toEqual({
      name: 'STR',
      stat: BaseStat.STR,
      value: '11',
      cost: '3',
      canRaise: true,
    });
    expect(agi?.canRaise).toBe(false);
    expect(agi?.cost).toBe('4');
    // A stat at its maximum shows no cost and cannot rise.
    expect(vit).toMatchObject({ value: '99', cost: '', canRaise: false });
    expect(int).toMatchObject({ value: '-', canRaise: false });
  });

  it('needs the status points', () => {
    const [str] = raiseRows(new Map([[StatId.STR_COST, 2n]]));
    expect(str?.canRaise).toBe(false);
  });
});

describe('detailRows', () => {
  it('shows the battle stats, points, and weight in whole units', () => {
    const stats = new Map<StatId, bigint>([
      [StatId.ATK, 25n],
      [StatId.MATK, 3n],
      [StatId.DEF, 1n],
      [StatId.MDEF, 0n],
      [StatId.HIT, 178n],
      [StatId.FLEE, 102n],
      [StatId.CRITICAL, 13n],
      [StatId.ASPD, 150n],
      [StatId.STATUS_POINTS, 48n],
      [StatId.MAX_WEIGHT, 20300n],
    ]);
    expect(detailRows(stats)).toEqual([
      ['ATK', '25'],
      ['MATK', '3'],
      ['DEF', '1'],
      ['MDEF', '0'],
      ['HIT', '178'],
      ['FLEE', '102'],
      ['CRIT', '13'],
      ['ASPD', '150'],
      ['Status Point', '48'],
      ['Weight', '- / 2030'],
    ]);
    stats.set(StatId.WEIGHT, 125n);
    expect(detailRows(stats).at(-1)).toEqual(['Weight', '12 / 2030']);
  });
});
