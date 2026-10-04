import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import {
  ItemBound,
  ItemRestriction,
  MailOutcome,
  MailSummarySchema,
} from '../gen/wathena/v1/event_pb';
import {
  type MailDraft,
  attachProblem,
  canDelete,
  mailButtonText,
  mailContentsText,
  mailDraftProblem,
  mailFee,
  mailFeeText,
  mailMessage,
  mailNewText,
  mailboxText,
  newestFirst,
  unreadCount,
} from './mail';
import plainStack from './testStacks';

const inventory = new Map([
  [1, plainStack(1, 909, 10)],
  [2, { ...plainStack(2, 2301, 1), equip: 16 }],
  [3, { ...plainStack(3, 501, 5), restrictions: ItemRestriction.NO_MAIL }],
  [4, { ...plainStack(4, 502, 5), bound: ItemBound.ACCOUNT }],
]);

const context = { maxItems: 2, zeny: 20000, inventory: (slot: number) => inventory.get(slot) };

const draft: MailDraft = { to: 'Bob', title: 'Hello', body: 'Hi Bob', zeny: 1000, items: [] };

describe('mailFee', () => {
  it("estimates rAthena's default fee", () => {
    expect(mailFee(0, 0)).toBe(0);
    expect(mailFee(1000, 0)).toBe(20);
    expect(mailFee(1050, 2)).toBe(5021);
    expect(mailFeeText(1000, 1)).toBe(
      "Fee about 2,520 z, total about 3,520 z (estimate, the server's rules may differ)",
    );
  });
});

describe('attachProblem', () => {
  it('refuses equipped, no_mail and bound items', () => {
    expect(attachProblem(inventory.get(1)!)).toBeUndefined();
    expect(attachProblem(inventory.get(2)!)).toBe('An equipped item cannot be mailed.');
    expect(attachProblem(inventory.get(3)!)).toBe('This item cannot be mailed.');
    expect(attachProblem(inventory.get(4)!)).toBe('A bound item cannot be mailed.');
  });
});

describe('mailDraftProblem', () => {
  it('lets a mail that fits go', () => {
    expect(mailDraftProblem(draft, context)).toBeUndefined();
    expect(
      mailDraftProblem({ ...draft, items: [{ slot: 1, amount: 10 }] }, context),
    ).toBeUndefined();
  });

  it('checks the recipient, title and text lengths', () => {
    expect(mailDraftProblem({ ...draft, to: ' ' }, context)).toBe("Enter the recipient's name.");
    expect(mailDraftProblem({ ...draft, title: '' }, context)).toBe('The title is empty.');
    expect(mailDraftProblem({ ...draft, title: 'a'.repeat(40) }, context)).toBe(
      'The title is too long: 40 of at most 39 bytes.',
    );
    expect(mailDraftProblem({ ...draft, body: 'a'.repeat(500) }, context)).toBe(
      'The text is too long: 500 of at most 499 bytes.',
    );
  });

  it('checks the attachments and the zeny', () => {
    const three = [
      { slot: 1, amount: 1 },
      { slot: 1, amount: 1 },
      { slot: 1, amount: 1 },
    ];
    expect(mailDraftProblem({ ...draft, items: three }, context)).toBe(
      'A mail holds at most 2 items.',
    );
    expect(mailDraftProblem({ ...draft, items: [{ slot: 1, amount: 11 }] }, context)).toBe(
      'You have 10 of that item.',
    );
    expect(mailDraftProblem({ ...draft, items: [{ slot: 9, amount: 1 }] }, context)).toBe(
      'An attachment is no longer in your inventory.',
    );
    expect(mailDraftProblem({ ...draft, items: [{ slot: 2, amount: 1 }] }, context)).toBe(
      'An equipped item cannot be mailed.',
    );
    // 19,700 zeny and a 394 fee is more than 20,000.
    expect(mailDraftProblem({ ...draft, zeny: 19700 }, context)).toBe(
      'You do not have enough zeny for the zeny and the fee.',
    );
  });
});

describe('mailMessage', () => {
  it('names every outcome, with the hook reason', () => {
    for (const o of Object.values(MailOutcome).filter((v) => typeof v === 'number')) {
      expect(mailMessage(o)).not.toBe('');
    }
    expect(mailMessage(MailOutcome.SENT)).toBe('The mail was sent.');
    expect(mailMessage(MailOutcome.HOOK, 'No mail on Sundays.')).toBe('No mail on Sundays.');
    expect(mailMessage(MailOutcome.HOOK)).toBe('The mail is not allowed.');
    expect(mailMessage(MailOutcome.NOT_EMPTY)).toBe(
      'Take the attachments before you delete the mail.',
    );
  });
});

describe('mailbox list', () => {
  const mail = (id: bigint, sentMs: bigint, read: boolean, zeny = 0, items = 0) =>
    create(MailSummarySchema, { id, sentMs, read, zeny, items, from: 'Bob', title: 'Hi' });

  it('sorts newest first and counts unread mails', () => {
    const mails = [mail(1n, 100n, true), mail(2n, 300n, false), mail(3n, 200n, false)];
    expect(newestFirst(mails).map((m) => m.id)).toEqual([2n, 3n, 1n]);
    expect(unreadCount(mails)).toBe(2);
    expect(mailButtonText(2)).toBe('Mail (2)');
    expect(mailButtonText(0)).toBe('Mail');
  });

  it('describes the mailbox and each mail', () => {
    expect(mailboxText(12, 30, 3, 10)).toBe('12 / 30 mails · Sent today 3 / 10');
    expect(mailboxText(0, 30, 1, 0)).toBe('0 / 30 mails · Sent today 1');
    expect(mailContentsText(mail(1n, 0n, true, 1000, 2))).toBe('1,000 z, 2 items');
    expect(mailContentsText(mail(1n, 0n, true, 0, 1))).toBe('1 item');
    expect(canDelete(mail(1n, 0n, true))).toBe(true);
    expect(canDelete(mail(1n, 0n, true, 5))).toBe(false);
    expect(mailNewText('Bob', 'Hi')).toBe('You have new mail from Bob: Hi');
  });
});
