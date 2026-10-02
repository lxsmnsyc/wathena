import { describe, expect, it } from 'vitest';
import {
  HOTKEY_SLOTS,
  type HotkeyStorage,
  draggedSkill,
  emptyHotkeys,
  hotkeySlot,
  hotkeyStorageKey,
  loadHotkeys,
  parseDraggedSkill,
  parseHotkeys,
  saveHotkeys,
  serializeHotkeys,
} from './hotkeys';

function memory(): HotkeyStorage & { data: Map<string, string> } {
  const data = new Map<string, string>();
  return {
    data,
    getItem: (k) => data.get(k) ?? null,
    setItem: (k, v) => {
      data.set(k, v);
    },
  };
}

describe('hotkeySlot', () => {
  it('maps F1 to F9 to slots 0 to 8', () => {
    expect(hotkeySlot('F1')).toBe(0);
    expect(hotkeySlot('F9')).toBe(8);
    expect(hotkeySlot('F10')).toBeUndefined();
    expect(hotkeySlot('KeyF')).toBeUndefined();
    expect(hotkeySlot('F0')).toBeUndefined();
  });
});

describe('hotkey storage', () => {
  it('saves and loads slots per character', () => {
    const s = memory();
    const slots = emptyHotkeys();
    slots[0] = { skillId: 5 };
    slots[8] = { skillId: 28, level: 3 };
    expect(saveHotkeys(() => s, 'Alice', slots)).toBe(true);
    expect([...s.data.keys()]).toEqual([hotkeyStorageKey('Alice')]);
    expect(loadHotkeys(() => s, 'Alice')).toEqual(slots);
    expect(loadHotkeys(() => s, 'Bob')).toEqual(emptyHotkeys());
  });

  it('survives storage that throws', () => {
    const blocked = (): HotkeyStorage => {
      throw new Error('SecurityError');
    };
    expect(loadHotkeys(blocked, 'Alice')).toEqual(emptyHotkeys());
    expect(saveHotkeys(blocked, 'Alice', emptyHotkeys())).toBe(false);
    const full: HotkeyStorage = {
      getItem: () => {
        throw new Error('no');
      },
      setItem: () => {
        throw new Error('QuotaExceededError');
      },
    };
    expect(loadHotkeys(() => full, 'Alice')).toHaveLength(HOTKEY_SLOTS);
    expect(saveHotkeys(() => full, 'Alice', emptyHotkeys())).toBe(false);
  });

  it('drops malformed slots', () => {
    expect(parseHotkeys(null)).toEqual(emptyHotkeys());
    expect(parseHotkeys('not json')).toEqual(emptyHotkeys());
    expect(parseHotkeys('{"a":1}')).toEqual(emptyHotkeys());
    const slots = parseHotkeys(
      JSON.stringify([{ skillId: 5, level: 0 }, { skillId: -1 }, 'x', null, { skillId: 1.5 }]),
    );
    expect(slots).toHaveLength(HOTKEY_SLOTS);
    expect(slots[0]).toEqual({ skillId: 5 });
    expect(slots.slice(1).every((h) => h === undefined)).toBe(true);
  });

  it('writes empty slots as null', () => {
    const slots = emptyHotkeys();
    slots[1] = { skillId: 7 };
    expect(JSON.parse(serializeHotkeys(slots))).toEqual([
      null,
      { skillId: 7 },
      null,
      null,
      null,
      null,
      null,
      null,
      null,
    ]);
  });
});

describe('dragged skills', () => {
  it('carries a skill and an optional level', () => {
    expect(parseDraggedSkill(draggedSkill({ skillId: 5 }))).toEqual({ skillId: 5 });
    expect(parseDraggedSkill(draggedSkill({ skillId: 28, level: 2 }))).toEqual({
      skillId: 28,
      level: 2,
    });
    expect(parseDraggedSkill('')).toBeUndefined();
    expect(parseDraggedSkill('0')).toBeUndefined();
    expect(parseDraggedSkill('5:x')).toBeUndefined();
    expect(parseDraggedSkill('5:0')).toEqual({ skillId: 5 });
  });
});
