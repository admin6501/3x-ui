import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Layout,
  Progress,
  Row,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  ApiOutlined,
  ArrowRightOutlined,
  ClockCircleOutlined,
  CloudDownloadOutlined,
  PoweroffOutlined,
  RiseOutlined,
  TeamOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter, IntlUtil } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { useDatepicker } from '@/hooks/useDatepicker';
import '@/styles/page-shell.css';
import './ResellerDashboardPage.css';

interface InboundSummary {
  id: number;
  remark: string;
  protocol: string;
  port: number;
  enable: boolean;
  up: number;
  down: number;
  clients: number;
}

interface ClientSummary {
  email: string;
  enable: boolean;
  up: number;
  down: number;
  total: number;
  expiryTime: number;
  online: boolean;
  createdAt: number;
}

interface Overview {
  username: string;
  role: string;
  disabled: boolean;
  trafficUsedBytes: number;
  trafficQuotaGB: number;
  currentClients: number;
  clientQuota: number;
  clientsCreatedTotal: number;
  clientsActive: number;
  clientsOnline: number;
  clientsExpiring: number;
  clientsEnded: number;
  clientsDisabled: number;
  inbounds: InboundSummary[];
  expiringSoon: ClientSummary[];
  recent: ClientSummary[];
}

const GB = 1024 * 1024 * 1024;

// Quota bars turn amber then red as the reseller approaches the ceiling, so the
// state is readable at a glance rather than by reading the numbers.
function quotaColor(pct: number): string | undefined {
  if (pct >= 90) return '#ff4d4f';
  if (pct >= 70) return '#faad14';
  return undefined;
}

export default function ResellerDashboardPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { datepicker } = useDatepicker();

  const pageClass = useMemo(() => {
    const classes = ['reseller-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const overviewQ = useQuery({
    queryKey: ['resellerOverview'],
    queryFn: async () =>
      (await HttpUtil.get<Overview>('/panel/api/reseller/overview', undefined, { silent: true })).obj,
    refetchInterval: 30000,
  });

  const data = overviewQ.data;
  const loading = overviewQ.isLoading;

  const trafficQuotaBytes = (data?.trafficQuotaGB ?? 0) * GB;
  const trafficUsed = data?.trafficUsedBytes ?? 0;
  const trafficPct = trafficQuotaBytes > 0
    ? Math.min(100, Math.round((trafficUsed / trafficQuotaBytes) * 100))
    : 0;
  const trafficLeft = Math.max(0, trafficQuotaBytes - trafficUsed);

  const clientQuota = data?.clientQuota ?? 0;
  const currentClients = data?.currentClients ?? 0;
  const clientPct = clientQuota > 0 ? Math.min(100, Math.round((currentClients / clientQuota) * 100)) : 0;
  const clientsLeft = Math.max(0, clientQuota - currentClients);

  const unlimited = t('pages.reseller.unlimited');

  const inboundColumns: ColumnsType<InboundSummary> = useMemo(() => [
    {
      title: t('pages.reseller.inbound'),
      dataIndex: 'remark',
      render: (remark: string, row) => (
        <div className="reseller-inbound-cell">
          <span className="reseller-inbound-name">{remark || `#${row.id}`}</span>
          <span className="reseller-inbound-meta">
            {row.protocol} · :{row.port}
          </span>
        </div>
      ),
    },
    {
      title: t('pages.reseller.status'),
      dataIndex: 'enable',
      width: 110,
      render: (enable: boolean) =>
        enable
          ? <Tag color="green">{t('enabled')}</Tag>
          : <Tag color="red">{t('disabled')}</Tag>,
    },
    {
      title: t('pages.reseller.clients'),
      dataIndex: 'clients',
      width: 100,
      align: 'right',
      render: (n: number) => <b>{n}</b>,
    },
    {
      title: t('pages.reseller.traffic'),
      key: 'traffic',
      width: 190,
      align: 'right',
      render: (_, row) => {
        const total = row.up + row.down;
        const share = trafficUsed > 0 ? Math.round((total / trafficUsed) * 100) : 0;
        return (
          <Tooltip title={`↑ ${SizeFormatter.sizeFormat(row.up)} · ↓ ${SizeFormatter.sizeFormat(row.down)}`}>
            <span>
              <b>{SizeFormatter.sizeFormat(total)}</b>
              {trafficUsed > 0 && <span className="reseller-share"> ({share}%)</span>}
            </span>
          </Tooltip>
        );
      },
    },
  ], [t, trafficUsed]);

  const clientColumns = (kind: 'expiring' | 'recent'): ColumnsType<ClientSummary> => [
    {
      title: t('pages.reseller.client'),
      dataIndex: 'email',
      render: (email: string, row) => (
        <div className="reseller-inbound-cell">
          <span className="reseller-inbound-name">{email}</span>
          <span className="reseller-inbound-meta">
            {row.total > 0
              ? `${SizeFormatter.sizeFormat(row.up + row.down)} / ${SizeFormatter.sizeFormat(row.total)}`
              : `${SizeFormatter.sizeFormat(row.up + row.down)} · ${unlimited}`}
          </span>
        </div>
      ),
    },
    {
      title: kind === 'expiring' ? t('pages.reseller.expires') : t('pages.reseller.created'),
      key: 'when',
      width: 170,
      align: 'right',
      render: (_, row) => {
        const ms = kind === 'expiring' ? row.expiryTime : row.createdAt;
        if (!ms) return <Typography.Text type="secondary">—</Typography.Text>;
        return (
          <Tooltip title={IntlUtil.formatDate(ms, datepicker)}>
            <span>{IntlUtil.formatRelativeTime(ms)}</span>
          </Tooltip>
        );
      },
    },
    {
      title: '',
      key: 'flags',
      width: 90,
      align: 'right',
      render: (_, row) => (row.online ? <Tag color="blue">{t('online')}</Tag> : null),
    },
  ];

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div data-testid="reseller-dashboard" className="reseller-dashboard">
              <div className="reseller-header">
                <div>
                  <Typography.Title level={3} style={{ margin: 0 }} data-testid="reseller-title">
                    {t('pages.reseller.greeting', { name: data?.username ?? '' })}
                  </Typography.Title>
                  <Typography.Text type="secondary">{t('pages.reseller.subtitle')}</Typography.Text>
                </div>
                <Link to="/clients">
                  <Button type="primary" icon={<TeamOutlined />}>
                    {t('pages.reseller.manageClients')} <ArrowRightOutlined />
                  </Button>
                </Link>
              </div>

              {data?.disabled && (
                <Alert type="error" showIcon message={t('pages.reseller.accountDisabled')} />
              )}

              <Row gutter={[16, 16]}>
                <Col xs={24} lg={12}>
                  <Card loading={loading} data-testid="reseller-traffic-card" className="reseller-quota-card">
                    <div className="reseller-quota-head">
                      <span className="reseller-quota-icon"><CloudDownloadOutlined /></span>
                      <div>
                        <div className="reseller-quota-title">{t('pages.reseller.trafficUsed')}</div>
                        <div className="reseller-quota-value">{SizeFormatter.sizeFormat(trafficUsed)}</div>
                      </div>
                    </div>
                    {trafficQuotaBytes > 0 ? (
                      <>
                        <Progress
                          percent={trafficPct}
                          strokeColor={quotaColor(trafficPct)}
                          data-testid="reseller-traffic-progress"
                        />
                        <div className="reseller-quota-foot">
                          <span>{`${SizeFormatter.sizeFormat(trafficUsed)} ${t('pages.reseller.of')} ${data?.trafficQuotaGB} GB`}</span>
                          <span>{`${SizeFormatter.sizeFormat(trafficLeft)} ${t('pages.reseller.left')}`}</span>
                        </div>
                      </>
                    ) : (
                      <div className="reseller-quota-foot">
                        <span>{`${t('pages.reseller.trafficQuota')}: ${unlimited}`}</span>
                      </div>
                    )}
                  </Card>
                </Col>

                <Col xs={24} lg={12}>
                  <Card loading={loading} data-testid="reseller-clients-card" className="reseller-quota-card">
                    <div className="reseller-quota-head">
                      <span className="reseller-quota-icon"><TeamOutlined /></span>
                      <div>
                        <div className="reseller-quota-title">{t('pages.reseller.currentClients')}</div>
                        <div className="reseller-quota-value">
                          {currentClients}
                          {clientQuota > 0 && <span className="reseller-quota-of"> / {clientQuota}</span>}
                        </div>
                      </div>
                    </div>
                    {clientQuota > 0 ? (
                      <>
                        <Progress
                          percent={clientPct}
                          strokeColor={quotaColor(clientPct)}
                          data-testid="reseller-clients-progress"
                        />
                        <div className="reseller-quota-foot">
                          <span>{`${t('pages.reseller.totalCreated')}: ${data?.clientsCreatedTotal ?? 0}`}</span>
                          <span>{`${clientsLeft} ${t('pages.reseller.slotsLeft')}`}</span>
                        </div>
                      </>
                    ) : (
                      <div className="reseller-quota-foot">
                        <span>{`${t('pages.reseller.clientQuota')}: ${unlimited}`}</span>
                        <span>{`${t('pages.reseller.totalCreated')}: ${data?.clientsCreatedTotal ?? 0}`}</span>
                      </div>
                    )}
                  </Card>
                </Col>
              </Row>

              <Row gutter={[16, 16]} className="reseller-stat-row">
                {([
                  ['clientsActive', <ThunderboltOutlined key="a" />, t('pages.reseller.activeClients'), data?.clientsActive ?? 0, 'green'],
                  ['clientsOnline', <RiseOutlined key="o" />, t('online'), data?.clientsOnline ?? 0, 'blue'],
                  ['clientsExpiring', <ClockCircleOutlined key="e" />, t('pages.reseller.expiringSoon'), data?.clientsExpiring ?? 0, 'orange'],
                  ['clientsEnded', <PoweroffOutlined key="x" />, t('pages.reseller.endedClients'), data?.clientsEnded ?? 0, 'red'],
                ] as const).map(([key, icon, label, value, color]) => (
                  <Col xs={12} md={6} key={key}>
                    <Card loading={loading} className={`reseller-mini-card is-${color}`} data-testid={`reseller-${key}`}>
                      <span className="reseller-mini-icon">{icon}</span>
                      <div>
                        <div className="reseller-mini-value">{value}</div>
                        <div className="reseller-mini-label">{label}</div>
                      </div>
                    </Card>
                  </Col>
                ))}
              </Row>

              <Card
                title={<span><ApiOutlined /> {t('pages.reseller.myInbounds')}</span>}
                loading={loading}
                data-testid="reseller-inbounds-card"
              >
                {(data?.inbounds?.length ?? 0) > 0 ? (
                  <Table<InboundSummary>
                    rowKey="id"
                    size="small"
                    pagination={false}
                    dataSource={data?.inbounds ?? []}
                    columns={inboundColumns}
                    scroll={{ x: 'max-content' }}
                  />
                ) : (
                  <Empty description={t('pages.reseller.noInbounds')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>

              <Row gutter={[16, 16]}>
                <Col xs={24} lg={12}>
                  <Card
                    title={<span><ClockCircleOutlined /> {t('pages.reseller.expiringSoon')}</span>}
                    loading={loading}
                    data-testid="reseller-expiring-card"
                  >
                    {(data?.expiringSoon?.length ?? 0) > 0 ? (
                      <Table<ClientSummary>
                        rowKey="email"
                        size="small"
                        pagination={false}
                        dataSource={data?.expiringSoon ?? []}
                        columns={clientColumns('expiring')}
                        scroll={{ x: 'max-content' }}
                      />
                    ) : (
                      <Empty description={t('pages.reseller.noExpiring')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                    )}
                  </Card>
                </Col>

                <Col xs={24} lg={12}>
                  <Card
                    title={<span><TeamOutlined /> {t('pages.reseller.recentClients')}</span>}
                    loading={loading}
                    data-testid="reseller-recent-card"
                  >
                    {(data?.recent?.length ?? 0) > 0 ? (
                      <Table<ClientSummary>
                        rowKey="email"
                        size="small"
                        pagination={false}
                        dataSource={data?.recent ?? []}
                        columns={clientColumns('recent')}
                        scroll={{ x: 'max-content' }}
                      />
                    ) : (
                      <Empty description={t('pages.reseller.noClients')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                    )}
                  </Card>
                </Col>
              </Row>
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
