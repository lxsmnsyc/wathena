import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { QuestInfoSchema } from '../gen/wathena/v1/event_pb';
import {
  type QuestView,
  levelText,
  objectiveText,
  questGroups,
  questIconMark,
  questOf,
  questRemoved,
  questUpdated,
  timeLeft,
  timerText,
} from './quests';

function quest(id: number, values: Partial<QuestView> = {}): QuestView {
  return {
    id,
    title: `Quest ${id}`,
    active: true,
    limitMs: -1,
    receivedAt: 0,
    objectives: [],
    ...values,
  };
}

describe('questOf', () => {
  it('copies a QuestInfo into plain data', () => {
    const info = create(QuestInfoSchema, {
      id: 7,
      title: 'Poring hunt',
      active: true,
      limitMs: 60_000n,
      objectives: [
        { label: 'Poring', monster: 1002, count: 2, goal: 3, minLevel: 10, maxLevel: 20 },
      ],
    });
    expect(questOf(info, 500)).toEqual({
      id: 7,
      title: 'Poring hunt',
      active: true,
      limitMs: 60_000,
      receivedAt: 500,
      objectives: [
        { label: 'Poring', monster: 1002, count: 2, goal: 3, minLevel: 10, maxLevel: 20 },
      ],
    });
  });

  it('keeps -1 for a quest without a limit', () => {
    expect(questOf(create(QuestInfoSchema, { id: 1, limitMs: -1n }), 0).limitMs).toBe(-1);
  });
});

describe('the quest log', () => {
  const log = [quest(1), quest(2), quest(3)];

  it('replaces a quest in its place', () => {
    const next = questUpdated(log, quest(2, { active: false }));
    expect(next.map((q) => [q.id, q.active])).toEqual([
      [1, true],
      [2, false],
      [3, true],
    ]);
    // The log before stays as it was.
    expect(log[1]?.active).toBe(true);
  });

  it('adds a new quest at the end', () => {
    expect(questUpdated(log, quest(9)).map((q) => q.id)).toEqual([1, 2, 3, 9]);
  });

  it('removes a quest, and ignores an ID it lacks', () => {
    expect(questRemoved(log, 2).map((q) => q.id)).toEqual([1, 3]);
    expect(questRemoved(log, 5).map((q) => q.id)).toEqual([1, 2, 3]);
  });

  it('groups active quests, then inactive ones, in log order', () => {
    const groups = questGroups([
      quest(1, { active: false }),
      quest(2),
      quest(3, { active: false }),
      quest(4),
    ]);
    expect(groups.active.map((q) => q.id)).toEqual([2, 4]);
    expect(groups.inactive.map((q) => q.id)).toEqual([1, 3]);
  });
});

describe('timers', () => {
  it('counts down from when the message arrived', () => {
    const q = quest(1, { limitMs: 60_000, receivedAt: 1000 });
    expect(timeLeft(q, 1000)).toBe(60_000);
    expect(timeLeft(q, 31_000)).toBe(30_000);
    expect(timeLeft(q, 100_000)).toBe(0);
    // A clock before the arrival takes nothing off.
    expect(timeLeft(q, 0)).toBe(60_000);
    expect(timeLeft(quest(2), 5000)).toBe(-1);
    expect(timeLeft(quest(3, { limitMs: 0 }), 5000)).toBe(0);
  });

  it('shows the time left as text', () => {
    expect(timerText(-1)).toBe('No time limit');
    expect(timerText(0)).toBe('Time is up');
    expect(timerText(1)).toBe('Time left 0:00:01');
    expect(timerText(3_552_000)).toBe('Time left 0:59:12');
    expect(timerText(3_600_000)).toBe('Time left 1:00:00');
    expect(timerText(86_400_000)).toBe('Time left 1 day 0:00:00');
    expect(timerText(2 * 86_400_000 + 3 * 3_600_000 + 4 * 60_000 + 5000)).toBe(
      'Time left 2 days 3:04:05',
    );
  });
});

describe('objectives', () => {
  it('shows a level range when it is set', () => {
    expect(levelText(0, 0)).toBe('');
    expect(levelText(10, 20)).toBe('Lv 10-20');
    expect(levelText(10, 10)).toBe('Lv 10');
    expect(levelText(10, 0)).toBe('Lv 10+');
    expect(levelText(0, 20)).toBe('Lv 1-20');
  });

  it('shows the label, the count and the goal', () => {
    const o = { label: 'Poring', monster: 1002, count: 2, goal: 3, minLevel: 0, maxLevel: 0 };
    expect(objectiveText(o)).toBe('Poring 2/3');
    expect(objectiveText({ ...o, minLevel: 10, maxLevel: 20 })).toBe('Poring 2/3 Lv 10-20');
    expect(objectiveText({ ...o, label: 'Prontera Fields, Plant, Small', monster: 0 })).toBe(
      'Prontera Fields, Plant, Small 2/3',
    );
  });
});

describe('questIconMark', () => {
  it('maps each icon name to a mark and a kind', () => {
    expect(questIconMark('quest')).toEqual({ mark: '!', kind: 'quest' });
    expect(questIconMark('quest_question')).toEqual({ mark: '?', kind: 'quest' });
    expect(questIconMark('job')).toEqual({ mark: '!', kind: 'job' });
    expect(questIconMark('job_question')).toEqual({ mark: '?', kind: 'job' });
    expect(questIconMark('event')).toEqual({ mark: '!', kind: 'event' });
    expect(questIconMark('event_question')).toEqual({ mark: '?', kind: 'event' });
    expect(questIconMark('warg')).toEqual({ mark: 'W', kind: 'warg' });
    expect(questIconMark('warg_face')).toEqual({ mark: 'W', kind: 'warg' });
    expect(questIconMark('unknown')).toEqual({ mark: '!', kind: 'quest' });
  });
});
