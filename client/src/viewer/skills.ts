import { modSkillName, skillIcon } from '../assets/art';
import type { AssetStore } from '../assets/store';
import { SkillFailure, type SkillInfo, SkillTarget, StatId } from '../gen/wathena/v1/event_pb';
import { cooldownText } from './cooldowns';
import { SKILL_DRAG_TYPE, draggedSkill, showBadge } from './hotkeys';

// The skill window, as RO's Alt+S window: the player's skills with their
// level, SP cost, and a "+" button while skill points remain. Skills not
// learned yet show what they need. A double click or the Use button
// casts a skill, and rows can be dragged onto the hotkey bar. A right
// click puts a skill in the first empty hotkey slot. Passive skills are
// listed but cannot be cast.

/** One skill of the player's skill list. */
export interface Skill {
  id: number;
  /** The learned level, 0 for a skill the player may learn. */
  level: number;
  maxLevel: number;
  /** The skill's key, such as SM_BASH. */
  name: string;
  displayName: string;
  target: SkillTarget;
  sp: number;
  range: number;
  upgradable: boolean;
  /** The skill whose icon and effects show. */
  icon: number;
  requires: { skillId: number; level: number }[];
}

/**
 * A skill's name to show: the name the server's client data gives a mod's
 * skill, or the skill list's display name, or its key.
 */
export function skillName(s: Pick<Skill, 'id' | 'displayName' | 'name'>): string {
  return modSkillName(s.id) ?? (s.displayName || s.name);
}

/** A skill from the server's SkillInfo. */
export function skillOf(s: SkillInfo): Skill {
  return {
    id: s.skillId,
    level: s.level,
    maxLevel: s.maxLevel,
    name: s.name,
    displayName: s.displayName,
    target: s.target,
    sp: s.sp,
    range: s.range,
    upgradable: s.upgradable,
    icon: s.icon || s.skillId,
    requires: s.requires.map((r) => ({ skillId: r.skillId, level: r.level })),
  };
}

/** The player's skill list, as the server last sent it. */
export class SkillModel {
  private skills = new Map<number, Skill>();
  /** Goes up on every change, so views know when to rebuild. */
  version = 0;

  /** Replaces the list, as SkillList. */
  reset(skills: readonly Skill[]): void {
    this.skills = new Map(skills.map((s) => [s.id, s]));
    this.version++;
  }

  get(id: number): Skill | undefined {
    return this.skills.get(id);
  }

  /** Every skill, in the server's order. */
  all(): Skill[] {
    return [...this.skills.values()];
  }

  /**
   * The skill whose icon and effects a skill uses. A mod's skill names a
   * core skill. Skills not in the list use their own.
   */
  iconOf(id: number): number {
    return this.skills.get(id)?.icon ?? id;
  }

  /** A skill's name to show, or "Skill <id>" for one not in the list. */
  label(id: number): string {
    const s = this.skills.get(id);
    return s === undefined ? (modSkillName(id) ?? `Skill ${id}`) : skillName(s);
  }
}

/**
 * The icons of skills whose records name art (art.ts), loaded once per
 * skill as blob URLs. Other skills keep their text badge. A missing icon
 * shows placeholder/texture, and the store logs the name once.
 */
export class SkillIcons {
  private readonly urls = new Map<number, string | undefined>();
  private readonly loading = new Set<number>();
  /** Goes up when an icon loads, so views know to show it. */
  version = 0;

  constructor(private readonly store: Pick<AssetStore, 'bytes'>) {}

  /** A skill's icon as a URL once loaded. Asking starts the load. */
  url(id: number): string | undefined {
    const known = this.urls.get(id);
    if (known !== undefined || this.loading.has(id)) {
      return known;
    }
    const name = skillIcon(id);
    if (name === undefined) {
      return undefined;
    }
    this.loading.add(id);
    this.load(name)
      .then((url) => {
        this.urls.set(id, url);
        this.version++;
      })
      .catch((error: unknown) => {
        console.error(`skill icon ${id}:`, error);
      });
    return undefined;
  }

  private async load(name: string): Promise<string | undefined> {
    const data =
      (await this.store.bytes(name, 'texture')) ??
      (await this.store.bytes('placeholder/texture', 'texture'));
    return data === undefined
      ? undefined
      : URL.createObjectURL(new Blob([data], { type: 'image/webp' }));
  }
}

/** Whether a skill can be cast: learned and not passive. */
export function castable(s: Skill): boolean {
  return s.level > 0 && s.target !== SkillTarget.PASSIVE && s.target !== SkillTarget.UNSPECIFIED;
}

/**
 * A short text that stands in for a skill's icon: the initials of a name
 * of several words, or the first two letters of one word.
 */
export function skillBadge(name: string): string {
  const words = name.split(/[\s_]+/).filter((w) => w !== '');
  if (words.length === 0) {
    return '?';
  }
  if (words.length === 1) {
    const w = words[0]!;
    return w.charAt(0).toUpperCase() + w.slice(1, 2).toLowerCase();
  }
  return words
    .slice(0, 2)
    .map((w) => w.charAt(0).toUpperCase())
    .join('');
}

/**
 * What a skill not learned yet still needs, such as "Needs Bash 5,
 * Endure 1". Requirements the player already meets are left out.
 */
export function requirementText(s: Skill, model: SkillModel): string {
  if (s.level > 0) {
    return '';
  }
  const missing = s.requires.filter((r) => (model.get(r.skillId)?.level ?? 0) < r.level);
  if (missing.length === 0) {
    return '';
  }
  return `Needs ${missing.map((r) => `${model.label(r.skillId)} ${r.level}`).join(', ')}`;
}

/** The player's skill points, or undefined before the server sends them. */
export function skillPoints(stats: ReadonlyMap<StatId, bigint>): bigint | undefined {
  return stats.get(StatId.SKILL_POINTS);
}

/** One row of the skill window. */
export interface SkillRow {
  id: number;
  name: string;
  badge: string;
  /** Such as "Lv 3 / 10". */
  level: string;
  /** Such as "15 SP", or '' for passive and unlearned skills. */
  sp: string;
  passive: boolean;
  learned: boolean;
  canUse: boolean;
  canLearn: boolean;
  needs: string;
}

/** The skill window's rows. */
export function skillRows(model: SkillModel, points: bigint | undefined): SkillRow[] {
  return model.all().map((s) => {
    const passive = s.target === SkillTarget.PASSIVE;
    return {
      id: s.id,
      name: skillName(s),
      badge: skillBadge(skillName(s)),
      level: `Lv ${s.level} / ${s.maxLevel}`,
      sp: s.level > 0 && !passive && s.sp > 0 ? `${s.sp} SP` : '',
      passive,
      learned: s.level > 0,
      canUse: castable(s),
      canLearn: s.upgradable && points !== undefined && points > 0n,
      needs: requirementText(s, model),
    };
  });
}

/** The level to cast a skill at: the chosen one, at most the learned level. */
export function castLevel(s: Skill, chosen: number | undefined): number {
  if (chosen === undefined || chosen < 1) {
    return s.level;
  }
  return Math.min(chosen, s.level);
}

/** What to tell the player when a skill fails, in RO's words where known. */
export function skillFailureMessage(reason: SkillFailure): string {
  switch (reason) {
    case SkillFailure.SP_INSUFFICIENT:
      return 'Insufficient SP.';
    case SkillFailure.HP_INSUFFICIENT:
      return 'Insufficient HP.';
    case SkillFailure.STUFF_INSUFFICIENT:
      return 'You do not have the items this skill needs.';
    case SkillFailure.SKILLINTERVAL:
      return 'You cannot use skills yet.';
    case SkillFailure.MONEY:
      return 'Insufficient Zeny.';
    case SkillFailure.THIS_WEAPON:
      return 'This skill cannot be used with this weapon.';
    case SkillFailure.REDJAMSTONE:
      return 'Red Gemstone required.';
    case SkillFailure.BLUEJAMSTONE:
      return 'Blue Gemstone required.';
    case SkillFailure.WEIGHTOVER:
      return 'You are carrying too much to use skills.';
    case SkillFailure.ARROW:
      return 'Please equip the proper ammunition first.';
    case SkillFailure.CART:
      return 'You need a cart to use this skill.';
    case SkillFailure.POS:
      return 'You cannot use this skill here.';
    case SkillFailure.RANGE:
      return 'Out of range.';
    case SkillFailure.TARGET:
      return 'Invalid target.';
    case SkillFailure.STATUS:
      return 'You cannot use skills now.';
    case SkillFailure.HOLYWATER:
      return 'An Empty Bottle is required.';
    default:
      return 'Skill has failed.';
  }
}

interface RowElements {
  li: HTMLLIElement;
  learn: HTMLButtonElement;
  use: HTMLButtonElement | undefined;
  level: HTMLSelectElement | undefined;
}

/** The skill window on the page. */
export class SkillWindow {
  private readonly points: HTMLElement;
  private readonly list: HTMLUListElement;
  private rows = new Map<number, RowElements>();
  private shownVersion = -1;
  private shownIcons = -1;
  /** The level the player chose for each skill, when lower than learned. */
  private readonly chosen = new Map<number, number>();

  constructor(
    private readonly root: HTMLElement,
    private readonly model: SkillModel,
    private readonly actions: {
      learn: (skillId: number) => void;
      use: (skillId: number, level: number) => void;
      /** Puts a skill at a level in the first empty hotkey slot. */
      assign: (skillId: number, level: number | undefined) => void;
    },
    private readonly icons?: SkillIcons,
  ) {
    const head = document.createElement('div');
    head.className = 'skill-head';
    const title = document.createElement('span');
    title.textContent = 'Skills';
    this.points = document.createElement('span');
    head.append(title, this.points);
    this.list = document.createElement('ul');
    const hint = document.createElement('div');
    hint.className = 'skill-hint';
    hint.textContent =
      'Double-click to use. Drag to F1 to F9, or right-click for the first free key.';
    root.append(head, this.list, hint);
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  toggle(): void {
    this.root.hidden = !this.root.hidden;
  }

  /** The level chosen for a skill in the window, if lower than learned. */
  chosenLevel(skillId: number): number | undefined {
    return this.chosen.get(skillId);
  }

  /** Shows the list, the points, and each skill's cooldown. */
  update(stats: ReadonlyMap<StatId, bigint>, cooldown: (skillId: number) => number): void {
    const points = skillPoints(stats);
    const rows = skillRows(this.model, points);
    const icons = this.icons?.version ?? 0;
    if (this.shownVersion !== this.model.version || this.shownIcons !== icons) {
      this.shownVersion = this.model.version;
      this.shownIcons = icons;
      this.build(rows);
    }
    setText(this.points, `Skill Points: ${points ?? '-'}`);
    for (const r of rows) {
      const el = this.rows.get(r.id);
      if (el === undefined) {
        continue;
      }
      if (el.learn.disabled === r.canLearn) {
        el.learn.disabled = !r.canLearn;
      }
      if (el.use !== undefined) {
        const left = cooldownText(cooldown(r.id));
        setText(el.use, left === '' ? 'Use' : left);
        el.li.classList.toggle('waiting', left !== '');
      }
    }
  }

  private build(rows: SkillRow[]): void {
    this.list.replaceChildren();
    this.rows = new Map();
    for (const r of rows) {
      const skill = this.model.get(r.id);
      if (skill === undefined) {
        continue;
      }
      const li = document.createElement('li');
      li.className = r.learned ? 'skill' : 'skill unlearned';
      const badge = document.createElement('span');
      badge.className = 'badge';
      showBadge(badge, r.badge, this.icons?.url(r.id));
      const text = document.createElement('div');
      text.className = 'skill-text';
      const name = document.createElement('span');
      name.className = 'name';
      name.textContent = r.name;
      const detail = document.createElement('span');
      detail.className = 'detail';
      detail.textContent = [r.level, r.sp, r.passive ? 'Passive' : '', r.needs]
        .filter((t) => t !== '')
        .join(' · ');
      text.append(name, detail);
      li.append(badge, text);
      let level: HTMLSelectElement | undefined;
      let use: HTMLButtonElement | undefined;
      if (r.canUse) {
        if (skill.level > 1) {
          level = document.createElement('select');
          level.title = 'Level to use';
          for (let lv = skill.level; lv >= 1; lv--) {
            level.add(new Option(`Lv ${lv}`, String(lv)));
          }
          const chosen = this.chosen.get(r.id);
          level.value = String(castLevel(skill, chosen));
          const select = level;
          select.addEventListener('change', () => {
            const lv = Number(select.value);
            if (lv >= skill.level) {
              this.chosen.delete(r.id);
            } else {
              this.chosen.set(r.id, lv);
            }
          });
          li.append(level);
        }
        use = document.createElement('button');
        use.type = 'button';
        use.className = 'use';
        use.textContent = 'Use';
        use.addEventListener('click', () => {
          this.use(skill.id);
        });
        li.append(use);
        li.draggable = true;
        li.title = `${r.name}: double-click to use, drag to a hotkey`;
        li.addEventListener('dblclick', (e) => {
          if (e.target instanceof HTMLButtonElement || e.target instanceof HTMLSelectElement) {
            return;
          }
          this.use(skill.id);
        });
        li.addEventListener('dragstart', (e) => {
          const lv = this.chosen.get(skill.id);
          e.dataTransfer?.setData(
            SKILL_DRAG_TYPE,
            draggedSkill(
              lv === undefined ? { skillId: skill.id } : { skillId: skill.id, level: lv },
            ),
          );
          if (e.dataTransfer !== null) {
            e.dataTransfer.effectAllowed = 'copy';
          }
        });
        li.addEventListener('contextmenu', (e) => {
          e.preventDefault();
          this.actions.assign(skill.id, this.chosen.get(skill.id));
        });
      }
      const learn = document.createElement('button');
      learn.type = 'button';
      learn.className = 'learn';
      learn.textContent = '+';
      learn.title = `Learn ${r.name}`;
      learn.disabled = !r.canLearn;
      learn.hidden = !skill.upgradable;
      learn.addEventListener('click', () => {
        this.actions.learn(skill.id);
      });
      li.append(learn);
      this.list.append(li);
      this.rows.set(r.id, { li, learn, use, level });
    }
  }

  private use(skillId: number): void {
    const s = this.model.get(skillId);
    if (s !== undefined && castable(s)) {
      this.actions.use(s.id, castLevel(s, this.chosen.get(s.id)));
    }
  }
}

function setText(el: HTMLElement, text: string): void {
  if (el.textContent !== text) {
    el.textContent = text;
  }
}
