import { describe, expect, it } from 'vitest';
import {
  BOOKING_JOBS,
  bookingJobName,
  bookingMap,
  checkAd,
  checkSearch,
  entryView,
  jobsLabel,
} from './booking';

describe('bookingMap', () => {
  it('adds map/ and keeps empty names empty', () => {
    expect(bookingMap('prt_fild08')).toBe('map/prt_fild08');
    expect(bookingMap(' Map/PRT_FILD08 ')).toBe('map/prt_fild08');
    expect(bookingMap('')).toBe('');
    expect(bookingMap('no spaces')).toBeUndefined();
  });
});

describe('checkAd', () => {
  const form = { level: '45', map: 'prt_fild08', jobs: [7, 8], note: ' Need a priest ' };

  it('gives the ad to send', () => {
    expect(checkAd(form)).toEqual({
      ad: { level: 45, map: 'map/prt_fild08', jobs: [7, 8], note: 'Need a priest' },
    });
    expect(checkAd({ ...form, map: '', jobs: [], note: '' })).toEqual({
      ad: { level: 45, map: '', jobs: [], note: '' },
    });
  });

  it('refuses bad levels', () => {
    for (const level of ['', '0', '1000', '4.5', 'abc', '-3']) {
      expect(checkAd({ ...form, level })).toHaveProperty('error');
    }
    expect(checkAd({ ...form, level: '999' })).toHaveProperty('ad');
  });

  it('refuses more than 6 jobs, counting each job once', () => {
    expect(checkAd({ ...form, jobs: [1, 2, 3, 4, 5, 6, 7] })).toEqual({
      error: 'Choose at most 6 jobs.',
    });
    expect(checkAd({ ...form, jobs: [1, 1, 2, 3, 4, 5, 6] })).toHaveProperty('ad');
  });

  it('refuses long notes and bad map names', () => {
    expect(checkAd({ ...form, note: 'x'.repeat(81) })).toHaveProperty('error');
    expect(checkAd({ ...form, note: 'x'.repeat(80) })).toHaveProperty('ad');
    expect(checkAd({ ...form, map: 'prt fild' })).toHaveProperty('error');
  });
});

describe('checkSearch', () => {
  it('searches by level and map or job', () => {
    expect(checkSearch({ level: '', map: '', job: 0 })).toEqual({
      search: { level: 0, map: '', job: 0 },
    });
    expect(checkSearch({ level: '50', map: 'gef_fild10', job: 0 })).toEqual({
      search: { level: 50, map: 'map/gef_fild10', job: 0 },
    });
    expect(checkSearch({ level: '50', map: '', job: 8 })).toEqual({
      search: { level: 50, map: '', job: 8 },
    });
  });

  it('refuses a map and a job together, and bad levels', () => {
    expect(checkSearch({ level: '', map: 'gef_fild10', job: 8 })).toEqual({
      error: 'Search by map or by job, not both.',
    });
    expect(checkSearch({ level: '1000', map: '', job: 0 })).toHaveProperty('error');
  });
});

describe('booking helpers', () => {
  it('names jobs', () => {
    expect(bookingJobName(8)).toBe('Priest');
    expect(bookingJobName(8, new Map([[8, 'Cleric']]))).toBe('Cleric');
    expect(jobsLabel([])).toBe('Any job');
    expect(jobsLabel([7, 8])).toBe('Knight, Priest');
    expect(BOOKING_JOBS).toContain(0);
    expect(BOOKING_JOBS).not.toContain(13);
  });

  it('reads PartyBookingEntry', () => {
    expect(
      entryView({
        name: 'Bob',
        ad: { level: 45, map: 'map/prt_fild08', jobs: [7], note: 'hi' },
        placedAtMs: 1000n,
      }),
    ).toEqual({
      name: 'Bob',
      level: 45,
      map: 'map/prt_fild08',
      jobs: [7],
      note: 'hi',
      placedAtMs: 1000,
    });
    expect(entryView({ name: 'Bob', placedAtMs: 0n }).jobs).toEqual([]);
  });
});
