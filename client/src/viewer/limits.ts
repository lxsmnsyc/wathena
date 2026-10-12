import { StorageKind } from '../gen/wathena/v1/event_pb';

/** A window whose size a player's limit sets. */
interface Sized {
  size: number;
}

/** A storage window: its kind and record name, and its size. */
interface SizedStorage extends Sized {
  kind: StorageKind;
  name: string;
}

/**
 * The player's values of the limits it holds, as PlayerLimits (proposal
 * 0002, review decision 32), by name, such as inventory_slots.
 */
export function playerLimitsOf(
  limits: readonly { name: string; value: number }[],
): Map<string, number> {
  const out = new Map<string, number>();
  for (const l of limits) {
    if (l.name !== '') {
      out.set(l.name, l.value);
    }
  }
  return out;
}

/**
 * Applies the player's limits to the windows they size: inventory_slots
 * to the inventory, storage_slots to the account's storage while it is
 * open, and cart_slots to the cart. An extra storage keeps its own size.
 * It returns whether a storage or cart size changed, so their windows
 * draw again.
 */
export function applyPlayerLimits(
  limits: ReadonlyMap<string, number>,
  inventory: Sized,
  storage: SizedStorage,
  cart: Sized,
): boolean {
  const inventorySlots = limits.get('inventory_slots');
  if (inventorySlots !== undefined) {
    inventory.size = inventorySlots;
  }
  let changed = false;
  const storageSlots = limits.get('storage_slots');
  if (
    storageSlots !== undefined &&
    storage.kind === StorageKind.PERSONAL &&
    storage.name === '' &&
    storage.size !== storageSlots
  ) {
    storage.size = storageSlots;
    changed = true;
  }
  const cartSlots = limits.get('cart_slots');
  if (cartSlots !== undefined && cart.size !== 0 && cart.size !== cartSlots) {
    cart.size = cartSlots;
    changed = true;
  }
  return changed;
}
