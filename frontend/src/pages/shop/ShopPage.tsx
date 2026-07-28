import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Input,
  InputNumber,
  Layout,
  Modal,
  Popconfirm,
  Progress,
  Row,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CheckOutlined,
  CloseOutlined,
  DeleteOutlined,
  DollarOutlined,
  InboxOutlined,
  ReloadOutlined,
  StopOutlined,
  ThunderboltOutlined,
  WalletOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { useAllSettings } from '@/api/queries/useAllSettings';
import '@/styles/page-shell.css';
import './ShopPage.css';

interface ShopUser {
  telegramId: number;
  username: string;
  firstName: string;
  balance: number;
  totalPaid: number;
  totalSpent: number;
  blocked: boolean;
}

interface TopUp {
  id: number;
  telegramId: number;
  telegramName: string;
  amount: number;
  status: string;
  receiptFileId: string;
  note: string;
  createdAt: number;
}

interface ConfigUsage {
  config: {
    id: number;
    telegramId: number;
    email: string;
    volumeGB: number;
    chargedTraffic: number;
    chargedDays: number;
    active: boolean;
    paused: boolean;
  };
  usedBytes: number;
  totalGB: number;
}

interface ShopStats {
  users: number;
  configs: number;
  activeConfigs: number;
  walletBalance: number;
  totalPaid: number;
  totalSpent: number;
  pendingTopUps: number;
  suspendedUsers: number;
}

const STATUS_COLORS: Record<string, string> = {
  pending: 'default',
  review: 'gold',
  approved: 'green',
  rejected: 'red',
};

const GB = 1024 * 1024 * 1024;

export default function ShopPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { allSetting } = useAllSettings();
  const currency = allSetting.salesBotCurrency || '';

  const pageClass = useMemo(() => {
    const classes = ['shop-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const usersQ = useQuery({
    queryKey: ['shopUsers'],
    queryFn: async () =>
      (await HttpUtil.get<ShopUser[]>('/panel/api/shop/users', undefined, { silent: true })).obj ?? [],
    refetchInterval: 30000,
  });
  const topupsQ = useQuery({
    queryKey: ['shopTopUps'],
    queryFn: async () =>
      (await HttpUtil.get<TopUp[]>('/panel/api/shop/topups', undefined, { silent: true })).obj ?? [],
    refetchInterval: 30000,
  });
  const configsQ = useQuery({
    queryKey: ['shopConfigs'],
    queryFn: async () =>
      (await HttpUtil.get<ConfigUsage[]>('/panel/api/shop/configs', undefined, { silent: true })).obj ?? [],
    refetchInterval: 30000,
  });
  const statsQ = useQuery({
    queryKey: ['shopStats'],
    queryFn: async () =>
      (await HttpUtil.get<ShopStats>('/panel/api/shop/stats', undefined, { silent: true })).obj,
    refetchInterval: 30000,
  });

  const refresh = () => {
    for (const key of ['shopUsers', 'shopTopUps', 'shopConfigs', 'shopStats']) {
      qc.invalidateQueries({ queryKey: [key] });
    }
  };

  const billMut = useMutation({
    mutationFn: async () => HttpUtil.post('/panel/api/shop/bill'),
    onSuccess: () => refresh(),
  });
  const approveMut = useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/shop/topups/approve/${id}`),
    onSuccess: () => refresh(),
  });
  const rejectMut = useMutation({
    mutationFn: async ({ id, note }: { id: number; note: string }) =>
      HttpUtil.post(`/panel/api/shop/topups/reject/${id}`, { note }),
    onSuccess: () => refresh(),
  });
  const adjustMut = useMutation({
    mutationFn: async ({ id, amount, details }: { id: number; amount: number; details: string }) =>
      HttpUtil.post(`/panel/api/shop/users/${id}/adjust`, { amount, details }),
    onSuccess: () => refresh(),
  });
  const blockMut = useMutation({
    mutationFn: async ({ id, blocked }: { id: number; blocked: boolean }) =>
      HttpUtil.post(`/panel/api/shop/users/${id}/block`, { blocked }),
    onSuccess: () => refresh(),
  });
  const deleteConfigMut = useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/shop/configs/del/${id}`),
    onSuccess: () => refresh(),
  });

  const [rejecting, setRejecting] = useState<TopUp | null>(null);
  const [rejectNote, setRejectNote] = useState('');
  const [adjusting, setAdjusting] = useState<ShopUser | null>(null);
  const [adjustAmount, setAdjustAmount] = useState<number>(0);
  const [adjustNote, setAdjustNote] = useState('');

  const money = (n: number) => `${(n ?? 0).toLocaleString()} ${currency}`;

  const userColumns: ColumnsType<ShopUser> = [
    {
      title: t('pages.shop.user'),
      dataIndex: 'telegramId',
      render: (id: number, row) => (
        <div className="shop-cell">
          <span className="shop-cell-title">{row.firstName || row.username || id}</span>
          <span className="shop-cell-meta">
            {id}{row.username ? ` · @${row.username}` : ''}
          </span>
        </div>
      ),
    },
    {
      title: t('pages.shop.balance'),
      dataIndex: 'balance',
      width: 170,
      render: (v: number) => <b style={{ color: v > 0 ? undefined : '#ff4d4f' }}>{money(v)}</b>,
    },
    { title: t('pages.shop.paid'), dataIndex: 'totalPaid', width: 150, render: (v: number) => money(v) },
    { title: t('pages.shop.spent'), dataIndex: 'totalSpent', width: 150, render: (v: number) => money(v) },
    {
      title: t('pages.shop.status'),
      dataIndex: 'blocked',
      width: 110,
      render: (blocked: boolean) =>
        blocked ? <Tag color="red">{t('pages.shop.blocked')}</Tag> : <Tag color="green">{t('pages.shop.active')}</Tag>,
    },
    {
      title: t('pages.shop.actions'),
      key: 'actions',
      width: 130,
      align: 'right',
      render: (_: unknown, row) => (
        <Space size={4}>
          <Tooltip title={t('pages.shop.adjust')}>
            <Button
              size="small"
              icon={<DollarOutlined />}
              data-testid={`adjust-${row.telegramId}`}
              onClick={() => { setAdjusting(row); setAdjustAmount(0); setAdjustNote(''); }}
            />
          </Tooltip>
          <Tooltip title={row.blocked ? t('pages.shop.unblock') : t('pages.shop.block')}>
            <Button
              size="small"
              danger={!row.blocked}
              icon={row.blocked ? <CheckOutlined /> : <StopOutlined />}
              onClick={() => blockMut.mutate({ id: row.telegramId, blocked: !row.blocked })}
            />
          </Tooltip>
        </Space>
      ),
    },
  ];

  const topupColumns: ColumnsType<TopUp> = [
    { title: '#', dataIndex: 'id', width: 64 },
    {
      title: t('pages.shop.user'),
      dataIndex: 'telegramName',
      render: (name: string, row) => (
        <div className="shop-cell">
          <span className="shop-cell-title">{name || '—'}</span>
          <span className="shop-cell-meta">{row.telegramId}</span>
        </div>
      ),
    },
    { title: t('pages.shop.amount'), dataIndex: 'amount', width: 160, render: (v: number) => <b>{money(v)}</b> },
    {
      title: t('pages.shop.status'),
      dataIndex: 'status',
      width: 150,
      render: (status: string, row) => (
        <Space direction="vertical" size={0}>
          <Tag color={STATUS_COLORS[status] ?? 'default'}>{t(`pages.shop.statuses.${status}`, status)}</Tag>
          {row.note && <Typography.Text type="secondary" style={{ fontSize: 12 }}>{row.note}</Typography.Text>}
        </Space>
      ),
    },
    {
      title: t('pages.shop.actions'),
      key: 'actions',
      width: 130,
      align: 'right',
      render: (_: unknown, row) => {
        if (row.status === 'approved' || row.status === 'rejected') {
          return <Typography.Text type="secondary">—</Typography.Text>;
        }
        return (
          <Space size={4}>
            <Popconfirm
              title={t('pages.shop.confirmApprove')}
              onConfirm={() => approveMut.mutate(row.id)}
              okText={t('confirm')}
              cancelText={t('cancel')}
            >
              <Tooltip title={t('pages.shop.approve')}>
                <Button size="small" type="primary" icon={<CheckOutlined />} loading={approveMut.isPending} />
              </Tooltip>
            </Popconfirm>
            <Tooltip title={t('pages.shop.reject')}>
              <Button size="small" danger icon={<CloseOutlined />}
                onClick={() => { setRejecting(row); setRejectNote(''); }} />
            </Tooltip>
          </Space>
        );
      },
    },
  ];

  const configColumns: ColumnsType<ConfigUsage> = [
    {
      title: t('pages.shop.config'),
      key: 'email',
      render: (_: unknown, row) => (
        <div className="shop-cell">
          <span className="shop-cell-title">{row.config.email}</span>
          <span className="shop-cell-meta">{row.config.telegramId}</span>
        </div>
      ),
    },
    {
      title: t('pages.shop.usage'),
      key: 'usage',
      width: 260,
      render: (_: unknown, row) => {
        const total = row.config.volumeGB * GB;
        const pct = total > 0 ? Math.min(100, Math.round((row.usedBytes / total) * 100)) : 0;
        return (
          <div className="admin-usage">
            <div className="admin-usage-row">
              <span>{SizeFormatter.sizeFormat(row.usedBytes)}</span>
              <span className="admin-usage-label">
                {row.config.volumeGB > 0 ? `${row.config.volumeGB} GB` : '∞'}
              </span>
            </div>
            {total > 0 && <Progress percent={pct} size="small" />}
          </div>
        );
      },
    },
    {
      title: t('pages.shop.cost'),
      key: 'cost',
      width: 150,
      render: (_: unknown, row) => <b>{money(row.config.chargedTraffic + row.config.chargedDays)}</b>,
    },
    {
      title: t('pages.shop.status'),
      key: 'active',
      width: 110,
      // A config its owner paused from the bot is off on purpose — worth telling
      // apart from one the billing run cut off for an empty wallet.
      render: (_: unknown, row) =>
        row.config.paused ? (
          <Tag color="orange">{t('pages.shop.pausedByUser')}</Tag>
        ) : row.config.active ? (
          <Tag color="green">{t('enabled')}</Tag>
        ) : (
          <Tag color="red">{t('disabled')}</Tag>
        ),
    },
    {
      title: t('pages.shop.actions'),
      key: 'actions',
      width: 90,
      align: 'right',
      render: (_: unknown, row) => (
        <Popconfirm
          title={t('pages.shop.deleteConfig')}
          onConfirm={() => deleteConfigMut.mutate(row.config.id)}
          okText={t('confirm')}
          cancelText={t('cancel')}
        >
          <Tooltip title={t('delete')}>
            <Button size="small" danger icon={<DeleteOutlined />} />
          </Tooltip>
        </Popconfirm>
      ),
    },
  ];

  const stats = statsQ.data;

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div className="shop-shell">
              <div className="shop-header">
                <div>
                  <Typography.Title level={3} style={{ margin: 0 }}>{t('pages.shop.title')}</Typography.Title>
                  <Typography.Text type="secondary">{t('pages.shop.subtitle')}</Typography.Text>
                </div>
                <Space wrap>
                  <Button icon={<ReloadOutlined />} onClick={refresh}>{t('pages.shop.refresh')}</Button>
                  <Button type="primary" icon={<ThunderboltOutlined />} loading={billMut.isPending}
                    onClick={() => billMut.mutate()} data-testid="bill-now">
                    {t('pages.shop.billNow')}
                  </Button>
                </Space>
              </div>

              {!allSetting.salesBotEnable && <Alert type="warning" showIcon message={t('pages.shop.botOff')} />}
              {allSetting.salesBotEnable && !allSetting.shopPricePerGB && (
                <Alert type="warning" showIcon message={t('pages.shop.noPrice')} />
              )}
              {allSetting.salesBotEnable && !allSetting.shopInboundId && (
                <Alert type="error" showIcon message={t('pages.shop.noInbound')} />
              )}

              <Row gutter={[16, 16]}>
                {([
                  ['balance', <WalletOutlined key="w" />, t('pages.shop.walletBalance'), money(stats?.walletBalance ?? 0), 'green'],
                  ['spent', <DollarOutlined key="d" />, t('pages.shop.spent'), money(stats?.totalSpent ?? 0), 'blue'],
                  ['configs', <ThunderboltOutlined key="c" />, t('pages.shop.activeConfigs'),
                    `${stats?.activeConfigs ?? 0} / ${stats?.configs ?? 0}`, 'purple'],
                  ['pending', <InboxOutlined key="p" />, t('pages.shop.pendingTopUps'), String(stats?.pendingTopUps ?? 0), 'orange'],
                ] as const).map(([key, icon, label, value, color]) => (
                  <Col xs={12} md={6} key={key}>
                    <Card className={`shop-stat is-${color}`} data-testid={`shop-stat-${key}`}>
                      <span className="shop-stat-icon">{icon}</span>
                      <div>
                        <div className="shop-stat-value">{value}</div>
                        <div className="shop-stat-label">{label}</div>
                      </div>
                    </Card>
                  </Col>
                ))}
              </Row>

              <Card title={<span><InboxOutlined /> {t('pages.shop.topups')}</span>}>
                {(topupsQ.data?.length ?? 0) > 0 ? (
                  <Table<TopUp> rowKey="id" size="small" loading={topupsQ.isLoading}
                    dataSource={topupsQ.data ?? []} columns={topupColumns}
                    pagination={{ pageSize: 10, hideOnSinglePage: true }} scroll={{ x: 'max-content' }} />
                ) : (
                  <Empty description={t('pages.shop.noTopUps')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>

              <Card title={<span><WalletOutlined /> {t('pages.shop.users')}</span>}>
                {(usersQ.data?.length ?? 0) > 0 ? (
                  <Table<ShopUser> rowKey="telegramId" size="small" loading={usersQ.isLoading}
                    dataSource={usersQ.data ?? []} columns={userColumns}
                    pagination={{ pageSize: 10, hideOnSinglePage: true }} scroll={{ x: 'max-content' }} />
                ) : (
                  <Empty description={t('pages.shop.noUsers')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>

              <Card title={<span><ThunderboltOutlined /> {t('pages.shop.configs')}</span>}>
                {(configsQ.data?.length ?? 0) > 0 ? (
                  <Table<ConfigUsage> rowKey={(row) => row.config.id} size="small" loading={configsQ.isLoading}
                    dataSource={configsQ.data ?? []} columns={configColumns}
                    pagination={{ pageSize: 10, hideOnSinglePage: true }} scroll={{ x: 'max-content' }} />
                ) : (
                  <Empty description={t('pages.shop.noConfigs')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>
            </div>

            <Modal
              title={t('pages.shop.reject')}
              open={!!rejecting}
              onCancel={() => setRejecting(null)}
              onOk={() => {
                if (rejecting) rejectMut.mutate({ id: rejecting.id, note: rejectNote });
                setRejecting(null);
              }}
              okText={t('confirm')} cancelText={t('cancel')} okButtonProps={{ danger: true }} destroyOnHidden
            >
              <Typography.Paragraph>{t('pages.shop.rejectNoteDesc')}</Typography.Paragraph>
              <Input.TextArea rows={3} value={rejectNote} onChange={(e) => setRejectNote(e.target.value)} />
            </Modal>

            <Modal
              title={`${t('pages.shop.adjust')}${adjusting ? ` — ${adjusting.telegramId}` : ''}`}
              open={!!adjusting}
              onCancel={() => setAdjusting(null)}
              onOk={() => {
                if (adjusting) adjustMut.mutate({ id: adjusting.telegramId, amount: adjustAmount, details: adjustNote });
                setAdjusting(null);
              }}
              okText={t('confirm')} cancelText={t('cancel')} destroyOnHidden
            >
              <Typography.Paragraph>{t('pages.shop.adjustDesc')}</Typography.Paragraph>
              <InputNumber style={{ width: '100%' }} value={adjustAmount} data-testid="adjust-amount"
                onChange={(v) => setAdjustAmount(Number(v) || 0)} />
              <Typography.Paragraph style={{ marginTop: 12, marginBottom: 4 }}>
                {t('pages.shop.adjustNote')}
              </Typography.Paragraph>
              <Input value={adjustNote} onChange={(e) => setAdjustNote(e.target.value)} />
            </Modal>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
