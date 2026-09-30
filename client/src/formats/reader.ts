// A little-endian reader for the binary asset formats. Every read checks
// the bytes left and throws FormatError when a file is truncated.

export class FormatError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'FormatError';
  }
}

const utf8 = new TextDecoder('utf-8', { fatal: true });

export class Reader {
  private readonly view: DataView;
  private p = 0;

  constructor(private readonly data: Uint8Array) {
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }

  done(): boolean {
    return this.p >= this.data.byteLength;
  }

  remaining(): number {
    return this.data.byteLength - this.p;
  }

  private need(n: number): number {
    if (n < 0 || n > this.remaining()) {
      throw new FormatError('file is truncated');
    }
    const at = this.p;
    this.p += n;
    return at;
  }

  take(n: number): Uint8Array {
    const at = this.need(n);
    return this.data.subarray(at, at + n);
  }

  u8(): number {
    return this.view.getUint8(this.need(1));
  }

  u16(): number {
    return this.view.getUint16(this.need(2), true);
  }

  i16(): number {
    return this.view.getInt16(this.need(2), true);
  }

  u32(): number {
    return this.view.getUint32(this.need(4), true);
  }

  i32(): number {
    return this.view.getInt32(this.need(4), true);
  }

  f32(): number {
    return this.view.getFloat32(this.need(4), true);
  }

  f32s(n: number): Float32Array {
    const out = new Float32Array(n);
    for (let i = 0; i < n; i++) {
      out[i] = this.f32();
    }
    return out;
  }

  /** A 4-byte ASCII tag. */
  tag(): string {
    return String.fromCharCode(...this.take(4));
  }

  /** A u8 length and UTF-8 bytes. */
  str(): string {
    try {
      return utf8.decode(this.take(this.u8()));
    } catch (error) {
      if (error instanceof FormatError) {
        throw error;
      }
      throw new FormatError('string is not UTF-8');
    }
  }

  /**
   * A u32 count, checked against the bytes left given the smallest size
   * of one item.
   */
  count(itemSize: number): number {
    const n = this.u32();
    if (n * itemSize > this.remaining()) {
      throw new FormatError(`count ${n} is too large`);
    }
    return n;
  }
}
