// The player's own settings, kept in the browser. For now there is one:
// whether the touch joystick shows (review decision 39).

export interface Settings {
  /** Whether the joystick shows while touch is used. */
  joystick: boolean;
}

export const DEFAULT_SETTINGS: Settings = { joystick: true };

const KEY = 'wathena.settings';

/** The part of Storage the settings use. */
export type SettingsStorage = Pick<Storage, 'getItem' | 'setItem'>;

/** Reads the settings. Missing or broken values take the defaults. */
export function loadSettings(storage: () => SettingsStorage): Settings {
  let raw: string | null = null;
  try {
    raw = storage().getItem(KEY);
  } catch {
    // Storage can be off, such as in a private window.
  }
  if (raw === null) {
    return { ...DEFAULT_SETTINGS };
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
  const out = { ...DEFAULT_SETTINGS };
  if (typeof parsed === 'object' && parsed !== null && 'joystick' in parsed) {
    if (typeof parsed.joystick === 'boolean') {
      out.joystick = parsed.joystick;
    }
  }
  return out;
}

/** Saves the settings. A storage that refuses is ignored. */
export function saveSettings(storage: () => SettingsStorage, s: Settings): void {
  try {
    storage().setItem(KEY, JSON.stringify(s));
  } catch {
    // The setting lasts for this page only.
  }
}
