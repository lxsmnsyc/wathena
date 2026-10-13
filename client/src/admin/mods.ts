// The mods screen (decision 0031): each loaded mod with its version, its
// signature status, its permissions granted and pending, and its Lua
// error counts by zone with the last error. With mods.disable, staff
// turn a mod off and on in one zone or every zone. Core cannot be
// turned off.

import type { ModInfo, ModZone } from '../gen/wathena/admin/v1/admin_pb';
import type { Context } from './context';
import { button, el, field, input, row, table, when, wideRow } from './dom';
import { REASON_MAX } from './restore';

/** What a signature status means, for people. */
export function signatureText(s: string): string {
  switch (s) {
    case 'builtin':
      return 'built in';
    case 'folder':
      return 'a folder, not signed';
    case 'unverified':
      return 'a .zip with a signature the server does not check';
    case 'unsigned':
      return 'a .zip without a signature';
    default:
      return s;
  }
}

/** Where a mod is off: "every zone", the maps, or "". */
export function offText(m: ModInfo): string {
  if (m.switches.some((s) => s.map === '')) {
    return 'every zone';
  }
  return m.switches.map((s) => s.map).join(', ');
}

/** A zone's errors for a table cell, such as "3, last at ...: text". */
export function zoneErrorText(z: ModZone): string {
  if (z.errors === 0n) {
    return z.off ? 'off' : '';
  }
  const last = z.lastError === '' ? '' : `: ${z.lastError}`;
  const off = z.off ? ' (off)' : '';
  return `${z.errors.toString()} errors${off}, last ${when(z.lastErrorAt)}${last}`;
}

/** Whether staff may turn a mod off or on. */
export function canSwitch(permissions: readonly string[], m: ModInfo): boolean {
  return permissions.includes('mods.disable') && !m.builtin;
}

/**
 * Checks a mod switch: turning a mod off needs a reason. It returns an
 * error, or "".
 */
export function switchError(off: boolean, reason: string): string {
  const r = reason.trim();
  if (off && r === '') {
    return 'Turning a mod off needs a reason.';
  }
  if (r.length > REASON_MAX) {
    return 'The reason is at most 255 characters.';
  }
  return '';
}

/** Renders the mods screen into a section and loads it. */
export function renderMods(ctx: Context, section: HTMLElement): void {
  section.replaceChildren(el('h2', 'Mods'));
  section.append(
    el(
      'p',
      'A mod that is off in a zone runs no event handlers, hooks, timers or NPC clicks there. Its data stays. It stays off across restarts until it is turned on.',
      'admin-who',
    ),
  );
  const { table: t, body } = table([
    'Mod',
    'Version',
    'Signature',
    'Permissions',
    'Pending',
    'Off in',
    'Errors by zone',
    '',
  ]);
  section.append(t);
  const load = async (): Promise<void> => {
    const r = await ctx.api.mods();
    if (!r.ok) {
      ctx.fail(r);
      return;
    }
    body.replaceChildren();
    for (const m of r.value.mods) {
      const errors = el('div');
      for (const z of m.zones) {
        const line = el('div', `${z.map}: ${zoneErrorText(z)}`);
        if (z.errors > 0n) {
          line.className = 'admin-error';
        }
        errors.append(line);
      }
      const tr = row([
        `${m.id} (${m.name})`,
        m.version,
        signatureText(m.signature),
        m.granted.join(', '),
        m.pending.join(', '),
        offText(m),
        errors,
        canSwitch(ctx.me.permissions, m) ? switchCell(ctx, m, r.value.maps, load) : '',
      ]);
      tr.dataset.mod = m.id;
      if (m.switches.length > 0) {
        tr.className = 'admin-off';
      }
      body.append(tr);
    }
    if (r.value.mods.length === 0) {
      body.append(wideRow('No mods are loaded.', 8));
    }
  };
  ctx.run(load());
}

/** The zone choice, the reason and the off and on buttons of a mod. */
function switchCell(
  ctx: Context,
  m: ModInfo,
  maps: readonly string[],
  switched: () => Promise<void>,
): HTMLElement {
  const cell = el('div', '', 'admin-review');
  const zone = el('select');
  zone.name = 'zone';
  const all = el('option', 'Every zone');
  all.value = '';
  zone.append(all);
  for (const name of maps) {
    const o = el('option', name);
    o.value = name;
    zone.append(o);
  }
  const reason = input('reason');
  reason.maxLength = REASON_MAX;
  reason.placeholder = 'Reason';
  const buttons: HTMLButtonElement[] = [];
  const flip = (off: boolean): void => {
    const err = switchError(off, reason.value);
    if (err !== '') {
      ctx.say(err);
      reason.focus();
      return;
    }
    for (const b of buttons) {
      b.disabled = true;
    }
    ctx.run(
      (async () => {
        const r = await ctx.api.switchMod(m.id, off, zone.value, reason.value);
        if (!r.ok) {
          ctx.fail(r);
          return;
        }
        const where = zone.value === '' ? 'every zone' : zone.value;
        ctx.say(`${m.id} is ${off ? 'off' : 'on'} in ${where}.`);
        await switched();
      })().finally(() => {
        for (const b of buttons) {
          b.disabled = false;
        }
      }),
    );
  };
  const offButton = button(
    'Turn off',
    () => {
      flip(true);
    },
    'admin-flag',
  );
  offButton.dataset.action = 'disable';
  const onButton = button('Turn on', () => {
    flip(false);
  });
  onButton.dataset.action = 'enable';
  buttons.push(offButton, onButton);
  cell.append(field('Zone', zone), reason, offButton, onButton);
  return cell;
}
