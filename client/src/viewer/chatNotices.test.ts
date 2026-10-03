import { describe, expect, it } from 'vitest';
import {
  ChannelFailure,
  ChatRefusal,
  ReportOutcome,
  SanctionKind,
  WhisperOutcome,
} from '../gen/wathena/v1/event_pb';
import {
  FILTERED,
  TOO_FAST,
  channelFailureMessage,
  channelListMessage,
  clockTime,
  membersMessage,
  refusalMessage,
  reportMessage,
  sanctionMessage,
  whisperMessage,
} from './chatNotices';

const at = (ms: number): string => `T${String(ms)}`;

describe('clockTime', () => {
  it('shows HH:MM today, and the date on other days', () => {
    const now = new Date(2026, 9, 3, 9, 0).getTime();
    expect(clockTime(new Date(2026, 9, 3, 14, 5).getTime(), now)).toBe('14:05');
    expect(clockTime(new Date(2026, 9, 4, 0, 30).getTime(), now)).toBe('2026-10-04 00:30');
  });
});

describe('sanctionMessage', () => {
  it('says until when and why', () => {
    expect(
      sanctionMessage({ kind: SanctionKind.MUTE, reason: 'spam', endsAtMs: 5, ended: false }, at),
    ).toBe('You are muted until T5 (reason: spam).');
    expect(
      sanctionMessage({ kind: SanctionKind.JAIL, reason: '', endsAtMs: 7, ended: false }, at),
    ).toBe('You are jailed until T7.');
    expect(
      sanctionMessage({ kind: SanctionKind.MUTE, reason: '', endsAtMs: 0, ended: false }, at),
    ).toBe('You are muted.');
  });

  it('says when it ended', () => {
    const ended = { reason: 'x', endsAtMs: 5, ended: true };
    expect(sanctionMessage({ kind: SanctionKind.MUTE, ...ended }, at)).toBe('Your mute ended.');
    expect(sanctionMessage({ kind: SanctionKind.JAIL, ...ended }, at)).toBe('Your jail ended.');
  });
});

describe('refusalMessage', () => {
  it('explains each refusal', () => {
    expect(refusalMessage(ChatRefusal.MUTED, '', undefined)).toBe('You are muted.');
    const mute = { kind: SanctionKind.MUTE, reason: 'spam', endsAtMs: 0, ended: false };
    expect(refusalMessage(ChatRefusal.MUTED, '', mute)).toBe('You are muted (reason: spam).');
    expect(refusalMessage(ChatRefusal.FILTERED, '', undefined)).toBe(FILTERED);
    expect(refusalMessage(ChatRefusal.NOT_IN_CHANNEL, '#trade', undefined)).toBe(
      'You are not in #trade.',
    );
    expect(refusalMessage(ChatRefusal.CANNOT_CHAT, '#news', undefined)).toBe(
      'You cannot talk in #news.',
    );
    expect(refusalMessage(ChatRefusal.TOO_FAST, '#trade', undefined)).toBe(TOO_FAST);
  });
});

describe('whisperMessage', () => {
  it('says nothing for a sent whisper', () => {
    expect(whisperMessage('Bob', WhisperOutcome.SENT)).toBeUndefined();
    expect(whisperMessage('Bob', WhisperOutcome.NOT_ONLINE)).toBe('Bob is not online.');
    expect(whisperMessage('Bob', WhisperOutcome.BLOCKED)).toBe('Bob blocks your whispers.');
  });
});

describe('channel notices', () => {
  it('explains failures', () => {
    expect(channelFailureMessage('#x', ChannelFailure.UNKNOWN)).toBe('There is no channel #x.');
    expect(channelFailureMessage('#global', ChannelFailure.CANNOT_LEAVE)).toBe(
      'You cannot leave #global.',
    );
  });

  it('lists channels and members', () => {
    expect(
      channelListMessage([
        { name: '#global', members: 12, joined: true },
        { name: '#trade', members: 3, joined: false },
      ]),
    ).toBe('Channels: #global (12, joined), #trade (3)');
    expect(channelListMessage([])).toBe('There are no channels to join.');
    expect(membersMessage('#trade', ['Alice', 'Bob'])).toBe('Members of #trade (2): Alice, Bob');
    expect(membersMessage('#trade', [])).toBe('Nobody is in #trade.');
  });
});

describe('reportMessage', () => {
  it('explains each outcome', () => {
    expect(reportMessage(ReportOutcome.SENT)).toBe('Your report was sent to the staff.');
    expect(reportMessage(ReportOutcome.LINE_NOT_FOUND)).toBe('That line is too old to report.');
  });
});
