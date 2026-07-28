import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Divider, Input, InputNumber, Select, Space, Switch, Tabs } from 'antd';
import { BellOutlined, SendOutlined, SettingOutlined, ShoppingOutlined } from '@ant-design/icons';
import { LanguageManager } from '@/utils';
import { HttpUtil } from '@/utils';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { TelegramNotifications } from '@/components/ui/notifications/TelegramNotifications';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { catTabLabel } from './catTabLabel';

interface TelegramTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

// The notification schedule is fed straight to robfig/cron's AddJob (see
// web.go startTask), which accepts @every <duration>, the @hourly/@daily/...
// macros, and full crontab expressions. This builder covers the common cases
// with dropdowns so users don't have to memorise the syntax, while "Custom"
// preserves the raw crontab escape hatch.
type Unit = 's' | 'm' | 'h';
type Macro = '@hourly' | '@daily' | '@weekly' | '@monthly';
type Mode = 'every' | Macro | 'custom';
const MACROS: Macro[] = ['@hourly', '@daily', '@weekly', '@monthly'];
const EVERY_RE = /^@every\s+(\d+)\s*([smh])$/i;

interface RunTime {
  mode: Mode;
  num: number;
  unit: Unit;
  custom: string;
}

function parseRunTime(raw: string): RunTime {
  const v = (raw ?? '').trim();
  const m = v.match(EVERY_RE);
  if (m) {
    return { mode: 'every', num: Math.max(1, Number(m[1]) || 1), unit: m[2].toLowerCase() as Unit, custom: '' };
  }
  if ((MACROS as string[]).includes(v)) {
    return { mode: v as Macro, num: 1, unit: 'h', custom: '' };
  }
  return { mode: 'custom', num: 1, unit: 'h', custom: v };
}

function composeRunTime(s: RunTime): string {
  if (s.mode === 'every') return `@every ${Math.max(1, s.num || 1)}${s.unit}`;
  if (s.mode === 'custom') return s.custom;
  return s.mode;
}

// The panel's cron runs with seconds enabled (cron.WithSeconds() in web.go), so
// crontab expressions are 6-field: "second minute hour day month weekday". When
// the user drops into Custom we seed the box with the crontab equivalent of the
// current selection rather than a bare @macro, so they get a real expression to
// edit (and one that the 6-field parser accepts).
function toCrontab(s: RunTime): string {
  switch (s.mode) {
    case '@hourly': return '0 0 * * * *';
    case '@daily': return '0 0 0 * * *';
    case '@weekly': return '0 0 0 * * 0';
    case '@monthly': return '0 0 0 1 * *';
    case 'every': {
      const n = Math.max(1, s.num || 1);
      if (s.unit === 's') return `*/${n} * * * * *`;
      if (s.unit === 'm') return `0 */${n} * * * *`;
      return `0 0 */${n} * * *`;
    }
    default: return s.custom;
  }
}

function NotifyTimeField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  // Init once: the Settings tabs only mount after settings are fetched, so the
  // incoming value is already the persisted one.
  const [state, setState] = useState<RunTime>(() => parseRunTime(value));

  function update(patch: Partial<RunTime>) {
    const next = { ...state, ...patch };
    setState(next);
    onChange(composeRunTime(next));
  }

  function onModeChange(mode: Mode) {
    // Seed Custom with the crontab equivalent of the current selection so the
    // box starts from a real expression (e.g. "0 0 0 * * *", not "@daily").
    if (mode === 'custom' && !state.custom.trim()) {
      update({ mode, custom: toCrontab(state) });
    } else {
      update({ mode });
    }
  }

  const modeOptions = [
    { value: 'every', label: t('pages.settings.notifyTime.every') },
    { value: '@hourly', label: t('pages.settings.notifyTime.hourly') },
    { value: '@daily', label: t('pages.settings.notifyTime.daily') },
    { value: '@weekly', label: t('pages.settings.notifyTime.weekly') },
    { value: '@monthly', label: t('pages.settings.notifyTime.monthly') },
    { value: 'custom', label: t('pages.settings.notifyTime.custom') },
  ];
  const unitOptions = [
    { value: 's', label: t('pages.settings.notifyTime.seconds') },
    { value: 'm', label: t('pages.settings.notifyTime.minutes') },
    { value: 'h', label: t('pages.settings.notifyTime.hours') },
  ];

  return (
    <Space orientation="vertical" size="small" style={{ width: '100%' }}>
      <Select<Mode>
        style={{ width: '100%' }}
        value={state.mode}
        options={modeOptions}
        onChange={onModeChange}
      />
      {state.mode === 'every' && (
        <Space.Compact style={{ width: '100%' }}>
          <InputNumber
            min={1}
            style={{ width: '50%' }}
            value={state.num}
            onChange={(v) => update({ num: Math.max(1, Number(v) || 1) })}
          />
          <Select<Unit>
            style={{ width: '50%' }}
            value={state.unit}
            options={unitOptions}
            onChange={(unit) => update({ unit })}
          />
        </Space.Compact>
      )}
      {state.mode === 'custom' && (
        <Input
          value={state.custom}
          placeholder="0 30 8 * * *"
          onChange={(e) => update({ custom: e.target.value })}
        />
      )}
    </Space>
  );
}

interface SalesBotStatus {
  running: boolean;
  enabled: boolean;
  hasToken: boolean;
  adminsCount: number;
}

export default function TelegramTab({ allSetting, updateSetting }: TelegramTabProps) {
  // The shop attaches every config it sells to one inbound, so the admin picks
  // it from the real list rather than typing an id.
  const [inboundOptions, setInboundOptions] = useState<{ value: number; label: string }[]>([]);
  const [inboundsLoading, setInboundsLoading] = useState(false);
  useEffect(() => {
    let cancelled = false;
    setInboundsLoading(true);
    HttpUtil.get<{ id: number; remark: string; protocol: string; port: number }[]>(
      '/panel/api/inbounds/options', undefined, { silent: true },
    )
      .then((msg) => {
        if (cancelled) return;
        setInboundOptions((msg.obj ?? []).map((ib) => ({
          value: ib.id,
          label: `#${ib.id} · ${ib.remark || ib.protocol} (:${ib.port})`,
        })));
      })
      .catch(() => null)
      .finally(() => { if (!cancelled) setInboundsLoading(false); });
    return () => { cancelled = true; };
  }, []);

  // Saving a bad token leaves the bot down with only a log line to show for it,
  // so the tab asks the server whether it is actually polling Telegram.
  const [salesStatus, setSalesStatus] = useState<SalesBotStatus | null>(null);
  useEffect(() => {
    let cancelled = false;
    const load = () => {
      HttpUtil.get<SalesBotStatus>('/panel/api/setting/salesBotStatus', undefined, { silent: true })
        .then((msg) => { if (!cancelled && msg?.success) setSalesStatus(msg.obj ?? null); })
        .catch(() => null);
    };
    load();
    const timer = setInterval(load, 10000);
    return () => { cancelled = true; clearInterval(timer); };
  }, []);

  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; msg: string } | null>(null);

  async function handleTestTgBot() {
    setTestLoading(true);
    setTestResult(null);
    try {
      const res = await HttpUtil.post('/panel/api/setting/testTgBot') as { success?: boolean; msg?: string };
      setTestResult({ success: !!res.success, msg: res.msg || '' });
    } catch (e: unknown) {
      setTestResult({ success: false, msg: e instanceof Error ? e.message : t('pages.settings.requestFailed') });
    } finally {
      setTestLoading(false);
    }
  }

  const langOptions = useMemo(
    () => LanguageManager.supportedLanguages.map((l: { value: string; name: string; icon: string }) => ({
      value: l.value,
      label: (
        <>
          <span role="img" aria-label={l.name}>{l.icon}</span>
          &nbsp;&nbsp;<span>{l.name}</span>
        </>
      ),
    })),
    [],
  );

  return (
    <Tabs defaultActiveKey="1" items={[
      {
        key: '1',
        label: catTabLabel(<SettingOutlined />, t('pages.settings.panelSettings'), isMobile),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.telegramBotEnable')} description={t('pages.settings.telegramBotEnableDesc')}>
              <Switch checked={allSetting.tgBotEnable} onChange={(v) => updateSetting({ tgBotEnable: v })} />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('pages.settings.telegramToken')}
              description={allSetting.hasTgBotToken ? t('pages.settings.telegramTokenConfigured') : t('pages.settings.telegramTokenDesc')}
            >
              <Input.Password
                value={allSetting.tgBotToken}
                placeholder={allSetting.hasTgBotToken ? t('pages.settings.telegramTokenPlaceholder') : ''}
                onChange={(e) => updateSetting({ tgBotToken: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.telegramChatId')} description={t('pages.settings.telegramChatIdDesc')}>
              <Input value={allSetting.tgBotChatId} onChange={(e) => updateSetting({ tgBotChatId: e.target.value })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.telegramBotLanguage')}>
              <Select
                value={allSetting.tgLang}
                onChange={(v) => updateSetting({ tgLang: v })}
                style={{ width: '100%' }}
                options={langOptions}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.telegramAPIServer')} description={t('pages.settings.telegramAPIServerDesc')}>
              <Input value={allSetting.tgBotAPIServer} placeholder="https://api.example.com"
                onChange={(e) => updateSetting({ tgBotAPIServer: e.target.value })} />
            </SettingListItem>

            <Space orientation="vertical" size={8} style={{ width: '100%', marginTop: 16 }}>
              <Button type="primary" icon={<SendOutlined />} loading={testLoading} onClick={handleTestTgBot}>
                {t('pages.settings.testTgBot')}
              </Button>
              {testResult && (
                <Alert
                  type={testResult.success ? 'success' : 'error'}
                  title={testResult.msg}
                  showIcon
                  closable
                  onClose={() => setTestResult(null)}
                />
              )}
            </Space>
          </>
        ),
      },
      {
        key: '3',
        label: catTabLabel(<ShoppingOutlined />, t('pages.settings.salesBot'), isMobile),
        children: (
          <>
            <Alert
              type="info"
              showIcon
              title={t('pages.settings.salesBotIntro')}
              style={{ marginBottom: 12 }}
            />
            {salesStatus && salesStatus.enabled && (
              <Alert
                type={salesStatus.running ? 'success' : 'error'}
                showIcon
                data-testid="sales-bot-status"
                title={salesStatus.running
                  ? t('pages.settings.salesBotRunning')
                  : t(salesStatus.hasToken ? 'pages.settings.salesBotNotRunning' : 'pages.settings.salesBotNoToken')}
                style={{ marginBottom: 12 }}
              />
            )}
            {salesStatus && salesStatus.enabled && salesStatus.running && salesStatus.adminsCount === 0 && (
              <Alert
                type="warning"
                showIcon
                title={t('pages.settings.salesBotNoAdmins')}
                style={{ marginBottom: 12 }}
              />
            )}
            <SettingListItem paddings="small" title={t('pages.settings.salesBotEnable')} description={t('pages.settings.salesBotEnableDesc')}>
              <Switch
                checked={allSetting.salesBotEnable}
                data-testid="sales-bot-enable"
                onChange={(v) => updateSetting({ salesBotEnable: v })}
              />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title={t('pages.settings.salesBotToken')}
              description={allSetting.hasSalesBotToken ? t('pages.settings.telegramTokenConfigured') : t('pages.settings.salesBotTokenDesc')}
            >
              <Input.Password
                value={allSetting.salesBotToken}
                disabled={!allSetting.salesBotEnable}
                placeholder={allSetting.hasSalesBotToken ? t('pages.settings.telegramTokenPlaceholder') : ''}
                onChange={(e) => updateSetting({ salesBotToken: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.salesBotAdmins')} description={t('pages.settings.salesBotAdminsDesc')}>
              <Input
                value={allSetting.salesBotAdmins}
                disabled={!allSetting.salesBotEnable}
                placeholder="123456789,987654321"
                onChange={(e) => updateSetting({ salesBotAdmins: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.salesBotPanelUrl')} description={t('pages.settings.salesBotPanelUrlDesc')}>
              <Input
                value={allSetting.salesBotPanelUrl}
                disabled={!allSetting.salesBotEnable}
                placeholder="panel.example.com"
                onChange={(e) => updateSetting({ salesBotPanelUrl: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.salesBotPayText')} description={t('pages.settings.salesBotPayTextDesc')}>
              <Input.TextArea
                rows={3}
                value={allSetting.salesBotPayText}
                disabled={!allSetting.salesBotEnable}
                onChange={(e) => updateSetting({ salesBotPayText: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.salesBotWelcome')} description={t('pages.settings.salesBotWelcomeDesc')}>
              <Input.TextArea
                rows={3}
                value={allSetting.salesBotWelcome}
                disabled={!allSetting.salesBotEnable}
                onChange={(e) => updateSetting({ salesBotWelcome: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.salesBotSupport')} description={t('pages.settings.salesBotSupportDesc')}>
              <Input
                value={allSetting.salesBotSupport}
                disabled={!allSetting.salesBotEnable}
                placeholder="@support"
                onChange={(e) => updateSetting({ salesBotSupport: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.salesBotCurrency')} description={t('pages.settings.salesBotCurrencyDesc')}>
              <Input
                value={allSetting.salesBotCurrency}
                disabled={!allSetting.salesBotEnable}
                onChange={(e) => updateSetting({ salesBotCurrency: e.target.value })}
              />
            </SettingListItem>

            <Divider style={{ marginTop: 24 }}>{t('pages.settings.shopSection')}</Divider>
            <Alert type="info" showIcon title={t('pages.settings.shopIntro')} style={{ marginBottom: 12 }} />

            <SettingListItem paddings="small" title={t('pages.settings.shopPricePerGB')} description={t('pages.settings.shopPricePerGBDesc')}>
              <InputNumber min={0} style={{ width: '100%' }} value={allSetting.shopPricePerGB}
                disabled={!allSetting.salesBotEnable} data-testid="shop-price-per-gb"
                onChange={(v) => updateSetting({ shopPricePerGB: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopPricePerDay')} description={t('pages.settings.shopPricePerDayDesc')}>
              <InputNumber min={0} style={{ width: '100%' }} value={allSetting.shopPricePerDay}
                disabled={!allSetting.salesBotEnable}
                onChange={(v) => updateSetting({ shopPricePerDay: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopInboundId')} description={t('pages.settings.shopInboundIdDesc')}>
              <Select
                style={{ width: '100%' }}
                value={allSetting.shopInboundId || undefined}
                disabled={!allSetting.salesBotEnable}
                loading={inboundsLoading}
                options={inboundOptions}
                optionFilterProp="label"
                showSearch
                allowClear
                data-testid="shop-inbound"
                onChange={(v) => updateSetting({ shopInboundId: Number(v) || 0 })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopConfigDays')} description={t('pages.settings.shopConfigDaysDesc')}>
              <InputNumber min={0} style={{ width: '100%' }} value={allSetting.shopConfigDays}
                disabled={!allSetting.salesBotEnable}
                onChange={(v) => updateSetting({ shopConfigDays: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopBillingInterval')} description={t('pages.settings.shopBillingIntervalDesc')}>
              <InputNumber min={1} max={1440} style={{ width: '100%' }} value={allSetting.shopBillingInterval}
                disabled={!allSetting.salesBotEnable} data-testid="shop-billing-interval"
                onChange={(v) => updateSetting({ shopBillingInterval: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopMinTopUp')} description={t('pages.settings.shopMinTopUpDesc')}>
              <InputNumber min={0} style={{ width: '100%' }} value={allSetting.shopMinTopUp}
                disabled={!allSetting.salesBotEnable} data-testid="shop-min-topup"
                onChange={(v) => updateSetting({ shopMinTopUp: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopMaxTopUp')} description={t('pages.settings.shopMaxTopUpDesc')}>
              <InputNumber min={0} style={{ width: '100%' }} value={allSetting.shopMaxTopUp}
                disabled={!allSetting.salesBotEnable} data-testid="shop-max-topup"
                onChange={(v) => updateSetting({ shopMaxTopUp: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopMinBalance')} description={t('pages.settings.shopMinBalanceDesc')}>
              <InputNumber min={0} style={{ width: '100%' }} value={allSetting.shopMinBalance}
                disabled={!allSetting.salesBotEnable}
                onChange={(v) => updateSetting({ shopMinBalance: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopMaxVolumeGB')} description={t('pages.settings.shopMaxVolumeGBDesc')}>
              <InputNumber min={0} style={{ width: '100%' }} value={allSetting.shopMaxVolumeGB}
                disabled={!allSetting.salesBotEnable}
                onChange={(v) => updateSetting({ shopMaxVolumeGB: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.shopJoinChannel')} description={t('pages.settings.shopJoinChannelDesc')}>
              <Input value={allSetting.shopJoinChannel} placeholder="@mychannel"
                disabled={!allSetting.salesBotEnable} data-testid="shop-channel"
                onChange={(e) => updateSetting({ shopJoinChannel: e.target.value })} />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '2',
        label: catTabLabel(<BellOutlined />, t('pages.settings.notifications'), isMobile),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.telegramNotifyTime')} description={t('pages.settings.telegramNotifyTimeDesc')}>
              <NotifyTimeField value={allSetting.tgRunTime} onChange={(v) => updateSetting({ tgRunTime: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.tgNotifyBackup')} description={t('pages.settings.tgNotifyBackupDesc')}>
              <Switch checked={allSetting.tgBotBackup} onChange={(v) => updateSetting({ tgBotBackup: v })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.tgEventBusNotify')} description={t('pages.settings.tgEventBusNotifyDesc')}>
              <TelegramNotifications allSetting={allSetting} updateSetting={updateSetting} />
            </SettingListItem>
          </>
        ),
      },
    ]} />
  );
}
