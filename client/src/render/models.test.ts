import { describe, expect, it } from 'vitest';
import { assetURIName } from './models';

describe('assetURIName', () => {
  it('decodes each segment', () => {
    expect(assetURIName('asset:texture/%EC%9B%8C%ED%84%B0/water000')).toBe('texture/워터/water000');
  });

  it('ignores other URIs', () => {
    expect(assetURIName('data:image/png;base64,')).toBeUndefined();
    expect(assetURIName('asset:texture/%E0%A4%A')).toBeUndefined();
    expect(assetURIName('asset:texture/a%2Fb')).toBeUndefined();
  });
});
