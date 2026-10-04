import { ItemBound } from '../gen/wathena/v1/event_pb';
import type { Stack } from './inventory';

/** A slot with no equipment details, for tests. */
export default function plainStack(slot: number, itemId: number, amount: number): Stack {
  return {
    slot,
    itemId,
    amount,
    equip: 0,
    refine: 0,
    cards: [],
    location: 0,
    card: false,
    rolls: [],
    cardRolls: [],
    options: [],
    bound: ItemBound.UNSPECIFIED,
    restrictions: 0,
  };
}
