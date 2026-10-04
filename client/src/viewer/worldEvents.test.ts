import { describe, expect, it } from 'vitest';
import { EventRuns, type WorldEventRun, mapLabel, noticeText, remainingText } from './worldEvents';

const boss: WorldEventRun = {
  name: 'core:Field_Boss',
  displayName: 'Field Boss',
  type: 'boss',
  map: 'map/prt_fild08',
  endsAtMs: 0,
};

describe('world events', () => {
  it('writes the banner of a start and an end', () => {
    expect(noticeText(boss, true, '')).toBe('Boss: Field Boss has started on prt_fild08.');
    expect(noticeText(boss, false, 'won')).toBe('Boss: Field Boss is over. It was won.');
    expect(noticeText(boss, false, 'lost')).toBe('Boss: Field Boss is over. It was lost.');
    expect(noticeText(boss, false, 'stopped')).toBe('Boss: Field Boss was stopped.');
    const party = { ...boss, type: 'script', displayName: 'Pumpkin Night', map: '' };
    expect(noticeText(party, true, '')).toBe('Pumpkin Night has started.');
    expect(noticeText(party, false, '')).toBe('Pumpkin Night is over.');
    expect(noticeText({ ...boss, type: 'treasure_hunt', displayName: 'Hunt' }, true, '')).toBe(
      'Treasure hunt: Hunt has started on prt_fild08.',
    );
  });

  it('names maps and the time left', () => {
    expect(mapLabel('map/prontera')).toBe('prontera');
    expect(mapLabel('placeholder/map')).toBe('placeholder/map');
    expect(remainingText(0, 1000)).toBe('');
    expect(remainingText(30_000, 0)).toBe('under a minute left');
    expect(remainingText(42 * 60_000, 30_000)).toBe('42 min left');
    expect(remainingText(5 * 3_600_000, 0)).toBe('5 h left');
  });

  it('keeps the runs that last', () => {
    const e = new EventRuns();
    e.set([boss, { ...boss, name: 'core:Raid', displayName: 'Arena Raid', type: 'invasion' }]);
    expect(e.list().map((r) => r.displayName)).toEqual(['Arena Raid', 'Field Boss']);
    e.notice(boss, false);
    expect(e.list().map((r) => r.name)).toEqual(['core:Raid']);
    e.notice(boss, true);
    expect(e.list()).toHaveLength(2);
  });
});
