import { describe, expect, it } from 'vitest';
import { blockChanges } from './blockList';

describe('blockChanges', () => {
  it('names the added and removed names', () => {
    expect(blockChanges(['Bob'], ['Bob', 'Eve'])).toEqual(['You blocked Eve.']);
    expect(blockChanges(['Bob', 'Eve'], ['Eve'])).toEqual(['You unblocked Bob.']);
    expect(blockChanges(['Bob'], ['Bob'])).toEqual([]);
  });
});
