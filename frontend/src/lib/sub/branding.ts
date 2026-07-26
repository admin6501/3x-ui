// Subscription page branding: the visual configuration an admin builds in
// Settings → Subscription → Branding and the subscription page renders.
//
// The document is stored as a JSON string in the `subBranding` setting, so both
// sides go through normalizeSubBranding: the editor to load what was saved, the
// page to render it. Anything missing or of the wrong type falls back to the
// default, which reproduces the page's stock appearance — a half-written or
// hand-edited document can never leave a user with a broken page.

import type { CSSProperties } from 'react';

export type SubBrandingTheme = 'auto' | 'light' | 'dark';

export interface SubBranding {
  /** Replaces the card title. Empty keeps the translated default. */
  brandName: string;
  /** Logo shown next to the title. URL or data: URI. */
  logoUrl: string;
  /** One-line subtitle under the title. */
  tagline: string;
  /** Notice rendered above the usage summary. */
  announcement: string;

  /** Forces the page theme; 'auto' leaves the visitor's own choice alone. */
  theme: SubBrandingTheme;
  /** Accent colour: buttons, progress, links. Empty = antd default. */
  primaryColor: string;
  /** Page background colour behind the card. */
  pageBg: string;
  /** Card surface colour. */
  cardBg: string;
  /** Body text colour. */
  textColor: string;
  /** Full-page background image (URL or data: URI), drawn under the card. */
  bgImageUrl: string;
  /** Card corner radius in px. */
  cardRadius: number;
  /** Card opacity 0–100; below 100 the background image shows through. */
  cardOpacity: number;

  /** Block visibility. */
  showDetails: boolean;
  showUsage: boolean;
  showSubLinks: boolean;
  showConfigLinks: boolean;
  showApps: boolean;
  showThemeToggle: boolean;
  showLangToggle: boolean;

  /** Call-to-action button under the card. */
  supportText: string;
  supportUrl: string;
  telegramUrl: string;
  websiteUrl: string;
  footerText: string;

  /** Escape hatch for anything the controls don't cover. */
  customCss: string;
}

export const DEFAULT_SUB_BRANDING: SubBranding = {
  brandName: '',
  logoUrl: '',
  tagline: '',
  announcement: '',

  theme: 'auto',
  primaryColor: '',
  pageBg: '',
  cardBg: '',
  textColor: '',
  bgImageUrl: '',
  cardRadius: 12,
  cardOpacity: 100,

  showDetails: true,
  showUsage: true,
  showSubLinks: true,
  showConfigLinks: true,
  showApps: true,
  showThemeToggle: true,
  showLangToggle: true,

  supportText: '',
  supportUrl: '',
  telegramUrl: '',
  websiteUrl: '',
  footerText: '',

  customCss: '',
};

const THEMES: SubBrandingTheme[] = ['auto', 'light', 'dark'];

function str(raw: unknown, fallback: string): string {
  return typeof raw === 'string' ? raw : fallback;
}

function bool(raw: unknown, fallback: boolean): boolean {
  return typeof raw === 'boolean' ? raw : fallback;
}

function num(raw: unknown, fallback: number, min: number, max: number): number {
  const n = typeof raw === 'number' ? raw : Number(raw);
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, Math.round(n)));
}

// normalizeSubBranding accepts the stored document in any of the shapes it can
// arrive in — a parsed object, the raw JSON string, null — and returns a
// complete, in-range branding.
export function normalizeSubBranding(raw: unknown): SubBranding {
  let source: unknown = raw;
  if (typeof raw === 'string') {
    const trimmed = raw.trim();
    if (!trimmed) return { ...DEFAULT_SUB_BRANDING };
    try {
      source = JSON.parse(trimmed);
    } catch {
      return { ...DEFAULT_SUB_BRANDING };
    }
  }
  if (!source || typeof source !== 'object' || Array.isArray(source)) {
    return { ...DEFAULT_SUB_BRANDING };
  }
  const o = source as Record<string, unknown>;
  const d = DEFAULT_SUB_BRANDING;
  const theme = THEMES.includes(o.theme as SubBrandingTheme) ? (o.theme as SubBrandingTheme) : d.theme;

  return {
    brandName: str(o.brandName, d.brandName),
    logoUrl: str(o.logoUrl, d.logoUrl),
    tagline: str(o.tagline, d.tagline),
    announcement: str(o.announcement, d.announcement),

    theme,
    primaryColor: str(o.primaryColor, d.primaryColor),
    pageBg: str(o.pageBg, d.pageBg),
    cardBg: str(o.cardBg, d.cardBg),
    textColor: str(o.textColor, d.textColor),
    bgImageUrl: str(o.bgImageUrl, d.bgImageUrl),
    cardRadius: num(o.cardRadius, d.cardRadius, 0, 48),
    cardOpacity: num(o.cardOpacity, d.cardOpacity, 20, 100),

    showDetails: bool(o.showDetails, d.showDetails),
    showUsage: bool(o.showUsage, d.showUsage),
    showSubLinks: bool(o.showSubLinks, d.showSubLinks),
    showConfigLinks: bool(o.showConfigLinks, d.showConfigLinks),
    showApps: bool(o.showApps, d.showApps),
    showThemeToggle: bool(o.showThemeToggle, d.showThemeToggle),
    showLangToggle: bool(o.showLangToggle, d.showLangToggle),

    supportText: str(o.supportText, d.supportText),
    supportUrl: str(o.supportUrl, d.supportUrl),
    telegramUrl: str(o.telegramUrl, d.telegramUrl),
    websiteUrl: str(o.websiteUrl, d.websiteUrl),
    footerText: str(o.footerText, d.footerText),

    customCss: str(o.customCss, d.customCss),
  };
}

// Only URL schemes that are safe to drop into `url()` or an href. Anything else
// (javascript:, vbscript:, …) is dropped rather than rendered, so a branding
// document can never smuggle script into the subscription page.
const SAFE_URL = /^(https?:\/\/|data:image\/|\/|\.\/)/i;

export function safeAssetUrl(url: string): string {
  const trimmed = (url || '').trim();
  if (!trimmed || !SAFE_URL.test(trimmed)) return '';
  // Quotes and parentheses would break out of the CSS url() wrapper.
  if (/["'()\\]/.test(trimmed)) return '';
  return trimmed;
}

export function safeLinkUrl(url: string): string {
  const trimmed = (url || '').trim();
  if (!trimmed) return '';
  if (!/^(https?:\/\/|tg:\/\/|mailto:|\/)/i.test(trimmed)) return '';
  return trimmed;
}

// brandingCssVars maps the branding onto the custom properties the page's
// stylesheet reads. Empty values are omitted so the stock theme keeps control
// of anything the admin did not set.
export function brandingCssVars(branding: SubBranding): CSSProperties {
  const vars: Record<string, string> = {};
  if (branding.pageBg) vars['--bg-page'] = branding.pageBg;
  if (branding.cardBg) vars['--bg-card'] = branding.cardBg;
  if (branding.textColor) vars['--brand-text'] = branding.textColor;
  if (branding.primaryColor) vars['--brand-primary'] = branding.primaryColor;
  vars['--brand-card-radius'] = `${branding.cardRadius}px`;
  vars['--brand-card-opacity'] = String(branding.cardOpacity / 100);
  const bg = safeAssetUrl(branding.bgImageUrl);
  if (bg) vars['--brand-bg-image'] = `url(${bg})`;
  return vars as CSSProperties;
}

// hasBrandingHeader reports whether the header area has anything to show, so
// the page can skip rendering an empty block.
export function hasBrandingHeader(branding: SubBranding): boolean {
  return !!(branding.brandName || safeAssetUrl(branding.logoUrl) || branding.tagline);
}
