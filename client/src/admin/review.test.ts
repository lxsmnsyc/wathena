import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { AuditEntrySchema, MeSchema } from '../gen/wathena/admin/v1/admin_pb';
import { canReview } from './review';

describe('canReview', () => {
  const me = create(MeSchema, { account: 'Owner', permissions: ['audit.read', 'audit.review'] });
  const row = create(AuditEntrySchema, {
    account: 'Gm',
    action: 'items.restore',
    needsReview: true,
  });

  it('offers a review of another account pending row', () => {
    expect(canReview(me, row)).toBe(true);
  });

  it('hides the buttons on the caller own rows, in any case', () => {
    expect(canReview(me, create(AuditEntrySchema, { ...row, account: 'owner' }))).toBe(false);
  });

  it('needs audit.review, a pending review and a row that needs one', () => {
    const reader = create(MeSchema, { account: 'Owner', permissions: ['audit.read'] });
    expect(canReview(reader, row)).toBe(false);
    expect(canReview(me, create(AuditEntrySchema, { ...row, needsReview: false }))).toBe(false);
    const done = create(AuditEntrySchema, {
      ...row,
      reviewedAt: timestampFromDate(new Date()),
      reviewedBy: 'Admin',
    });
    expect(canReview(me, done)).toBe(false);
  });
});
