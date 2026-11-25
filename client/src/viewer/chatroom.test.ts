import { describe, expect, it } from 'vitest';
import { ChatRoomChange, ChatRoomOutcome } from '../gen/wathena/v1/event_pb';
import {
  RoomBook,
  type RoomInfo,
  type RoomMember,
  memberNotice,
  needsPassword,
  outcomeText,
  roomConditions,
  roomLabel,
  roomProblem,
} from './chatroom';

function info(over: Partial<RoomInfo>): RoomInfo {
  return {
    room: 1,
    owner: 5,
    title: 'Arena',
    users: 2,
    limit: 10,
    public: true,
    npc: true,
    minLevel: 1,
    maxLevel: 275,
    zeny: 0,
    ...over,
  };
}

function member(over: Partial<RoomMember>): RoomMember {
  return {
    room: 1,
    name: 'Bea',
    change: ChatRoomChange.JOIN,
    title: '',
    limit: 0,
    public: true,
    ...over,
  };
}

describe('room texts', () => {
  it('labels a room as its bubble does', () => {
    expect(roomLabel(info({}))).toBe('Arena 2/10');
    expect(roomLabel(info({ public: false, npc: false }))).toBe('Arena (locked) 2/10');
  });

  it('names the conditions of an NPC room', () => {
    expect(roomConditions(info({}))).toBe('');
    expect(roomConditions(info({ minLevel: 50, maxLevel: 69, zeny: 1000 }))).toBe(
      'level 50 to 69, 1000 z',
    );
  });

  it('asks for a password only for a locked player room', () => {
    expect(needsPassword(info({ public: false, npc: false }))).toBe(true);
    expect(needsPassword(info({ public: true, npc: false }))).toBe(false);
    expect(needsPassword(info({ public: false, npc: true }))).toBe(false);
  });

  it('says why a join was refused', () => {
    expect(outcomeText(ChatRoomOutcome.WRONG_PASSWORD)).toBe('Wrong password.');
    expect(outcomeText(ChatRoomOutcome.KICKED)).toContain('kicked');
    expect(outcomeText(ChatRoomOutcome.OK)).toBeUndefined();
  });

  it('checks a new room', () => {
    expect(roomProblem('Club', 'pw', '5')).toBeUndefined();
    expect(roomProblem('  ', '', '5')).toContain('title');
    expect(roomProblem('x'.repeat(37), '', '5')).toContain('title');
    expect(roomProblem('Club', '123456789', '5')).toContain('password');
    expect(roomProblem('Club', '', '0')).toContain('most users');
    expect(roomProblem('Club', '', '21')).toContain('most users');
  });

  it('words the member notices', () => {
    expect(memberNotice(member({}), 'Ann')).toBe('Bea joins the room.');
    expect(memberNotice(member({ name: 'Ann', change: ChatRoomChange.KICKED }), 'Ann')).toBe(
      'You were kicked from the room.',
    );
    expect(memberNotice(member({ change: ChatRoomChange.OWNER }), 'Ann')).toBe(
      'Bea is the owner of the room now.',
    );
  });
});

describe('RoomBook', () => {
  it('keeps the rooms near the player, newest first', () => {
    const b = new RoomBook();
    b.show(info({ room: 1 }));
    b.show(info({ room: 3 }));
    b.show(info({ room: 1, users: 5 }));
    expect(b.list().map((r) => [r.room, r.users])).toEqual([
      [3, 2],
      [1, 5],
    ]);
    b.clear(3);
    expect(b.list().map((r) => r.room)).toEqual([1]);
  });

  it('follows the members of the room the player is in', () => {
    const b = new RoomBook('Ann');
    b.joined({
      room: 1,
      members: ['Ann', 'Bea'],
      owner: 'Ann',
      title: 'Club',
      limit: 5,
      public: true,
    });
    expect(b.owns).toBe(true);
    b.member(member({ name: 'Cy' }));
    expect(b.current?.members).toEqual(['Ann', 'Bea', 'Cy']);
    b.member(member({ name: 'Cy', change: ChatRoomChange.OWNER }));
    expect(b.current?.members).toEqual(['Cy', 'Bea', 'Ann']);
    expect(b.owns).toBe(false);
    b.member(member({ name: 'Bea', change: ChatRoomChange.LEAVE }));
    expect(b.current?.members).toEqual(['Cy', 'Ann']);
    b.member(
      member({ name: 'x', change: ChatRoomChange.STATUS, title: 'New', limit: 3, public: false }),
    );
    expect(b.current).toMatchObject({ title: 'New', limit: 3, public: false });
  });

  it('ends the room when the player leaves or is kicked, and ignores other rooms', () => {
    const b = new RoomBook('Ann');
    b.joined({ room: 1, members: ['Ann'], owner: '', title: 'Arena', limit: 5, public: true });
    expect(b.member(member({ room: 9, name: 'Zed' }))).toBeUndefined();
    expect(b.current?.members).toEqual(['Ann']);
    expect(b.member(member({ name: 'Ann', change: ChatRoomChange.KICKED }))).toBe(
      'You were kicked from the room.',
    );
    expect(b.current).toBeUndefined();
  });
});
