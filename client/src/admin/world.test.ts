import { create } from '@bufbuild/protobuf';
import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import {
  RestartPlanSchema,
  RetentionKindSchema,
  RetentionStatusSchema,
  ZoneViewSchema,
} from '../gen/wathena/admin/v1/admin_pb';
import {
  announceError,
  bytesText,
  countdownText,
  localInput,
  msText,
  planText,
  restartRequest,
  retentionRow,
  retentionRunText,
  shownZones,
  uptimeText,
} from './world';

describe('dashboard cells', () => {
  it('writes tick times, memory and uptime', () => {
    expect(msText(1250n)).toBe('1.25');
    expect(msText(0n)).toBe('0.00');
    expect(bytesText(120n * 1024n * 1024n)).toBe('120 MB');
    expect(uptimeText(45n)).toBe('45 s');
    expect(uptimeText(125n * 60n)).toBe('2 h 5 min');
    expect(uptimeText(3n * 86_400n + 4n * 3600n)).toBe('3 d 4 h');
    expect(uptimeText(600n)).toBe('10 min');
  });

  it('counts down to a restart', () => {
    expect(countdownText(0)).toBe('now');
    expect(countdownText(-5)).toBe('now');
    expect(countdownText(30_000)).toBe('0:30');
    expect(countdownText(29_001)).toBe('0:30');
    expect(countdownText(12 * 60_000 + 5000)).toBe('12:05');
    expect(countdownText(3600_000 + 5 * 60_000 + 9000)).toBe('1:05:09');
  });

  it('shows awake zones first, most players first', () => {
    const zones = [
      create(ZoneViewSchema, { map: 'geffen', players: 2 }),
      create(ZoneViewSchema, { map: 'alberta', asleep: true }),
      create(ZoneViewSchema, { map: 'prontera', players: 9 }),
      create(ZoneViewSchema, { map: 'izlude', players: 2 }),
    ];
    expect(shownZones(zones, false).map((z) => z.map)).toEqual(['prontera', 'geffen', 'izlude']);
    expect(shownZones(zones, true).map((z) => z.map)).toEqual([
      'prontera',
      'geffen',
      'izlude',
      'alberta',
    ]);
  });
});

describe('announceError', () => {
  it('needs one line of at most 255 characters', () => {
    expect(announceError(' Market day! ')).toBe('');
    expect(announceError('  ')).not.toBe('');
    expect(announceError('a'.repeat(256))).not.toBe('');
    expect(announceError('two\nlines')).not.toBe('');
  });
});

describe('restartRequest', () => {
  const now = new Date(2026, 9, 13, 20, 0, 0);
  const at = (minutes: number): string => localInput(new Date(now.getTime() + minutes * 60_000));

  it('plans from 1 minute to 7 days ahead of the server clock', () => {
    const r = restartRequest({ at: at(30), reason: ' update ', message: ' New maps! ' }, now);
    expect(r.ok).toBe(true);
    if (r.ok) {
      expect(r.value.reason).toBe('update');
      expect(r.value.message).toBe('New maps!');
      expect(r.value.at === undefined ? 0 : timestampDate(r.value.at).getTime()).toBe(
        now.getTime() + 30 * 60_000,
      );
    }
    expect(restartRequest({ at: at(0), reason: 'x', message: '' }, now).ok).toBe(false);
    expect(restartRequest({ at: at(8 * 24 * 60), reason: 'x', message: '' }, now).ok).toBe(false);
    expect(restartRequest({ at: '', reason: 'x', message: '' }, now).ok).toBe(false);
    expect(restartRequest({ at: 'soon', reason: 'x', message: '' }, now).ok).toBe(false);
  });

  it('needs a reason, and keeps the texts short', () => {
    expect(restartRequest({ at: at(30), reason: '  ', message: '' }, now).ok).toBe(false);
    expect(restartRequest({ at: at(30), reason: 'r'.repeat(256), message: '' }, now).ok).toBe(
      false,
    );
    expect(restartRequest({ at: at(30), reason: 'x', message: 'm'.repeat(256) }, now).ok).toBe(
      false,
    );
  });

  it('says who planned it and whether logins are closed', () => {
    const p = create(RestartPlanSchema, {
      at: timestampFromDate(now),
      reason: 'update',
      plannedBy: 'Admin',
      loginsClosed: true,
    });
    expect(planText(p)).toContain('planned by Admin. Reason: update. New logins are refused.');
  });
});

describe('localInput', () => {
  it('writes a datetime-local value in the viewer time zone', () => {
    expect(localInput(new Date(2026, 0, 2, 3, 4, 59))).toBe('2026-01-02T03:04');
  });
});

describe('retention', () => {
  it('writes a kind as a row', () => {
    const k = create(RetentionKindSchema, {
      kind: 'chat',
      days: 45,
      defaultDays: 30,
      lastDeleted: 12n,
      deletedSinceStart: 40n,
    });
    expect(retentionRow(k)).toEqual(['Chat lines', '45 days', '30 days', '12', '40']);
    expect(
      retentionRow(create(RetentionKindSchema, { kind: 'new_kind', days: 1, defaultDays: 1 }))[0],
    ).toBe('new_kind');
  });

  it('writes the last run', () => {
    const at = timestampFromDate(new Date('2026-10-18T03:00:00Z'));
    expect(retentionRunText(create(RetentionStatusSchema, {}))).toBe(
      'The retention job has not run yet.',
    );
    expect(retentionRunText(create(RetentionStatusSchema, { skipped: true }))).toBe(
      'The retention job has not run yet. This server skipped its last run, since another process ran the job.',
    );
    // Another process's run, while this one skipped.
    const skipped = retentionRunText(create(RetentionStatusSchema, { lastRun: at, skipped: true }));
    expect(skipped).toContain('deleted old data.');
    expect(skipped).toContain('another process ran the job');
    expect(
      retentionRunText(create(RetentionStatusSchema, { lastRun: at, lastError: 'chat: timeout' })),
    ).toContain('failed: chat: timeout');
    const ok = retentionRunText(
      create(RetentionStatusSchema, { lastRun: at, partitionsDropped: 2, nextRun: at }),
    );
    expect(ok).toContain('deleted old data. It dropped 2 month partitions');
    expect(ok).toContain('The next run here is at');
  });
});
