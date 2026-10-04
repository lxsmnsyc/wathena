import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { WelcomeSchema } from '../gen/wathena/v1/session_pb';
import { DEFAULT_LIMITS, limitsOf, unpackSteps } from './connection';

describe('limitsOf', () => {
  it('takes the limits Welcome gives', () => {
    const welcome = create(WelcomeSchema, {
      limits: {
        partyMembers: 8,
        friends: 20,
        blockList: 30,
        guildMembers: 10,
        guildPositions: 8,
        guildAllies: 2,
        guildEnemies: 4,
        guildStorage: 600,
      },
    });
    expect(limitsOf(welcome)).toEqual({
      partyMembers: 8,
      friends: 20,
      blockList: 30,
      guildMembers: 10,
      guildPositions: 8,
      guildAllies: 2,
      guildEnemies: 4,
      guildStorage: 600,
    });
  });

  it("uses rAthena's defaults for missing limits", () => {
    expect(limitsOf(create(WelcomeSchema))).toEqual(DEFAULT_LIMITS);
    expect(limitsOf(create(WelcomeSchema, { limits: { friends: 10 } }))).toEqual({
      ...DEFAULT_LIMITS,
      friends: 10,
    });
  });
});

describe('unpackSteps', () => {
  it('reads facings low bits first and skips 0xF', () => {
    expect(unpackSteps(new Uint8Array([0x21, 0xf3]))).toEqual([1, 2, 3]);
  });
});
