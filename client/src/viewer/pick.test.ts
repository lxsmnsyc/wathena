import { describe, expect, it } from 'vitest';
import nearest, { type Target } from './pick';

const unit: Target = { id: 1, item: false, x: 100, y: 65, radius: 40 };
const item: Target = { id: 2, item: true, x: 100, y: 100, radius: 20 };

describe('nearest', () => {
  it('prefers an item under the click to the unit above it', () => {
    expect(nearest([unit, item], 100, 98)?.id).toBe(2);
  });

  it('prefers the unit when the click is on its body', () => {
    expect(nearest([unit, item], 100, 70)?.id).toBe(1);
  });

  it('ignores targets out of reach', () => {
    expect(nearest([unit, item], 300, 300)).toBeUndefined();
    expect(nearest([item], 100, 125)).toBeUndefined();
  });
});
