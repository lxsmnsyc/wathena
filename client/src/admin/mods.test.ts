import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { ModInfoSchema, ModZoneSchema } from '../gen/wathena/admin/v1/admin_pb';
import { canSwitch, offText, signatureText, switchError, zoneErrorText } from './mods';

describe('mod cells', () => {
  it('says what a signature status means', () => {
    expect(signatureText('builtin')).toBe('built in');
    expect(signatureText('unverified')).toContain('does not check');
    expect(signatureText('other')).toBe('other');
  });

  it('says where a mod is off', () => {
    expect(offText(create(ModInfoSchema, {}))).toBe('');
    expect(
      offText(create(ModInfoSchema, { switches: [{ map: 'prontera' }, { map: 'geffen' }] })),
    ).toBe('prontera, geffen');
    expect(offText(create(ModInfoSchema, { switches: [{ map: 'prontera' }, { map: '' }] }))).toBe(
      'every zone',
    );
  });

  it('writes a zone errors with the last one', () => {
    const at = timestampFromDate(new Date(2026, 9, 13, 20, 0, 0));
    expect(zoneErrorText(create(ModZoneSchema, { map: 'prontera', off: true }))).toBe('off');
    const z = create(ModZoneSchema, { errors: 3n, lastError: 'timer: boom', lastErrorAt: at });
    expect(zoneErrorText(z)).toMatch(/^3 errors, last .*: timer: boom$/);
    expect(zoneErrorText(create(ModZoneSchema, { ...z, off: true }))).toMatch(/^3 errors \(off\)/);
  });
});

describe('switching', () => {
  it('needs mods.disable, and never offers core', () => {
    const m = create(ModInfoSchema, { id: 'example.a' });
    expect(canSwitch(['mods.read', 'mods.disable'], m)).toBe(true);
    expect(canSwitch(['mods.read'], m)).toBe(false);
    expect(canSwitch(['mods.disable'], create(ModInfoSchema, { id: 'core', builtin: true }))).toBe(
      false,
    );
  });

  it('needs a reason to turn a mod off', () => {
    expect(switchError(true, ' ')).not.toBe('');
    expect(switchError(true, 'broken quest')).toBe('');
    expect(switchError(false, '')).toBe('');
    expect(switchError(false, 'r'.repeat(256))).not.toBe('');
  });
});
