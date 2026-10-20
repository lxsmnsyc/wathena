import type { AssetStore } from '../assets/store';

// Plays sounds with Web Audio and music with an audio element. Browsers
// start audio only after a user gesture, so nothing plays until resume.

/** Something that plays sounds by asset name. */
export interface SoundPlayer {
  /**
   * Plays a sound once, or again and again with loop until stopLoops.
   * volume is 0 to 1, and pan -1 (left) to 1 (right).
   */
  play(name: string, volume?: number, pan?: number, loop?: boolean): void;
}

export class AudioSystem implements SoundPlayer {
  readonly context = new AudioContext();
  /** Sounds started, and sounds decoded, since the system started. */
  readonly stats = { played: 0, decoded: 0 };
  private readonly sounds: GainNode;
  private readonly buffers = new Map<string, Promise<AudioBuffer | undefined>>();
  private music: { name: string; element: HTMLAudioElement; url: string } | undefined;
  private readonly musicVolume = 0.5;
  private wantedMusic = '';
  /** The sounds that play again and again, until stopLoops. */
  private readonly loops = new Set<AudioBufferSourceNode>();
  /** Bumped by stopLoops, so a loop still loading does not start. */
  private loopGeneration = 0;
  /** Whether the user turned sound on. */
  private enabled = false;

  constructor(private readonly store: AssetStore) {
    this.sounds = this.context.createGain();
    this.sounds.connect(this.context.destination);
    // Some browsers start a context running. Sound stays off until the
    // user turns it on either way.
    this.context.suspend().catch(() => undefined);
  }

  get running(): boolean {
    return this.enabled && this.context.state === 'running';
  }

  /** Whether sound is on. */
  get on(): boolean {
    return this.enabled;
  }

  /** Starts audio. Call it from a user gesture, such as a click. */
  async resume(): Promise<void> {
    this.enabled = true;
    await this.context.resume();
    await this.playMusic(this.wantedMusic);
  }

  /** Stops audio until resume. */
  async suspend(): Promise<void> {
    this.enabled = false;
    this.music?.element.pause();
    await this.context.suspend();
  }

  play(name: string, volume = 1, pan = 0, loop = false): void {
    if (!this.running || volume <= 0) {
      return;
    }
    const generation = this.loopGeneration;
    this.buffer(name)
      .then((buffer) => {
        if (buffer === undefined || (loop && generation !== this.loopGeneration)) {
          return;
        }
        const source = this.context.createBufferSource();
        source.buffer = buffer;
        if (loop) {
          // soundeffect's repeat: the sound plays until the map changes.
          source.loop = true;
          this.loops.add(source);
          source.onended = () => this.loops.delete(source);
        }
        const gain = this.context.createGain();
        gain.gain.value = Math.min(volume, 1);
        const panner = this.context.createStereoPanner();
        panner.pan.value = Math.max(-1, Math.min(1, pan));
        source.connect(panner).connect(gain).connect(this.sounds);
        source.start();
        this.stats.played++;
      })
      .catch((error: unknown) => {
        console.error(`sound ${name}:`, error);
      });
  }

  /** Stops the sounds that play again and again. */
  stopLoops(): void {
    this.loopGeneration++;
    for (const source of this.loops) {
      source.stop();
    }
    this.loops.clear();
  }

  /**
   * Plays music on a loop, replacing the current track. An empty name
   * stops music. The track starts when audio is running.
   */
  async playMusic(name: string): Promise<void> {
    this.wantedMusic = name;
    if (this.music?.name === name) {
      if (this.running) {
        await this.music.element.play();
      }
      return;
    }
    this.stopMusic();
    if (name === '' || !this.running) {
      return;
    }
    const data = await this.store.bytes(name, 'music');
    if (data === undefined || this.wantedMusic !== name) {
      return;
    }
    // The bytes are checked against their hash, then played from memory.
    const url = URL.createObjectURL(new Blob([data], { type: 'audio/mpeg' }));
    const element = new Audio(url);
    element.loop = true;
    element.volume = this.musicVolume;
    this.music = { name, element, url };
    await element.play();
  }

  private stopMusic(): void {
    if (this.music !== undefined) {
      this.music.element.pause();
      URL.revokeObjectURL(this.music.url);
      this.music = undefined;
    }
  }

  private async buffer(name: string): Promise<AudioBuffer | undefined> {
    let b = this.buffers.get(name);
    if (b === undefined) {
      b = this.decode(name);
      this.buffers.set(name, b);
    }
    return b;
  }

  private async decode(name: string): Promise<AudioBuffer | undefined> {
    const data = await this.store.bytes(name, 'sound');
    if (data === undefined) {
      return undefined;
    }
    // decodeAudioData takes over its buffer, so it gets a copy.
    const buffer = await this.context.decodeAudioData(data.slice().buffer);
    this.stats.decoded++;
    return buffer;
  }
}
