import { useCallback, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Col,
  ColorPicker,
  Input,
  InputNumber,
  Row,
  Segmented,
  Slider,
  Switch,
  Upload,
  message,
} from 'antd';
import { DeleteOutlined, UploadOutlined } from '@ant-design/icons';
import type { UploadFile } from 'antd';

import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { useTheme } from '@/hooks/useTheme';
import {
  DEFAULT_SUB_BRANDING,
  normalizeSubBranding,
  type SubBranding,
  type SubBrandingTheme,
} from '@/lib/sub/branding';
import SubBrandingPreview from './SubBrandingPreview';

// Images are stored inside the branding document as data URIs, which keeps the
// panel free of an upload directory to serve and back up. The cap is what stops
// someone pasting a 5 MB photo into a settings row.
const MAX_IMAGE_BYTES = 512 * 1024;

interface SubscriptionBrandingTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

export default function SubscriptionBrandingTab({ allSetting, updateSetting }: SubscriptionBrandingTabProps) {
  const { t } = useTranslation();
  const { isDark } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();

  // The stored document is the source of truth; the editor never keeps its own
  // copy, so an external change (import, another tab) is picked up on re-render
  // instead of being silently overwritten.
  const branding = useMemo(() => normalizeSubBranding(allSetting.subBranding), [allSetting.subBranding]);
  const brandingRef = useRef(branding);
  brandingRef.current = branding;

  const patch = useCallback((next: Partial<SubBranding>) => {
    updateSetting({ subBranding: JSON.stringify({ ...brandingRef.current, ...next }) });
  }, [updateSetting]);

  const readImage = useCallback(async (file: UploadFile & Blob, field: 'logoUrl' | 'bgImageUrl') => {
    if (file.size && file.size > MAX_IMAGE_BYTES) {
      messageApi.error(t('pages.settings.branding.imageTooLarge', { kb: Math.round(MAX_IMAGE_BYTES / 1024) }));
      return;
    }
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ''));
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(file);
    }).catch(() => '');
    if (!dataUrl.startsWith('data:image/')) {
      messageApi.error(t('pages.settings.branding.imageInvalid'));
      return;
    }
    patch({ [field]: dataUrl } as Partial<SubBranding>);
  }, [messageApi, patch, t]);

  const resetAll = useCallback(() => {
    updateSetting({ subBranding: JSON.stringify(DEFAULT_SUB_BRANDING) });
  }, [updateSetting]);

  const colorValue = (value: string) => value || undefined;

  return (
    <>
      {messageContextHolder}
      <Row gutter={24}>
        <Col xs={24} lg={13}>
          <SettingListItem paddings="small" title={t('pages.settings.branding.enable')} description={t('pages.settings.branding.enableDesc')}>
            <Switch checked={allSetting.subBrandingEnable} onChange={(v) => updateSetting({ subBrandingEnable: v })} />
          </SettingListItem>

          {!allSetting.subBrandingEnable && (
            <Alert
              type="info"
              showIcon
              style={{ margin: '8px 0 16px' }}
              message={t('pages.settings.branding.disabledNotice')}
            />
          )}

          <SettingListItem paddings="small" title={t('pages.settings.branding.brandName')} description={t('pages.settings.branding.brandNameDesc')}>
            <Input value={branding.brandName} maxLength={80} onChange={(e) => patch({ brandName: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.tagline')}>
            <Input value={branding.tagline} maxLength={120} onChange={(e) => patch({ tagline: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.logo')} description={t('pages.settings.branding.logoDesc', { kb: Math.round(MAX_IMAGE_BYTES / 1024) })}>
            <div style={{ display: 'flex', gap: 8, width: '100%' }}>
              <Input
                value={branding.logoUrl}
                placeholder="https://… / data:image/…"
                onChange={(e) => patch({ logoUrl: e.target.value })}
              />
              <Upload
                accept="image/*"
                showUploadList={false}
                beforeUpload={(file) => { void readImage(file as UploadFile & Blob, 'logoUrl'); return false; }}
              >
                <Button icon={<UploadOutlined />} />
              </Upload>
              <Button icon={<DeleteOutlined />} disabled={!branding.logoUrl} onClick={() => patch({ logoUrl: '' })} />
            </div>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.announcement')} description={t('pages.settings.branding.announcementDesc')}>
            <Input.TextArea value={branding.announcement} rows={2} maxLength={500} onChange={(e) => patch({ announcement: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.theme')} description={t('pages.settings.branding.themeDesc')}>
            <Segmented
              value={branding.theme}
              onChange={(v) => patch({ theme: v as SubBrandingTheme })}
              options={[
                { value: 'auto', label: t('pages.settings.branding.themeAuto') },
                { value: 'light', label: t('pages.settings.branding.themeLight') },
                { value: 'dark', label: t('pages.settings.branding.themeDark') },
              ]}
            />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.primaryColor')} description={t('pages.settings.branding.primaryColorDesc')}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <ColorPicker
                value={colorValue(branding.primaryColor)}
                onChangeComplete={(c) => patch({ primaryColor: c.toHexString() })}
                showText
              />
              <Button size="small" disabled={!branding.primaryColor} onClick={() => patch({ primaryColor: '' })}>
                {t('reset')}
              </Button>
            </div>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.pageBg')}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <ColorPicker
                value={colorValue(branding.pageBg)}
                onChangeComplete={(c) => patch({ pageBg: c.toHexString() })}
                showText
              />
              <Button size="small" disabled={!branding.pageBg} onClick={() => patch({ pageBg: '' })}>
                {t('reset')}
              </Button>
            </div>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.cardBg')}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <ColorPicker
                value={colorValue(branding.cardBg)}
                onChangeComplete={(c) => patch({ cardBg: c.toHexString() })}
                showText
              />
              <Button size="small" disabled={!branding.cardBg} onClick={() => patch({ cardBg: '' })}>
                {t('reset')}
              </Button>
            </div>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.textColor')}>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <ColorPicker
                value={colorValue(branding.textColor)}
                onChangeComplete={(c) => patch({ textColor: c.toHexString() })}
                showText
              />
              <Button size="small" disabled={!branding.textColor} onClick={() => patch({ textColor: '' })}>
                {t('reset')}
              </Button>
            </div>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.bgImage')} description={t('pages.settings.branding.bgImageDesc')}>
            <div style={{ display: 'flex', gap: 8, width: '100%' }}>
              <Input
                value={branding.bgImageUrl}
                placeholder="https://… / data:image/…"
                onChange={(e) => patch({ bgImageUrl: e.target.value })}
              />
              <Upload
                accept="image/*"
                showUploadList={false}
                beforeUpload={(file) => { void readImage(file as UploadFile & Blob, 'bgImageUrl'); return false; }}
              >
                <Button icon={<UploadOutlined />} />
              </Upload>
              <Button icon={<DeleteOutlined />} disabled={!branding.bgImageUrl} onClick={() => patch({ bgImageUrl: '' })} />
            </div>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.cardOpacity')} description={t('pages.settings.branding.cardOpacityDesc')}>
            <Slider min={20} max={100} value={branding.cardOpacity} onChange={(v) => patch({ cardOpacity: Number(v) })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.cardRadius')}>
            <InputNumber min={0} max={48} value={branding.cardRadius} style={{ width: '100%' }}
              onChange={(v) => patch({ cardRadius: Number(v) || 0 })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.blocks')} description={t('pages.settings.branding.blocksDesc')}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%' }}>
              {([
                ['showDetails', t('pages.settings.branding.blockDetails')],
                ['showUsage', t('pages.settings.branding.blockUsage')],
                ['showSubLinks', t('pages.settings.branding.blockSubLinks')],
                ['showConfigLinks', t('pages.settings.branding.blockConfigLinks')],
                ['showApps', t('pages.settings.branding.blockApps')],
                ['showThemeToggle', t('pages.settings.branding.blockThemeToggle')],
                ['showLangToggle', t('pages.settings.branding.blockLangToggle')],
              ] as [keyof SubBranding, string][]).map(([key, label]) => (
                <div key={String(key)} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                  <span>{label}</span>
                  <Switch
                    size="small"
                    checked={!!branding[key]}
                    onChange={(v) => patch({ [key]: v } as Partial<SubBranding>)}
                  />
                </div>
              ))}
            </div>
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.supportText')} description={t('pages.settings.branding.supportTextDesc')}>
            <Input value={branding.supportText} maxLength={40} onChange={(e) => patch({ supportText: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.supportUrl')}>
            <Input value={branding.supportUrl} placeholder="https://…" onChange={(e) => patch({ supportUrl: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.telegramUrl')}>
            <Input value={branding.telegramUrl} placeholder="https://t.me/…" onChange={(e) => patch({ telegramUrl: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.websiteUrl')}>
            <Input value={branding.websiteUrl} placeholder="https://…" onChange={(e) => patch({ websiteUrl: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.footerText')}>
            <Input.TextArea value={branding.footerText} rows={2} maxLength={300} onChange={(e) => patch({ footerText: e.target.value })} />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.customCss')} description={t('pages.settings.branding.customCssDesc')}>
            <Input.TextArea
              value={branding.customCss}
              rows={4}
              spellCheck={false}
              style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }}
              onChange={(e) => patch({ customCss: e.target.value })}
            />
          </SettingListItem>

          <SettingListItem paddings="small" title={t('pages.settings.branding.resetAll')} description={t('pages.settings.branding.resetAllDesc')}>
            <Button danger onClick={resetAll}>{t('reset')}</Button>
          </SettingListItem>
        </Col>

        <Col xs={24} lg={11}>
          <div style={{ position: 'sticky', top: 12 }}>
            <div style={{ marginBottom: 8, fontWeight: 600 }}>{t('pages.settings.branding.preview')}</div>
            <SubBrandingPreview branding={branding} isDark={isDark} />
          </div>
        </Col>
      </Row>
    </>
  );
}
