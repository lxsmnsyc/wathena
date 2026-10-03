import { describe, expect, it } from 'vitest';
import {
  ALL_TAB,
  ChatTabs,
  LOCAL_LINES,
  MAX_WHISPER_TABS,
  channelKey,
  placeholder,
  showsLine,
  whisperKey,
} from './chatTabs';

describe('ChatTabs', () => {
  it('starts with All, which sends local chat', () => {
    const tabs = new ChatTabs();
    expect(tabs.tabs.map((t) => t.label)).toEqual(['All']);
    expect(tabs.active).toBe(ALL_TAB);
    expect(tabs.target()).toEqual({ kind: 'local' });
  });

  it('keeps channels before whispers', () => {
    const tabs = new ChatTabs();
    tabs.openWhisper('Bob');
    tabs.joinChannel('#global', '[Global]', 0xffffff, false);
    tabs.joinChannel('#trade', '', 0, true);
    expect(tabs.tabs.map((t) => t.key)).toEqual([
      ALL_TAB,
      channelKey('#global'),
      channelKey('#trade'),
      whisperKey('Bob'),
    ]);
    expect(tabs.channel('#TRADE')?.alias).toBe('[#trade]');
    expect(tabs.channel('#global')?.closable).toBe(false);
  });

  it('sends to the active tab', () => {
    const tabs = new ChatTabs();
    tabs.joinChannel('#trade', '[Trade]', 0, true);
    expect(tabs.activate(channelKey('#trade'))).toBe(true);
    expect(tabs.target()).toEqual({ kind: 'channel', name: '#trade' });
    tabs.activate(tabs.openWhisper('Bob'));
    expect(tabs.target()).toEqual({ kind: 'whisper', name: 'Bob' });
    expect(tabs.activate('whisper:Nobody')).toBe(false);
    expect(tabs.target()).toEqual({ kind: 'whisper', name: 'Bob' });
  });

  it('goes back to All when the active tab closes', () => {
    const tabs = new ChatTabs();
    tabs.joinChannel('#trade', '[Trade]', 0, true);
    tabs.activate(channelKey('#trade'));
    tabs.leaveChannel('#Trade');
    expect(tabs.active).toBe(ALL_TAB);
    tabs.close(ALL_TAB);
    expect(tabs.tabs).toHaveLength(1);
  });

  it('marks hidden tabs unread until they show', () => {
    const tabs = new ChatTabs();
    const bob = tabs.openWhisper('Bob');
    expect(tabs.markUnread(bob)).toBe(true);
    expect(tabs.markUnread(bob)).toBe(false);
    expect(tabs.get(bob)?.unread).toBe(true);
    tabs.activate(bob);
    expect(tabs.get(bob)?.unread).toBe(false);
    expect(tabs.markUnread(bob)).toBe(false);
    expect(tabs.markUnread(ALL_TAB)).toBe(false);
  });

  it('closes the oldest whisper tab past the limit, but not the active one', () => {
    const tabs = new ChatTabs();
    tabs.activate(tabs.openWhisper('P0'));
    for (let i = 1; i <= MAX_WHISPER_TABS; i++) {
      tabs.openWhisper(`P${String(i)}`);
    }
    const whispers = tabs.tabs.filter((t) => t.kind === 'whisper').map((t) => t.name);
    expect(whispers).toHaveLength(MAX_WHISPER_TABS);
    expect(whispers).toContain('P0');
    expect(whispers).not.toContain('P1');
  });
});

describe('showsLine', () => {
  it('shows everything on All, and a tab its own lines and notices', () => {
    expect(showsLine(ALL_TAB, LOCAL_LINES)).toBe(true);
    expect(showsLine(ALL_TAB, 'whisper:Bob')).toBe(true);
    expect(showsLine('whisper:Bob', 'whisper:Bob')).toBe(true);
    expect(showsLine('whisper:Bob', '')).toBe(true);
    expect(showsLine('whisper:Bob', LOCAL_LINES)).toBe(false);
    expect(showsLine('whisper:Bob', 'channel:#trade')).toBe(false);
  });
});

describe('placeholder', () => {
  it('says where the line goes', () => {
    expect(placeholder({ kind: 'local' })).toBe('Press Enter to chat');
    expect(placeholder({ kind: 'channel', name: '#trade' })).toBe('Say in #trade');
    expect(placeholder({ kind: 'whisper', name: 'Bob' })).toBe('Whisper to Bob');
  });
});
