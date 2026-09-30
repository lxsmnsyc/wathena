import { describe, expect, it } from 'vitest';
import { effects, weapons } from './tables';

describe('tables', () => {
  it('loads the YAML tables', () => {
    expect(weapons.get(11)?.constant).toBe('W_BOW');
    expect(effects.get(109)?.parts[0]).toEqual({
      variants: ['effect/bubble1', 'effect/bubble2', 'effect/bubble3', 'effect/bubble4'],
    });
  });
});
