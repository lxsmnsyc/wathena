import { describe, expect, it } from 'vitest';
import { SkillTarget } from '../gen/wathena/v1/event_pb';
import { TargetMode, targetKind } from './targeting';

describe('targetKind', () => {
  it('maps each skill target', () => {
    expect(targetKind(SkillTarget.SELF)).toBe('self');
    expect(targetKind(SkillTarget.ENEMY)).toBe('entity');
    expect(targetKind(SkillTarget.SUPPORT)).toBe('entity');
    expect(targetKind(SkillTarget.GROUND)).toBe('cell');
    expect(targetKind(SkillTarget.TRAP)).toBe('cell');
    expect(targetKind(SkillTarget.PASSIVE)).toBeUndefined();
    expect(targetKind(SkillTarget.UNSPECIFIED)).toBeUndefined();
  });
});

describe('TargetMode', () => {
  it('casts a self skill at once', () => {
    const m = new TargetMode();
    expect(m.begin(8, 3, SkillTarget.SELF)).toEqual({
      skillId: 8,
      level: 3,
      target: { case: undefined },
    });
    expect(m.kind).toBeUndefined();
  });

  it('does nothing for a passive skill', () => {
    const m = new TargetMode();
    expect(m.begin(2, 1, SkillTarget.PASSIVE)).toBeUndefined();
    expect(m.kind).toBeUndefined();
  });

  it('waits for an entity for an enemy skill, and not the player', () => {
    const m = new TargetMode();
    expect(m.begin(5, 10, SkillTarget.ENEMY)).toBeUndefined();
    expect(m.kind).toBe('entity');
    expect(m.skillId).toBe(5);
    expect(m.clickCell(1, 2)).toBeUndefined();
    expect(m.clickEntity(1, true)).toBeUndefined();
    expect(m.kind).toBe('entity');
    expect(m.clickEntity(40, false)).toEqual({
      skillId: 5,
      level: 10,
      target: { case: 'entity', value: 40 },
    });
    expect(m.kind).toBeUndefined();
    expect(m.clickEntity(40, false)).toBeUndefined();
  });

  it('lets a support skill target the player', () => {
    const m = new TargetMode();
    m.begin(28, 5, SkillTarget.SUPPORT);
    expect(m.clickEntity(1, true)?.target).toEqual({ case: 'entity', value: 1 });
  });

  it('waits for a cell for a ground skill', () => {
    const m = new TargetMode();
    m.begin(18, 4, SkillTarget.GROUND);
    expect(m.kind).toBe('cell');
    expect(m.clickEntity(3, false)).toBeUndefined();
    expect(m.clickCell(10, 12)).toEqual({
      skillId: 18,
      level: 4,
      target: { case: 'cell', value: { x: 10, y: 12 } },
    });
    expect(m.kind).toBeUndefined();
  });

  it('cancels, and a new skill replaces the waiting one', () => {
    const m = new TargetMode();
    expect(m.cancel()).toBe(false);
    m.begin(18, 4, SkillTarget.GROUND);
    m.begin(5, 1, SkillTarget.ENEMY);
    expect(m.kind).toBe('entity');
    m.begin(8, 1, SkillTarget.SELF);
    expect(m.kind).toBeUndefined();
    m.begin(5, 1, SkillTarget.ENEMY);
    expect(m.cancel()).toBe(true);
    expect(m.kind).toBeUndefined();
  });
});
