import type { AssetStore } from '../assets/store';

// The player's status icons, such as Blessing or Poison, in a bar on the
// page. The server sends StatusIcon events for the player's own statuses.
// An icon that shows replaces the one with the same ID. The server sends
// the icons again after a map change, so the bar clears then.

/** The time left on a status, as whole minutes or seconds, rounded up. */
export function remainingText(ms: number): string {
  if (ms >= 60_000) {
    return `${Math.ceil(ms / 60_000)}m`;
  }
  return `${Math.max(0, Math.ceil(ms / 1000))}s`;
}

/** An icon to show, and its time left, or undefined for none. */
export interface ShownIcon {
  icon: number;
  left: string | undefined;
}

/** The icons that show, by EFST_ ID, in the order they came. */
export class StatusIconList {
  /** When each icon's status ends, as a time like performance.now(), or undefined. */
  private readonly ends = new Map<number, number | undefined>();

  /**
   * Shows or hides an icon. remainingMs is how long the status lasts, or
   * negative when it does not end by itself. now is the current time.
   */
  set(icon: number, on: boolean, remainingMs: number, now: number): void {
    if (!on) {
      this.ends.delete(icon);
      return;
    }
    this.ends.set(icon, remainingMs >= 0 ? now + remainingMs : undefined);
  }

  /** Hides every icon. */
  clear(): void {
    this.ends.clear();
  }

  /** The icons that show at a time, with their time left. */
  shown(now: number): ShownIcon[] {
    return [...this.ends].map(([icon, end]) => ({
      icon,
      left: end === undefined ? undefined : remainingText(end - now),
    }));
  }
}

/** Where icon images come from, such as the AssetStore. */
type IconSource = Pick<AssetStore, 'bytes'>;

interface Entry {
  li: HTMLLIElement;
  left: HTMLSpanElement;
}

/**
 * The status icon bar. Each icon shows its image, icon/status/<id>, or
 * its EFST_ number when the image is missing, and the time left on its
 * status. Call update every frame or so to count down.
 */
export class StatusIconBar {
  readonly list = new StatusIconList();
  private readonly root: HTMLOListElement;
  private readonly entries = new Map<number, Entry>();
  private readonly urls = new Map<number, Promise<string | undefined>>();

  constructor(
    parent: HTMLElement,
    private readonly store: IconSource,
  ) {
    this.root = document.createElement('ol');
    this.root.id = 'status-icons';
    this.root.setAttribute('aria-label', 'Statuses');
    parent.append(this.root);
  }

  /** Shows or hides an icon, as StatusIconList.set at the current time. */
  set(icon: number, on: boolean, remainingMs: number, now = performance.now()): void {
    this.list.set(icon, on, remainingMs, now);
    this.update(now);
  }

  /** Hides every icon. */
  clear(): void {
    this.list.clear();
    this.update(performance.now());
  }

  /** Shows the icons and their time left at a time. */
  update(now: number): void {
    const shown = this.list.shown(now);
    const ids = new Set(shown.map((s) => s.icon));
    for (const [icon, entry] of this.entries) {
      if (!ids.has(icon)) {
        entry.li.remove();
        this.entries.delete(icon);
      }
    }
    for (const s of shown) {
      let entry = this.entries.get(s.icon);
      if (entry === undefined) {
        entry = this.entry(s.icon);
        this.entries.set(s.icon, entry);
        this.root.append(entry.li);
      }
      const text = s.left ?? '';
      if (entry.left.textContent !== text) {
        entry.left.textContent = text;
      }
    }
  }

  private entry(icon: number): Entry {
    const li = document.createElement('li');
    li.className = 'status-icon';
    li.title = `Status ${icon}`;
    const img = document.createElement('img');
    img.alt = '';
    img.hidden = true;
    const badge = document.createElement('span');
    badge.className = 'badge';
    badge.textContent = String(icon);
    badge.hidden = true;
    const left = document.createElement('span');
    left.className = 'left';
    li.append(img, badge, left);
    this.image(icon)
      .then((src) => {
        if (src === undefined) {
          badge.hidden = false;
        } else {
          img.src = src;
          img.hidden = false;
        }
      })
      .catch((error: unknown) => {
        badge.hidden = false;
        console.error(`status icon ${icon}:`, error);
      });
    return { li, left };
  }

  /** Loads an icon's image, icon/status/<id>, once. Icons without one get undefined. */
  private async image(icon: number): Promise<string | undefined> {
    let url = this.urls.get(icon);
    if (url === undefined) {
      url = this.store
        .bytes(`icon/status/${icon}`, 'texture')
        .then((data) =>
          data === undefined
            ? undefined
            : URL.createObjectURL(new Blob([data], { type: 'image/webp' })),
        );
      this.urls.set(icon, url);
    }
    return await url;
  }
}
