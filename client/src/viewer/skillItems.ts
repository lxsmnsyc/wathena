// The item list of a skill that asks for an item, as RO's Arrow Crafting,
// Weapon Repair and Upgrade Weapon windows. The server sends
// SkillItemList after the cast; a click on a line picks it, and Cancel or
// Escape closes the list. The server checks the pick again and answers
// with inventory changes and a system message. Item names are only ever
// set with textContent.

import { button, element } from './forms';
import { refinedLabel } from './itemText';

/** A line of the list, as SkillListItem. */
export interface SkillItemLine {
  itemId: number;
  refine: number;
  cards: number[];
}

/** Skill IDs of the skills that open the list. */
export const SKILL_ARROW_CRAFTING = 147;
export const SKILL_WEAPON_REPAIR = 108;
export const SKILL_UPGRADE_WEAPON = 477;

/** The window's title for a skill. */
export function skillItemTitle(skillId: number): string {
  switch (skillId) {
    case SKILL_ARROW_CRAFTING:
      return 'Arrow Crafting';
    case SKILL_WEAPON_REPAIR:
      return 'Weapon Repair';
    case SKILL_UPGRADE_WEAPON:
      return 'Upgrade Weapon';
    default:
      return 'Choose an item';
  }
}

/** What an empty list says. */
export function skillItemEmpty(skillId: number): string {
  return skillId === SKILL_UPGRADE_WEAPON ? 'No weapon can be upgraded.' : 'No item can be used.';
}

/** A line as it shows: "+7 Sword [3]", with its cards after it. */
export function skillItemText(line: SkillItemLine, label: (itemId: number) => string): string {
  const name = refinedLabel(label(line.itemId), line.refine);
  if (line.cards.length === 0) {
    return name;
  }
  return `${name} (${line.cards.map(label).join(', ')})`;
}

/** What the window sends. */
export interface SkillItemActions {
  pick(skillId: number, index: number): void;
  cancel(skillId: number): void;
}

/** The skill item list window. */
export class SkillItemsWindow {
  private readonly title: HTMLElement;
  private readonly list: HTMLOListElement;
  private readonly empty: HTMLElement;
  private skillId = 0;

  constructor(
    private readonly root: HTMLElement,
    private readonly actions: SkillItemActions,
    private readonly label: (itemId: number) => string,
  ) {
    this.title = element('h2', 'skill-items-title');
    this.list = element('ol', 'skill-items-list');
    this.empty = element('p', 'skill-items-empty');
    const cancel = button('Cancel', () => {
      this.cancel();
    });
    root.append(this.title, this.list, this.empty, cancel);
    root.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        this.cancel();
      }
    });
  }

  /** Shows a skill's list, replacing the one before. */
  show(skillId: number, lines: SkillItemLine[]): void {
    this.skillId = skillId;
    this.title.textContent = skillItemTitle(skillId);
    this.list.replaceChildren();
    for (const [index, line] of lines.entries()) {
      const li = element('li');
      const b = button(skillItemText(line, this.label), () => {
        this.root.hidden = true;
        this.actions.pick(this.skillId, index);
      });
      li.append(b);
      this.list.append(li);
    }
    this.empty.textContent = lines.length === 0 ? skillItemEmpty(skillId) : '';
    this.empty.hidden = lines.length > 0;
    this.root.hidden = false;
    this.list.querySelector('button')?.focus();
  }

  private cancel(): void {
    if (this.root.hidden) {
      return;
    }
    this.root.hidden = true;
    this.actions.cancel(this.skillId);
  }
}
