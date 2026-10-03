import { describe, expect, it } from 'vitest';
import { checkFriendName, friendsFromList, setFriendOnline, sortFriends } from './friends';

const list = [
  { name: 'Alice', online: false },
  { name: 'Bob', online: true },
];

describe('friend list state', () => {
  it('copies FriendList', () => {
    const friends = friendsFromList(list);
    expect(friends).toEqual(list);
    expect(friends[0]).not.toBe(list[0]);
  });

  it('changes the online state of a friend', () => {
    expect(setFriendOnline(list, 'Alice', true)).toEqual([
      { name: 'Alice', online: true },
      { name: 'Bob', online: true },
    ]);
    expect(list[0]?.online).toBe(false);
  });

  it('changes nothing for the same state or an unknown name', () => {
    expect(setFriendOnline(list, 'Bob', true)).toBeUndefined();
    expect(setFriendOnline(list, 'Eve', true)).toBeUndefined();
  });

  it('shows online friends first', () => {
    expect(sortFriends(list).map((f) => f.name)).toEqual(['Bob', 'Alice']);
  });
});

describe('checkFriendName', () => {
  it('takes a new name', () => {
    expect(checkFriendName('  Eve ', list, 40)).toEqual({ name: 'Eve' });
  });

  it('refuses empty, known, own and too many names', () => {
    expect(checkFriendName('', list, 40)).toHaveProperty('error');
    expect(checkFriendName('Bob', list, 40)).toEqual({ error: 'Bob is your friend already.' });
    expect(checkFriendName('Me', list, 40, 'Me')).toEqual({ error: 'You cannot add yourself.' });
    expect(checkFriendName('Eve', list, 2)).toEqual({
      error: 'Your friend list is full. Remove a name first.',
    });
    expect(checkFriendName('x'.repeat(25), list, 40)).toHaveProperty('error');
  });
});
