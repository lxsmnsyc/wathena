import { create } from '@bufbuild/protobuf';
import { describe, expect, it, vi } from 'vitest';
import { setArt } from '../assets/art';
import { SkillFailure, SkillInfoSchema, SkillTarget, StatId } from '../gen/wathena/v1/event_pb';
import {
  type Skill,
  SkillIcons,
  SkillModel,
  castLevel,
  castable,
  requirementText,
  skillBadge,
  skillFailureMessage,
  skillOf,
  skillPoints,
  skillRows,
} from './skills';

function skill(s: Partial<Skill> & Pick<Skill, 'id'>): Skill {
  return {
    level: 0,
    maxLevel: 10,
    name: '',
    displayName: '',
    target: SkillTarget.ENEMY,
    sp: 0,
    range: 1,
    upgradable: false,
    icon: s.id,
    requires: [],
    ...s,
  };
}

function model(): SkillModel {
  const m = new SkillModel();
  m.reset([
    skill({
      id: 2,
      name: 'SM_SWORD',
      displayName: 'Sword Mastery',
      level: 3,
      target: SkillTarget.PASSIVE,
    }),
    skill({ id: 5, name: 'SM_BASH', displayName: 'Bash', level: 4, sp: 8, upgradable: true }),
    skill({
      id: 7,
      name: 'SM_MAGNUM',
      displayName: 'Magnum Break',
      requires: [{ skillId: 5, level: 5 }],
      upgradable: false,
    }),
    skill({ id: 3001, name: 'MOD_SLASH', displayName: 'Slash', level: 1, icon: 5 }),
  ]);
  return m;
}

describe('skillOf', () => {
  it('reads SkillInfo, with the skill itself as the icon when none is named', () => {
    const s = skillOf(
      create(SkillInfoSchema, {
        skillId: 28,
        level: 2,
        maxLevel: 10,
        name: 'AL_HEAL',
        displayName: 'Heal',
        target: SkillTarget.SUPPORT,
        sp: 16,
        requires: [{ skillId: 22, level: 1 }],
      }),
    );
    expect(s).toMatchObject({
      id: 28,
      level: 2,
      icon: 28,
      sp: 16,
      requires: [{ skillId: 22, level: 1 }],
    });
  });
});

describe('SkillModel', () => {
  it('keeps the list and counts changes', () => {
    const m = model();
    expect(m.version).toBe(1);
    expect(m.all().map((s) => s.id)).toEqual([2, 5, 7, 3001]);
    expect(m.label(5)).toBe('Bash');
    expect(m.label(99)).toBe('Skill 99');
  });

  it('names mod skills by the client data, and loads their icons', async () => {
    setArt({
      version: 2,
      mods: [],
      items: {},
      monsters: {},
      npc_sprites: {},
      jobs: {},
      skills: {
        '3001': { name: 'Lantern Flare', icon: 'mod/example.lantern/icon/skill/LANTERN_FLARE' },
        '20001': { name: 'Far Flare' },
      },
      companions: {},
      mounts: {},
    });
    try {
      const m = model();
      expect(m.label(3001)).toBe('Lantern Flare');
      expect(m.label(20001)).toBe('Far Flare');
      expect(skillRows(m, 0n).find((r) => r.id === 3001)?.name).toBe('Lantern Flare');
      const asked: string[] = [];
      const icons = new SkillIcons({
        bytes: async (name: string) => {
          asked.push(name);
          return Promise.resolve(name.startsWith('mod/') ? new Uint8Array([1]) : undefined);
        },
      });
      const blob = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:icon');
      expect(icons.url(5)).toBeUndefined();
      expect(icons.url(3001)).toBeUndefined();
      await new Promise((resolve) => {
        setTimeout(resolve, 0);
      });
      blob.mockRestore();
      expect(icons.url(3001)).toBe('blob:icon');
      expect(icons.version).toBe(1);
      expect(asked).toEqual(['mod/example.lantern/icon/skill/LANTERN_FLARE']);
    } finally {
      setArt(undefined);
    }
  });

  it('uses the icon skill of mod skills', () => {
    const m = model();
    expect(m.iconOf(3001)).toBe(5);
    expect(m.iconOf(5)).toBe(5);
    expect(m.iconOf(42)).toBe(42);
  });
});

describe('skillRows', () => {
  it('shows levels, SP, passives, and what unlearned skills need', () => {
    const rows = skillRows(model(), 2n);
    expect(rows[0]).toMatchObject({
      name: 'Sword Mastery',
      level: 'Lv 3 / 10',
      sp: '',
      passive: true,
      canUse: false,
      canLearn: false,
    });
    expect(rows[1]).toMatchObject({ badge: 'Ba', sp: '8 SP', canUse: true, canLearn: true });
    expect(rows[2]).toMatchObject({
      learned: false,
      canUse: false,
      needs: 'Needs Bash 5',
      badge: 'MB',
    });
  });

  it('needs skill points to learn', () => {
    expect(skillRows(model(), 0n)[1]?.canLearn).toBe(false);
    expect(skillRows(model(), undefined)[1]?.canLearn).toBe(false);
  });

  it('reads the skill points', () => {
    expect(skillPoints(new Map([[StatId.SKILL_POINTS, 4n]]))).toBe(4n);
    expect(skillPoints(new Map())).toBeUndefined();
  });
});

describe('skill helpers', () => {
  it('casts only learned skills that are not passive', () => {
    const m = model();
    expect(castable(m.get(2)!)).toBe(false);
    expect(castable(m.get(5)!)).toBe(true);
    expect(castable(m.get(7)!)).toBe(false);
  });

  it('casts at the chosen level, up to the learned one', () => {
    const bash = model().get(5)!;
    expect(castLevel(bash, undefined)).toBe(4);
    expect(castLevel(bash, 2)).toBe(2);
    expect(castLevel(bash, 9)).toBe(4);
    expect(castLevel(bash, 0)).toBe(4);
  });

  it('makes badges from names', () => {
    expect(skillBadge('Heal')).toBe('He');
    expect(skillBadge('Double Strafe')).toBe('DS');
    expect(skillBadge('Increase AGI')).toBe('IA');
    expect(skillBadge('SM_BASH')).toBe('SB');
    expect(skillBadge('')).toBe('?');
  });

  it('lists no requirements for learned skills', () => {
    const m = model();
    expect(requirementText(m.get(5)!, m)).toBe('');
  });

  it('lists only the requirements still missing', () => {
    const m = model();
    expect(requirementText(m.get(7)!, m)).toBe('Needs Bash 5');
    m.reset(m.all().map((s) => (s.id === 5 ? { ...s, level: 5 } : s)));
    expect(requirementText(m.get(7)!, m)).toBe('');
  });
});

describe('skillFailureMessage', () => {
  it('explains why a skill failed', () => {
    expect(skillFailureMessage(SkillFailure.SP_INSUFFICIENT)).toBe('Insufficient SP.');
    expect(skillFailureMessage(SkillFailure.LEVEL)).toBe('Skill has failed.');
    expect(skillFailureMessage(SkillFailure.UNSPECIFIED)).toBe('Skill has failed.');
    expect(skillFailureMessage(SkillFailure.RANGE)).toBe('Out of range.');
    expect(skillFailureMessage(SkillFailure.REDJAMSTONE)).toBe('Red Gemstone required.');
    expect(skillFailureMessage(SkillFailure.BLUEJAMSTONE)).toBe('Blue Gemstone required.');
    expect(skillFailureMessage(SkillFailure.ARROW)).toBe(
      'Please equip the proper ammunition first.',
    );
    expect(skillFailureMessage(SkillFailure.SKILLINTERVAL)).toBe('You cannot use skills yet.');
    // Every reason has its own message.
    const all = Object.values(SkillFailure).filter((v) => typeof v === 'number');
    const messages = new Set(
      all.filter((r) => r !== SkillFailure.UNSPECIFIED).map(skillFailureMessage),
    );
    expect(messages.size).toBe(all.length - 1);
  });
});
