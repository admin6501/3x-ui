import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Card, Col, ConfigProvider, Layout, Progress, Row, Statistic, Typography } from 'antd';
import { CloudDownloadOutlined, TeamOutlined, RiseOutlined } from '@ant-design/icons';

import { HttpUtil, SizeFormatter } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import '@/styles/page-shell.css';

interface ResellerMe {
  username: string;
  role: string;
  trafficUsedBytes: number;
  currentClients: number;
  clientsCreatedTotal: number;
  trafficQuotaGB: number;
  clientQuota: number;
}

const GB = 1024 * 1024 * 1024;

export default function ResellerDashboardPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();

  const pageClass = useMemo(() => {
    const classes = ['reseller-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const meQ = useQuery({
    queryKey: ['resellerMe'],
    queryFn: async () =>
      (await HttpUtil.get<ResellerMe>('/panel/api/reseller/me', undefined, { silent: true })).obj,
    refetchInterval: 30000,
  });

  const data = meQ.data;
  const trafficQuotaBytes = (data?.trafficQuotaGB ?? 0) * GB;
  const trafficPct =
    trafficQuotaBytes > 0 ? Math.min(100, Math.round(((data?.trafficUsedBytes ?? 0) / trafficQuotaBytes) * 100)) : 0;
  const clientPct =
    (data?.clientQuota ?? 0) > 0
      ? Math.min(100, Math.round(((data?.currentClients ?? 0) / (data?.clientQuota ?? 1)) * 100))
      : 0;

  const unlimited = t('pages.reseller.unlimited');
  const statusColor = (pct: number) => (pct >= 90 ? '#ff4d4f' : pct >= 70 ? '#faad14' : undefined);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div data-testid="reseller-dashboard" style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div>
                <Typography.Title level={4} style={{ margin: 0 }} data-testid="reseller-title">
                  {t('pages.reseller.title')}
                </Typography.Title>
                <Typography.Text type="secondary">
                  {t('pages.reseller.subtitle')}
                  {data?.username ? ` — ${data.username}` : ''}
                </Typography.Text>
              </div>

              <Row gutter={[16, 16]}>
                <Col xs={24} md={12}>
                  <Card loading={meQ.isLoading} data-testid="reseller-traffic-card">
                    <Statistic
                      title={t('pages.reseller.trafficUsed')}
                      value={SizeFormatter.sizeFormat(data?.trafficUsedBytes ?? 0)}
                      prefix={<CloudDownloadOutlined />}
                    />
                    <div style={{ marginTop: 12 }}>
                      {trafficQuotaBytes > 0 ? (
                        <>
                          <Progress percent={trafficPct} strokeColor={statusColor(trafficPct)} data-testid="reseller-traffic-progress" />
                          <Typography.Text type="secondary">
                            {`${SizeFormatter.sizeFormat(data?.trafficUsedBytes ?? 0)} ${t('pages.reseller.of')} ${data?.trafficQuotaGB} GB`}
                          </Typography.Text>
                        </>
                      ) : (
                        <Typography.Text type="secondary">{`${t('pages.reseller.trafficQuota')}: ${unlimited}`}</Typography.Text>
                      )}
                    </div>
                  </Card>
                </Col>

                <Col xs={24} md={12}>
                  <Card loading={meQ.isLoading} data-testid="reseller-clients-card">
                    <Statistic
                      title={t('pages.reseller.currentClients')}
                      value={data?.currentClients ?? 0}
                      suffix={(data?.clientQuota ?? 0) > 0 ? ` / ${data?.clientQuota}` : ''}
                      prefix={<TeamOutlined />}
                    />
                    <div style={{ marginTop: 12 }}>
                      {(data?.clientQuota ?? 0) > 0 ? (
                        <Progress percent={clientPct} strokeColor={statusColor(clientPct)} data-testid="reseller-clients-progress" />
                      ) : (
                        <Typography.Text type="secondary">{`${t('pages.reseller.clientQuota')}: ${unlimited}`}</Typography.Text>
                      )}
                    </div>
                  </Card>
                </Col>

                <Col xs={24} md={12}>
                  <Card loading={meQ.isLoading} data-testid="reseller-total-created-card">
                    <Statistic
                      title={t('pages.reseller.totalCreated')}
                      value={data?.clientsCreatedTotal ?? 0}
                      prefix={<RiseOutlined />}
                    />
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
