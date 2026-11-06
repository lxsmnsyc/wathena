import { describe, expect, it } from 'vitest';
import {
  SKILL_ARROW_CRAFTING,
  SKILL_UPGRADE_WEAPON,
  skillItemEmpty,
  skillItemText,
  skillItemTitle,
} from './skillItems';

const label = (id: number): string => (id === 1101 ? 'Sword [3]' : `Item ${String(id)}`);

describe('skillItemText', () => {
  it('shows the refine and the cards', () => {
    expect(skillItemText({ itemId: 1101, refine: 0, cards: [] }, label)).toBe('Sword [3]');
    expect(skillItemText({ itemId: 1101, refine: 7, cards: [4001] }, label)).toBe(
      '+7 Sword [3] (Item 4001)',
    );
  });
});

describe('skillItemTitle', () => {
  it('names the skill', () => {
    expect(skillItemTitle(SKILL_ARROW_CRAFTING)).toBe('Arrow Crafting');
    expect(skillItemTitle(1)).toBe('Choose an item');
  });

  it('says why a list is empty', () => {
    expect(skillItemEmpty(SKILL_UPGRADE_WEAPON)).toBe('No weapon can be upgraded.');
  });
});
