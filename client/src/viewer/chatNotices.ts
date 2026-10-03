// The text of chat notices: whisper answers, refused lines, channel
// answers, sanctions and reports. Lines follow rAthena's messages where
// rAthena has one.

import {
  ChannelFailure,
  type ChannelInfo,
  ChatRefusal,
  ReportOutcome,
  SanctionKind,
  WhisperOutcome,
} from '../gen/wathena/v1/event_pb';

/** The notice for a filtered line. */
export const FILTERED = 'Your message was not sent: it has a word the server does not allow.';

/** rAthena's msg 1455, for a channel line sent before the channel's delay. */
export const TOO_FAST = "You're talking too fast!";

/** A sanction as SanctionNotice gives it. */
export interface Sanction {
  kind: SanctionKind;
  reason: string;
  /** When it ends, in Unix milliseconds, or 0 when it does not end by itself. */
  endsAtMs: number;
  ended: boolean;
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/**
 * A time in local time as HH:MM, with the date as YYYY-MM-DD before it
 * when it is not on the same day as now.
 */
export function clockTime(ms: number, now = Date.now()): string {
  const d = new Date(ms);
  const today = new Date(now);
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  if (
    d.getFullYear() === today.getFullYear() &&
    d.getMonth() === today.getMonth() &&
    d.getDate() === today.getDate()
  ) {
    return time;
  }
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${time}`;
}

/**
 * The notice for a mute or a jail, such as "You are muted until 14:05
 * (reason: spam)." or "Your mute ended.". at formats the end time.
 */
export function sanctionMessage(s: Sanction, at: (ms: number) => string = clockTime): string {
  const jail = s.kind === SanctionKind.JAIL;
  if (s.ended) {
    return jail ? 'Your jail ended.' : 'Your mute ended.';
  }
  let text = jail ? 'You are jailed' : 'You are muted';
  if (s.endsAtMs > 0) {
    text += ` until ${at(s.endsAtMs)}`;
  }
  if (s.reason !== '') {
    text += ` (reason: ${s.reason})`;
  }
  return `${text}.`;
}

/**
 * The notice for a refused line. to is the whisper's target or the
 * channel's name. mute is the player's mute, when the client knows it.
 */
export function refusalMessage(
  reason: ChatRefusal,
  to: string,
  mute: Sanction | undefined,
): string {
  switch (reason) {
    case ChatRefusal.MUTED:
      return mute === undefined ? 'You are muted.' : sanctionMessage(mute);
    case ChatRefusal.FILTERED:
      return FILTERED;
    case ChatRefusal.NOT_IN_CHANNEL:
      return to === '' ? 'You are not in that channel.' : `You are not in ${to}.`;
    case ChatRefusal.CANNOT_CHAT:
      return to === '' ? 'You cannot talk in that channel.' : `You cannot talk in ${to}.`;
    case ChatRefusal.TOO_FAST:
      return TOO_FAST;
    default:
      return 'Your message was not sent.';
  }
}

/** The notice for a whisper the server did not deliver, or undefined when it did. */
export function whisperMessage(to: string, outcome: WhisperOutcome): string | undefined {
  switch (outcome) {
    case WhisperOutcome.SENT:
      return undefined;
    case WhisperOutcome.NOT_ONLINE:
      return `${to} is not online.`;
    case WhisperOutcome.BLOCKED:
      return `${to} blocks your whispers.`;
    default:
      return `Your whisper to ${to} was not sent.`;
  }
}

/** The notice for a channel command that did nothing. */
export function channelFailureMessage(name: string, reason: ChannelFailure): string {
  switch (reason) {
    case ChannelFailure.UNKNOWN:
      return `There is no channel ${name}.`;
    case ChannelFailure.ALREADY_JOINED:
      return `You are already in ${name}.`;
    case ChannelFailure.INVITE_ONLY:
      return `You cannot join ${name} by yourself.`;
    case ChannelFailure.NOT_JOINED:
      return `You are not in ${name}.`;
    case ChannelFailure.CANNOT_LEAVE:
      return `You cannot leave ${name}.`;
    case ChannelFailure.TOO_MANY:
      return 'You are in too many channels. Leave one first.';
    default:
      return `The channel command for ${name} did nothing.`;
  }
}

/** The notice for the server's answer to a report. */
export function reportMessage(outcome: ReportOutcome): string {
  switch (outcome) {
    case ReportOutcome.SENT:
      return 'Your report was sent to the staff.';
    case ReportOutcome.LINE_NOT_FOUND:
      return 'That line is too old to report.';
    case ReportOutcome.TOO_MANY:
      return 'You sent too many reports. Try again later.';
    case ReportOutcome.BAD_NAME:
      return 'That is not a valid character name.';
    default:
      return 'Your report was not sent.';
  }
}

/** The notice for the channel list, such as "Channels: #global (12, joined), #trade (3)". */
export function channelListMessage(
  channels: Pick<ChannelInfo, 'name' | 'members' | 'joined'>[],
): string {
  if (channels.length === 0) {
    return 'There are no channels to join.';
  }
  const items = channels.map(
    (c) => `${c.name} (${String(c.members)}${c.joined ? ', joined' : ''})`,
  );
  return `Channels: ${items.join(', ')}`;
}

/** The notice for the members of a channel. */
export function membersMessage(name: string, names: string[]): string {
  if (names.length === 0) {
    return `Nobody is in ${name}.`;
  }
  return `Members of ${name} (${String(names.length)}): ${names.join(', ')}`;
}
