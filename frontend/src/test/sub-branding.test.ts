import { describe, it, expect } from 'vitest';

import {
  DEFAULT_SUB_BRANDING,
  brandingCssVars,
  hasBrandingHeader,
  normalizeSubBranding,
  safeAssetUrl,
  safeLinkUrl,
} from '@/lib/sub/branding';

describe('normalizeSubBranding', () => {
  it('returns the stock defaults for anything unusable', () => {
    for (const input of [undefined, null, '', '   ', 'not json', 42, [], '[]', '"str"']) {
      expect(normalizeSubBranding(input)).toEqual(DEFAULT_SUB_BRANDING);
    }
  });

  it('parses the stored JSON string the settings row holds', () => {
    const parsed = normalizeSubBranding('{"brandName":"Acme VPN","cardRadius":20}');
    expect(parsed.brandName).toBe('Acme VPN');
    expect(parsed.cardRadius).toBe(20);
    // Untouched fields keep the default so a partial document still renders.
    expect(parsed.showApps).toBe(true);
    expect(parsed.theme).toBe('auto');
  });

  it('keeps every default a partially-written document leaves out', () => {
    const parsed = normalizeSubBranding({ tagline: 'fast and private' });
    expect(parsed.tagline).toBe('fast and private');
    expect(parsed).toEqual({ ...DEFAULT_SUB_BRANDING, tagline: 'fast and private' });
  });

  it('rejects values of the wrong type instead of passing them through', () => {
    const parsed = normalizeSubBranding({
      brandName: 123,
      showApps: 'yes',
      cardRadius: 'huge',
      theme: 'neon',
    });
    expect(parsed.brandName).toBe(DEFAULT_SUB_BRANDING.brandName);
    expect(parsed.showApps).toBe(DEFAULT_SUB_BRANDING.showApps);
    expect(parsed.cardRadius).toBe(DEFAULT_SUB_BRANDING.cardRadius);
    expect(parsed.theme).toBe('auto');
  });

  it('clamps numbers into the range the page can render', () => {
    expect(normalizeSubBranding({ cardRadius: 999 }).cardRadius).toBe(48);
    expect(normalizeSubBranding({ cardRadius: -5 }).cardRadius).toBe(0);
    // Opacity has a floor: a fully transparent card would be unreadable.
    expect(normalizeSubBranding({ cardOpacity: 0 }).cardOpacity).toBe(20);
    expect(normalizeSubBranding({ cardOpacity: 400 }).cardOpacity).toBe(100);
  });

  it('accepts the three known themes', () => {
    for (const theme of ['auto', 'light', 'dark'] as const) {
      expect(normalizeSubBranding({ theme }).theme).toBe(theme);
    }
  });
});

describe('url guards', () => {
  it('allows the schemes an image can safely come from', () => {
    expect(safeAssetUrl('https://cdn.example/logo.png')).toBe('https://cdn.example/logo.png');
    expect(safeAssetUrl('data:image/png;base64,AAAA')).toBe('data:image/png;base64,AAAA');
    expect(safeAssetUrl('/static/logo.svg')).toBe('/static/logo.svg');
  });

  it('drops anything that could execute or break out of url()', () => {
    expect(safeAssetUrl('javascript:alert(1)')).toBe('');
    expect(safeAssetUrl('data:text/html,<script>alert(1)</script>')).toBe('');
    expect(safeAssetUrl('https://x/a.png") ; background: url("evil')).toBe('');
    expect(safeAssetUrl("https://x/a'.png")).toBe('');
    expect(safeAssetUrl('')).toBe('');
  });

  it('allows only navigable schemes for the branded buttons', () => {
    expect(safeLinkUrl('https://t.me/support')).toBe('https://t.me/support');
    expect(safeLinkUrl('tg://resolve?domain=support')).toBe('tg://resolve?domain=support');
    expect(safeLinkUrl('mailto:help@example.com')).toBe('mailto:help@example.com');
    expect(safeLinkUrl('javascript:alert(1)')).toBe('');
    expect(safeLinkUrl('ftp://example.com')).toBe('');
  });
});

describe('brandingCssVars', () => {
  it('omits colours the admin did not set so the stock theme keeps control', () => {
    const vars = brandingCssVars(DEFAULT_SUB_BRANDING) as Record<string, string>;
    expect(vars['--bg-page']).toBeUndefined();
    expect(vars['--bg-card']).toBeUndefined();
    expect(vars['--brand-primary']).toBeUndefined();
    expect(vars['--brand-bg-image']).toBeUndefined();
    // Geometry always has a value — the page reads it with a fallback anyway.
    expect(vars['--brand-card-radius']).toBe('12px');
    expect(vars['--brand-card-opacity']).toBe('1');
  });

  it('maps the configured values onto the page variables', () => {
    const vars = brandingCssVars(normalizeSubBranding({
      pageBg: '#101820',
      cardBg: '#1b2430',
      textColor: '#f5f5f5',
      primaryColor: '#00b96b',
      cardRadius: 4,
      cardOpacity: 60,
      bgImageUrl: 'https://cdn.example/bg.jpg',
    })) as Record<string, string>;
    expect(vars['--bg-page']).toBe('#101820');
    expect(vars['--bg-card']).toBe('#1b2430');
    expect(vars['--brand-text']).toBe('#f5f5f5');
    expect(vars['--brand-primary']).toBe('#00b96b');
    expect(vars['--brand-card-radius']).toBe('4px');
    expect(vars['--brand-card-opacity']).toBe('0.6');
    expect(vars['--brand-bg-image']).toBe('url(https://cdn.example/bg.jpg)');
  });

  it('leaves out a background image that failed the url guard', () => {
    const vars = brandingCssVars(normalizeSubBranding({ bgImageUrl: 'javascript:alert(1)' })) as Record<string, string>;
    expect(vars['--brand-bg-image']).toBeUndefined();
  });
});

describe('hasBrandingHeader', () => {
  it('is false until something is worth showing in the header', () => {
    expect(hasBrandingHeader(DEFAULT_SUB_BRANDING)).toBe(false);
    expect(hasBrandingHeader(normalizeSubBranding({ brandName: 'Acme' }))).toBe(true);
    expect(hasBrandingHeader(normalizeSubBranding({ tagline: 'hi' }))).toBe(true);
    expect(hasBrandingHeader(normalizeSubBranding({ logoUrl: 'https://x/logo.png' }))).toBe(true);
  });

  it('does not count a logo url that was rejected as unsafe', () => {
    expect(hasBrandingHeader(normalizeSubBranding({ logoUrl: 'javascript:alert(1)' }))).toBe(false);
  });
});
