import { describe, expect, it } from 'vitest';
import {
  SKILL_ARROW_CRAFTING,
  SKILL_PREPARE_POTION,
  SKILL_UPGRADE_WEAPON,
  skillItemEmpty,
  skillItemText,
  skillItemTitle,
} from './skillItems';

const label = (id: number): string => (id === 1101 ? 'Sword [3]' : `Item ${String(id)}`);

describe('skillItemText', () => {
  it('shows the refine and the cards', () => {
    expect(skillItemText({ itemId: 1101, refine: 0, cards: [], materials: [] }, label)).toBe(
      'Sword [3]',
    );
    expect(skillItemText({ itemId: 1101, refine: 7, cards: [4001], materials: [] }, label)).toBe(
      '+7 Sword [3] (Item 4001)',
    );
  });

  it('shows a recipe with its materials', () => {
    const materials = [
      { itemId: 7144, amount: 0 },
      { itemId: 507, amount: 1 },
      { itemId: 7033, amount: 5 },
    ];
    expect(skillItemText({ itemId: 501, refine: 0, cards: [], materials }, label)).toBe(
      'Item 501: Item 7144, Item 507, 5 Item 7033',
    );
  });
});

describe('skillItemTitle', () => {
  it('names the skill', () => {
    expect(skillItemTitle(SKILL_ARROW_CRAFTING)).toBe('Arrow Crafting');
    expect(skillItemTitle(1)).toBe('Choose an item');
    expect(skillItemTitle(SKILL_PREPARE_POTION)).toBe('Prepare Potion');
    expect(skillItemTitle(0, true)).toBe('Cooking');
    expect(skillItemTitle(0)).toBe('Produce');
  });

  it('says why a list is empty', () => {
    expect(skillItemEmpty(SKILL_UPGRADE_WEAPON)).toBe('No weapon can be upgraded.');
  });
});
