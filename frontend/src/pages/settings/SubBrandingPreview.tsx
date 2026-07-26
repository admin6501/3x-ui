import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, ConfigProvider, Progress, Space, Tag, theme as antdTheme } from 'antd';
import { AndroidOutlined, AppleOutlined, CopyOutlined, QrcodeOutlined } from '@ant-design/icons';

import {
  brandingCssVars,
  hasBrandingHeader,
  safeAssetUrl,
  safeLinkUrl,
  type SubBranding,
} from '@/lib/sub/branding';
import '@/pages/sub/SubPage.css';
import './SubBrandingPreview.css';

interface SubBrandingPreviewProps {
  branding: SubBranding;
  isDark: boolean;
}

// SubBrandingPreview is a stand-in for the real subscription page: same class
// names, same CSS variables, same block order, with sample data instead of a
// live client. It is a facsimile, not the page itself — the point is that an
// admin can judge colours, logo, texts and which blocks are visible without
// opening a real subscription link in another tab.
export default function SubBrandingPreview({ branding, isDark }: SubBrandingPreviewProps) {
  const { t } = useTranslation();

  const logo = safeAssetUrl(branding.logoUrl);
  const supportUrl = safeLinkUrl(branding.supportUrl);
  const telegramUrl = safeLinkUrl(branding.telegramUrl);
  const websiteUrl = safeLinkUrl(branding.websiteUrl);
  const hasLinks = !!(supportUrl || telegramUrl || websiteUrl);

  // The preview honours the branding's own theme choice; on 'auto' it follows
  // whatever theme the admin is using in the panel right now.
  const dark = branding.theme === 'auto' ? isDark : branding.theme === 'dark';

  // The preview lives inside the panel, which has its own antd theme. Without
  // its own provider the sample buttons would wear the panel's accent and light
  // /dark mode, and the preview would lie about what visitors see.
  const previewTheme = useMemo(() => ({
    algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    ...(branding.primaryColor ? { token: { colorPrimary: branding.primaryColor } } : {}),
  }), [dark, branding.primaryColor]);

  const classes = ['subscription-page', 'brand-preview'];
  if (dark) classes.push('is-dark');
  if (branding.bgImageUrl) classes.push('has-brand-bg');
  if (branding.cardOpacity < 100) classes.push('has-brand-glass');

  const title = hasBrandingHeader(branding) ? (
    <div className="brand-header">
      {logo && <img className="brand-logo" src={logo} alt="" />}
      <div className="brand-header-text">
        <span className="brand-name">{branding.brandName || t('subscription.title')}</span>
        {branding.tagline && <span className="brand-tagline">{branding.tagline}</span>}
      </div>
    </div>
  ) : (
    <Space>
      <span>{t('subscription.title')}</span>
      <Tag>demo-sub-id</Tag>
    </Space>
  );

  return (
    <ConfigProvider theme={previewTheme}>
    <div className={classes.join(' ')} style={brandingCssVars(branding)}>
      {branding.customCss && <style>{branding.customCss.replace(/<\/style/gi, '')}</style>}
      <Card className="subscription-card" title={title}>
        {branding.announcement && <div className="brand-announcement">{branding.announcement}</div>}

        {branding.showDetails && (
          <div className="brand-preview-table">
            <div><span>{t('subscription.subId')}</span><span>demo-sub-id</span></div>
            <div><span>{t('subscription.status')}</span><Tag color="green">{t('subscription.active')}</Tag></div>
            <div><span>{t('usage')}</span><span>12.4 GB / 50 GB</span></div>
          </div>
        )}

        {branding.showUsage && (
          <div className="brand-preview-usage">
            <Progress percent={25} showInfo={false} />
            <div className="brand-preview-usage-labels">
              <span>12.4 GB</span>
              <span>50 GB</span>
            </div>
          </div>
        )}

        {branding.showSubLinks && (
          <div className="links-section brand-preview-links">
            <div className="sub-link-row">
              <Tag color="green" className="sub-link-tag">SUB</Tag>
              <span className="sub-link-title">demo-sub-id</span>
              <div className="sub-link-actions">
                <Button size="small" icon={<CopyOutlined />} />
                <Button size="small" icon={<QrcodeOutlined />} />
              </div>
            </div>
          </div>
        )}

        {branding.showConfigLinks && (
          <div className="links-section brand-preview-links">
            <div className="sub-link-row">
              <Tag className="sub-link-tag">VLESS</Tag>
              <span className="sub-link-title">Germany-01</span>
              <div className="sub-link-actions">
                <Button size="small" icon={<CopyOutlined />} />
              </div>
            </div>
          </div>
        )}

        {branding.showApps && (
          <div className="brand-preview-apps">
            <Button type="primary" size="large"><AndroidOutlined /> Android</Button>
            <Button type="primary" size="large"><AppleOutlined /> iOS</Button>
          </div>
        )}

        {hasLinks && (
          <div className="brand-links">
            {supportUrl && (
              <Button type="primary" size="large">{branding.supportText || t('subscription.support')}</Button>
            )}
            {telegramUrl && <Button size="large">Telegram</Button>}
            {websiteUrl && <Button size="large">{t('subscription.website')}</Button>}
          </div>
        )}

        {branding.footerText && <div className="brand-footer">{branding.footerText}</div>}
      </Card>
    </div>
    </ConfigProvider>
  );
}
