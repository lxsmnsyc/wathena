import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import {
  ChatLineSchema,
  ReportSummarySchema,
  SanctionSchema,
} from '../gen/wathena/admin/v1/admin_pb';
import {
  closeCheck,
  hasCharacter,
  lineSource,
  lineWhere,
  linkable,
  outcomeText,
  reportKindText,
  reportStatusText,
} from './reports';

describe('report cells', () => {
  it('names the kinds and outcomes', () => {
    expect(reportKindText('guild_emblem')).toBe('Guild emblem');
    expect(reportKindText('line')).toBe('Chat line');
    expect(reportKindText('new')).toBe('new');
    expect(outcomeText('no_action')).toBe('no action');
  });

  it('writes the status with who closed it', () => {
    expect(reportStatusText(create(ReportSummarySchema, { status: 'open' }))).toBe('open');
    expect(
      reportStatusText(
        create(ReportSummarySchema, { status: 'closed', closedBy: 'Gm', outcome: 'warned' }),
      ),
    ).toBe('closed by Gm: warned');
  });

  it('tells the reporter lines from the target lines', () => {
    const seen = create(ChatLineSchema, { kind: 'local', map: 'prontera', seen: true });
    expect(lineSource(seen)).toBe('reporter saw');
    expect(lineWhere(seen)).toBe('local on prontera');
    const own = create(ChatLineSchema, { kind: 'whisper', target: 'Ann' });
    expect(lineSource(own)).toBe('target log');
    expect(lineWhere(own)).toBe('whisper to Ann');
  });

  it('offers sanctions only on a character target', () => {
    expect(hasCharacter(create(ReportSummarySchema, { kind: 'line', targetId: 5n }))).toBe(true);
    expect(hasCharacter(create(ReportSummarySchema, { kind: 'player', targetId: 0n }))).toBe(false);
    expect(hasCharacter(create(ReportSummarySchema, { kind: 'guild_name', guildId: 3n }))).toBe(
      false,
    );
  });

  it('links only saved sanctions', () => {
    const list = [
      create(SanctionSchema, { id: 4n, kind: 'mute' }),
      create(SanctionSchema, { id: 0n, kind: 'mute' }),
    ];
    expect(linkable(list).map((s) => s.id)).toEqual([4n]);
  });
});

describe('closeCheck', () => {
  it('closes with no action or a warning without a sanction', () => {
    expect(closeCheck('no_action', ' ok ', 4n)).toEqual({
      ok: true,
      value: { outcome: 'no_action', note: 'ok', sanctionId: 0n },
    });
    expect(closeCheck('warned', '', 0n).ok).toBe(true);
  });

  it('needs a sanction for sanctioned, and a known outcome and a short note', () => {
    expect(closeCheck('sanctioned', '', 0n).ok).toBe(false);
    expect(closeCheck('sanctioned', '', 4n)).toEqual({
      ok: true,
      value: { outcome: 'sanctioned', note: '', sanctionId: 4n },
    });
    expect(closeCheck('', '', 0n).ok).toBe(false);
    expect(closeCheck('warned', 'x'.repeat(256), 0n).ok).toBe(false);
  });
});
