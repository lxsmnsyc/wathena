import { describe, expect, it } from 'vitest';
import { DEFAULT_SETTINGS, type SettingsStorage, loadSettings, saveSettings } from './settings';

function memory(): SettingsStorage {
  const data = new Map<string, string>();
  return {
    getItem: (k) => data.get(k) ?? null,
    setItem: (k, v) => {
      data.set(k, v);
    },
  };
}

describe('settings', () => {
  it('starts with the joystick on', () => {
    expect(loadSettings(memory)).toEqual(DEFAULT_SETTINGS);
    expect(DEFAULT_SETTINGS.joystick).toBe(true);
  });

  it('saves and loads', () => {
    const s = memory();
    saveSettings(() => s, { joystick: false });
    expect(loadSettings(() => s)).toEqual({ joystick: false });
  });

  it('takes the defaults for broken values', () => {
    const s = memory();
    s.setItem('wathena.settings', '{nope');
    expect(loadSettings(() => s)).toEqual(DEFAULT_SETTINGS);
    s.setItem('wathena.settings', '{"joystick":"no"}');
    expect(loadSettings(() => s)).toEqual(DEFAULT_SETTINGS);
  });

  it('works without storage', () => {
    const broken = (): SettingsStorage => {
      throw new Error('denied');
    };
    expect(loadSettings(broken)).toEqual(DEFAULT_SETTINGS);
    expect(() => {
      saveSettings(broken, { joystick: false });
    }).not.toThrow();
  });
});
