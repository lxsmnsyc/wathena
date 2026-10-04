// Guild emblems: a 24 x 24 PNG that the game server serves at
// /emblems/<guild_id>/<emblem_version>.png, on the host and port of its
// WebSocket. Version 0 means the guild has no emblem. Each emblem loads
// once per version. One that fails to load shows nothing.

/**
 * The URL of a guild's emblem, from the game server's WebSocket URL: ws
 * becomes http and wss becomes https. Returns undefined for version 0 or
 * a server URL that cannot be read.
 */
export function emblemURL(server: string, guildId: bigint, version: number): string | undefined {
  if (version === 0 || guildId === 0n) {
    return undefined;
  }
  let u: URL;
  try {
    u = new URL(server);
  } catch {
    return undefined;
  }
  let protocol: string;
  if (u.protocol === 'wss:' || u.protocol === 'https:') {
    protocol = 'https:';
  } else if (u.protocol === 'ws:' || u.protocol === 'http:') {
    protocol = 'http:';
  } else {
    return undefined;
  }
  return `${protocol}//${u.host}/emblems/${String(guildId)}/${String(version)}.png`;
}

/** Loads an image and says whether it loaded. */
export type ImageLoader = (url: string) => Promise<boolean>;

/**
 * Loads an image in the browser. crossOrigin is anonymous, so the image
 * could also be drawn into a canvas or a WebGL texture. The server sends
 * Access-Control-Allow-Origin for emblems.
 */
export const browserLoader: ImageLoader = async (url) =>
  new Promise((resolve) => {
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.decoding = 'async';
    img.addEventListener('load', () => {
      resolve(img.naturalWidth > 0);
    });
    img.addEventListener('error', () => {
      resolve(false);
    });
    img.src = url;
  });

type EmblemState = { url: string; status: 'loading' | 'ok' | 'failed' };

/** Guild emblems by guild ID and version. */
export class EmblemCache {
  private readonly emblems = new Map<string, EmblemState>();

  /** Called when an emblem has loaded, so the page can show it. */
  onLoad: (() => void) | undefined;

  constructor(
    /** The game server's WebSocket URL. */
    private readonly server: string,
    private readonly load: ImageLoader = browserLoader,
  ) {}

  /**
   * The URL of a guild's emblem once it has loaded. The first ask starts
   * the load. Version 0, a load in progress, and a failed load give
   * undefined.
   */
  get(guildId: bigint, version: number): string | undefined {
    const key = `${String(guildId)}/${String(version)}`;
    const known = this.emblems.get(key);
    if (known !== undefined) {
      return known.status === 'ok' ? known.url : undefined;
    }
    const url = emblemURL(this.server, guildId, version);
    if (url === undefined) {
      return undefined;
    }
    const state: EmblemState = { url, status: 'loading' };
    this.emblems.set(key, state);
    this.load(url)
      .then((ok) => {
        state.status = ok ? 'ok' : 'failed';
        if (ok) {
          this.onLoad?.();
        }
      })
      .catch(() => {
        state.status = 'failed';
      });
    return undefined;
  }
}
