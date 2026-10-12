import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { AuditEntrySchema } from '../gen/wathena/admin/v1/admin_pb';
import { argumentsText, canGive, reviewText } from './app';

describe('audit cells', () => {
  it('writes arguments sorted by key', () => {
    const e = create(AuditEntrySchema, { arguments: { role: 'admin', previous: 'moderator' } });
    expect(argumentsText(e)).toBe('previous="moderator" role="admin"');
  });

  it('says whether a row needs or had a review', () => {
    expect(reviewText(create(AuditEntrySchema, {}))).toBe('');
    expect(reviewText(create(AuditEntrySchema, { needsReview: true }))).toBe('Needs review');
    expect(
      reviewText(
        create(AuditEntrySchema, {
          needsReview: true,
          reviewedBy: 'Owner',
          reviewedAt: timestampFromDate(new Date()),
        }),
      ),
    ).toBe('Reviewed by Owner');
  });
});

describe('canGive', () => {
  it('offers only roles the caller holds every permission of', () => {
    expect(canGive(['audit.read', 'staff.assign'], ['audit.read'])).toBe(true);
    expect(canGive(['audit.read'], ['audit.read', 'moderation.ban'])).toBe(false);
  });
});
