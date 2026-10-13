import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { EventInfoSchema, EventResultSchema } from '../gen/wathena/admin/v1/admin_pb';
import {
  type TriggerFields,
  canonical,
  changedText,
  eventChange,
  fieldsOf,
  resultText,
  timeLeft,
  triggerValue,
} from './events';

const daily: TriggerFields = {
  kind: 'daily',
  time: '20:00',
  weekday: 'saturday',
  day: '1',
  minute: '0',
  chance: '100',
  module: '',
  fn: '',
  lasting: '0',
};

describe('triggers', () => {
  it('reads each kind of the config form', () => {
    expect(fieldsOf({ daily: '21:30' })).toMatchObject({ kind: 'daily', time: '21:30' });
    expect(fieldsOf({ weekly: { day: 'friday', time: '08:00' } })).toMatchObject({
      kind: 'weekly',
      weekday: 'friday',
      time: '08:00',
    });
    expect(fieldsOf({ monthly: { day: 15, time: '12:00' } })).toMatchObject({
      kind: 'monthly',
      day: '15',
    });
    expect(fieldsOf({ hourly: { minute: 5 } })).toMatchObject({ kind: 'hourly', minute: '5' });
    expect(fieldsOf({ random: { chance_per_hour: 250 } })).toMatchObject({
      kind: 'random',
      chance: '250',
    });
    expect(
      fieldsOf({ condition: { module: 'a.b', function: 'quiet', lasting_minutes: 10 } }),
    ).toMatchObject({ kind: 'condition', module: 'a.b', fn: 'quiet', lasting: '10' });
    expect(fieldsOf(undefined).kind).toBe('daily');
  });

  it('writes each kind back, and checks its fields', () => {
    expect(triggerValue(daily)).toEqual({ ok: true, value: { daily: '20:00' } });
    expect(triggerValue({ ...daily, kind: 'weekly', weekday: 'Saturday' })).toEqual({
      ok: true,
      value: { weekly: { day: 'saturday', time: '20:00' } },
    });
    expect(triggerValue({ ...daily, kind: 'monthly', day: '31' })).toEqual({
      ok: true,
      value: { monthly: { day: 31, time: '20:00' } },
    });
    expect(triggerValue({ ...daily, kind: 'hourly', minute: '59' })).toEqual({
      ok: true,
      value: { hourly: { minute: 59 } },
    });
    expect(triggerValue({ ...daily, kind: 'random', chance: '10000' })).toEqual({
      ok: true,
      value: { random: { chance_per_hour: 10000 } },
    });
    expect(
      triggerValue({ ...daily, kind: 'condition', module: 'a.b', fn: 'quiet', lasting: '5' }),
    ).toEqual({
      ok: true,
      value: { condition: { module: 'a.b', function: 'quiet', lasting_minutes: 5 } },
    });
    for (const bad of [
      { ...daily, time: '24:00' },
      { ...daily, time: '8:00' },
      { ...daily, kind: 'weekly' as const, weekday: 'someday' },
      { ...daily, kind: 'monthly' as const, day: '32' },
      { ...daily, kind: 'hourly' as const, minute: '60' },
      { ...daily, kind: 'random' as const, chance: '0' },
      { ...daily, kind: 'random' as const, chance: '1.5' },
      { ...daily, kind: 'condition' as const, module: '', fn: 'x' },
      { ...daily, kind: 'condition' as const, module: 'a', fn: 'x', lasting: '10081' },
    ]) {
      expect(triggerValue(bad).ok).toBe(false);
    }
  });

  it('compares triggers by content', () => {
    expect(canonical({ weekly: { time: '20:00', day: 'saturday' } })).toBe(
      canonical({ weekly: { day: 'saturday', time: '20:00' } }),
    );
  });
});

describe('eventChange', () => {
  const e = create(EventInfoSchema, {
    name: 'core:Field_Boss',
    on: false,
    trigger: { daily: '20:00' },
    durationMinutes: 60,
  });

  it('sends only what changed', () => {
    const r = eventChange(e, { on: true, trigger: daily, minutes: '60', reason: ' weekend ' });
    expect(r.ok && r.value.on).toBe(true);
    expect(r.ok && r.value.trigger).toBeUndefined();
    expect(r.ok && r.value.durationMinutes).toBe(0);
    expect(r.ok && r.value.reason).toBe('weekend');
    const t = eventChange(e, {
      on: false,
      trigger: { ...daily, time: '21:00' },
      minutes: '90',
      reason: '',
    });
    expect(t.ok && t.value.on).toBeUndefined();
    expect(t.ok && t.value.trigger).toEqual({ daily: '21:00' });
    expect(t.ok && t.value.durationMinutes).toBe(90);
  });

  it('refuses no change, a bad duration and a long reason', () => {
    expect(eventChange(e, { on: false, trigger: daily, minutes: '60', reason: '' }).ok).toBe(false);
    expect(eventChange(e, { on: true, trigger: daily, minutes: '0', reason: '' }).ok).toBe(false);
    expect(eventChange(e, { on: true, trigger: daily, minutes: '10081', reason: '' }).ok).toBe(
      false,
    );
    expect(
      eventChange(e, { on: true, trigger: daily, minutes: '60', reason: 'r'.repeat(256) }).ok,
    ).toBe(false);
  });
});

describe('event cells', () => {
  const now = new Date(2026, 9, 13, 20, 0, 0);

  it('writes the time left, the results and who changed it', () => {
    expect(timeLeft(timestampFromDate(new Date(now.getTime() + 90_000)), now)).toBe('1:30 left');
    expect(timeLeft(undefined, now)).toBe('');
    const r = create(EventResultSchema, { result: 'won', ended: timestampFromDate(now) });
    expect(resultText(r)).toMatch(/^won /);
    expect(changedText(create(EventInfoSchema, {}))).toBe('config');
    expect(
      changedText(
        create(EventInfoSchema, {
          override: { changedBy: 'Admin', changedAt: timestampFromDate(now) },
        }),
      ),
    ).toMatch(/^Admin, /);
  });
});
