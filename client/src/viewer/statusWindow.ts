import { BaseStat } from '../gen/wathena/v1/command_pb';
import { StatId } from '../gen/wathena/v1/event_pb';

// The status window, as RO's Alt+A window: the six stats with their raise
// cost and a "+" button, and the battle stats the server computes. The
// values follow the encodings in docs/proposals/0001-protocol.md.

/** A stat that status points raise, and the stats that carry it. */
interface Raised {
  name: string;
  stat: BaseStat;
  value: StatId;
  cost: StatId;
}

const RAISED: readonly Raised[] = [
  { name: 'STR', stat: BaseStat.STR, value: StatId.STR, cost: StatId.STR_COST },
  { name: 'AGI', stat: BaseStat.AGI, value: StatId.AGI, cost: StatId.AGI_COST },
  { name: 'VIT', stat: BaseStat.VIT, value: StatId.VIT, cost: StatId.VIT_COST },
  { name: 'INT', stat: BaseStat.INT, value: StatId.INT, cost: StatId.INT_COST },
  { name: 'DEX', stat: BaseStat.DEX, value: StatId.DEX, cost: StatId.DEX_COST },
  { name: 'LUK', stat: BaseStat.LUK, value: StatId.LUK, cost: StatId.LUK_COST },
];

/** One raised stat's row. */
export interface RaiseRow {
  name: string;
  stat: BaseStat;
  /** The total, with job bonuses. */
  value: string;
  /** The points the next raise costs. Empty at the stat's maximum. */
  cost: string;
  /** Whether the status points pay for the next raise. */
  canRaise: boolean;
}

/** Shows a stat the server has not sent yet as "-". */
function show(v: bigint | undefined): string {
  return v === undefined ? '-' : String(v);
}

/** The rows of STR to LUK. */
export function raiseRows(stats: ReadonlyMap<StatId, bigint>): RaiseRow[] {
  const points = stats.get(StatId.STATUS_POINTS);
  return RAISED.map((r) => {
    const cost = stats.get(r.cost);
    return {
      name: r.name,
      stat: r.stat,
      value: show(stats.get(r.value)),
      // A cost of 0 means the stat is at its maximum.
      cost: cost === undefined || cost === 0n ? '' : String(cost),
      canRaise: points !== undefined && cost !== undefined && cost > 0n && points >= cost,
    };
  });
}

/**
 * The battle stats, status points, and weight, as label and value pairs.
 * Weight is in tenths on the wire, and shows in whole units. The server
 * does not send the current weight yet, so it shows "-" until it does.
 */
export function detailRows(stats: ReadonlyMap<StatId, bigint>): [string, string][] {
  const weight = (id: StatId): string => {
    const v = stats.get(id);
    return v === undefined ? '-' : String(v / 10n);
  };
  return [
    ['ATK', show(stats.get(StatId.ATK))],
    ['MATK', show(stats.get(StatId.MATK))],
    ['DEF', show(stats.get(StatId.DEF))],
    ['MDEF', show(stats.get(StatId.MDEF))],
    ['HIT', show(stats.get(StatId.HIT))],
    ['FLEE', show(stats.get(StatId.FLEE))],
    ['CRIT', show(stats.get(StatId.CRITICAL))],
    ['ASPD', show(stats.get(StatId.ASPD))],
    ['Status Point', show(stats.get(StatId.STATUS_POINTS))],
    ['Weight', `${weight(StatId.WEIGHT)} / ${weight(StatId.MAX_WEIGHT)}`],
  ];
}

/** The status window on the page. */
export class StatusWindow {
  private readonly raise: { value: HTMLElement; cost: HTMLElement; button: HTMLButtonElement }[] =
    [];
  private readonly details: HTMLElement[] = [];

  constructor(
    private readonly root: HTMLElement,
    onRaise: (stat: BaseStat) => void,
  ) {
    const left = document.createElement('table');
    for (const r of RAISED) {
      const row = left.insertRow();
      const name = row.insertCell();
      name.textContent = r.name;
      const value = row.insertCell();
      value.className = 'value';
      const buttonCell = row.insertCell();
      const button = document.createElement('button');
      button.type = 'button';
      button.textContent = '+';
      button.title = `Raise ${r.name}`;
      button.disabled = true;
      button.addEventListener('click', () => {
        onRaise(r.stat);
      });
      buttonCell.append(button);
      const cost = row.insertCell();
      cost.className = 'cost';
      this.raise.push({ value, cost, button });
    }
    const right = document.createElement('table');
    for (const [label] of detailRows(new Map())) {
      const row = right.insertRow();
      row.insertCell().textContent = label;
      const value = row.insertCell();
      value.className = 'value';
      this.details.push(value);
    }
    root.append(left, right);
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  toggle(): void {
    this.root.hidden = !this.root.hidden;
  }

  /** Shows the stats. Only changed text is written. */
  update(stats: ReadonlyMap<StatId, bigint>): void {
    raiseRows(stats).forEach((r, i) => {
      const el = this.raise[i];
      if (el === undefined) {
        return;
      }
      set(el.value, r.value);
      set(el.cost, r.cost);
      if (el.button.disabled === r.canRaise) {
        el.button.disabled = !r.canRaise;
      }
    });
    detailRows(stats).forEach(([, v], i) => {
      const el = this.details[i];
      if (el !== undefined) {
        set(el, v);
      }
    });
  }
}

function set(el: HTMLElement, text: string): void {
  if (el.textContent !== text) {
    el.textContent = text;
  }
}
