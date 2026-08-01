import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Badge,
  Button,
  Card,
  ConfigProvider,
  Empty,
  Input,
  Layout,
  Modal,
  Result,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, GlobalOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';

import { HttpUtil, IntlUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import AppSidebar from '@/layouts/AppSidebar';
import { setMessageInstance } from '@/utils/messageBus';
import type { ClientActivitySummary, ClientActivityDetail } from '@/generated/types';
import ClientActivityDetailModal from './ClientActivityDetailModal';
import '@/styles/page-shell.css';

interface ListResponse {
  enabled: boolean;
  clients: ClientActivitySummary[];
}

export default function ClientActivityPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [modal, modalContextHolder] = Modal.useModal();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => { setMessageInstance(messageApi); }, [messageApi]);

  const [enabled, setEnabled] = useState(true);
  const [rows, setRows] = useState<ClientActivitySummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetched, setFetched] = useState(false);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  const [detail, setDetail] = useState<ClientActivityDetail | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const msg = await HttpUtil.get<ListResponse>('/panel/api/clientActivity/list', undefined, { silent: true });
      if (!msg?.success || !msg.obj) {
        setFetchError(msg?.msg || t('pages.clientActivity.toasts.loadError'));
        return;
      }
      setEnabled(msg.obj.enabled);
      setRows(msg.obj.clients ?? []);
      setFetchError(null);
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
      setFetched(true);
    }
  }, [t]);

  useEffect(() => { void load(); }, [load]);

  const openDetail = useCallback(async (email: string) => {
    setDetailOpen(true);
    setDetail(null);
    setDetailLoading(true);
    try {
      const msg = await HttpUtil.get<ClientActivityDetail>(
        `/panel/api/clientActivity/detail/${encodeURIComponent(email)}`,
        undefined,
        { silent: true },
      );
      if (msg?.success && msg.obj) setDetail(msg.obj);
    } finally {
      setDetailLoading(false);
    }
  }, []);

  const clearAll = useCallback(() => {
    modal.confirm({
      title: t('pages.clientActivity.clearConfirmTitle'),
      content: t('pages.clientActivity.clearConfirmBody'),
      okText: t('pages.clientActivity.clearConfirmOk'),
      okButtonProps: { danger: true },
      cancelText: t('cancel'),
      onOk: async () => {
        const msg = await HttpUtil.post('/panel/api/clientActivity/clear');
        if (msg?.success) {
          messageApi.success(t('pages.clientActivity.toasts.cleared'));
          void load();
        }
      },
    });
  }, [modal, t, messageApi, load]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter(
      (r) =>
        r.email.toLowerCase().includes(q)
        || (r.operators ?? []).some((o) => o.toLowerCase().includes(q))
        || (r.countries ?? []).some((cty) => cty.toLowerCase().includes(q)),
    );
  }, [rows, search]);

  const columns: ColumnsType<ClientActivitySummary> = useMemo(() => [
    {
      title: t('pages.clientActivity.col.client'),
      dataIndex: 'email',
      key: 'email',
      render: (email: string, row) => (
        <Space>
          <Badge status={row.online ? 'success' : 'default'} />
          <Typography.Link onClick={() => openDetail(email)}>{email}</Typography.Link>
        </Space>
      ),
    },
    {
      title: t('pages.clientActivity.col.operator'),
      key: 'operator',
      render: (_: unknown, row) =>
        (row.operators ?? []).length > 0
          ? (row.operators ?? []).map((op) => <Tag key={op}>{op}</Tag>)
          : <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: t('pages.clientActivity.col.country'),
      key: 'country',
      responsive: ['md'],
      render: (_: unknown, row) =>
        (row.countries ?? []).length > 0
          ? (row.countries ?? []).join(', ')
          : <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: t('pages.clientActivity.col.ips'),
      dataIndex: 'ipCount',
      key: 'ipCount',
      align: 'center',
      responsive: ['sm'],
    },
    {
      title: t('pages.clientActivity.col.sites'),
      dataIndex: 'destCount',
      key: 'destCount',
      align: 'center',
    },
    {
      title: t('pages.clientActivity.col.lastSeen'),
      dataIndex: 'lastSeen',
      key: 'lastSeen',
      responsive: ['md'],
      render: (ts: number) => (ts > 0 ? IntlUtil.formatDate(ts * 1000) : '—'),
    },
  ], [t, openDetail]);

  const pageClass = useMemo(() => {
    const classes = ['page-shell'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Spin spinning={!fetched} delay={200} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={<Button type="primary" loading={loading} onClick={() => void load()}>{t('refresh')}</Button>}
                />
              ) : (
                <Card
                  title={
                    <Space>
                      <GlobalOutlined />
                      {t('menu.clientActivity')}
                    </Space>
                  }
                  extra={
                    <Space wrap>
                      <Input
                        allowClear
                        prefix={<SearchOutlined />}
                        placeholder={t('pages.clientActivity.searchPlaceholder')}
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        style={{ width: isMobile ? 140 : 240 }}
                      />
                      <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void load()}>
                        {!isMobile && t('refresh')}
                      </Button>
                      <Button danger icon={<DeleteOutlined />} disabled={rows.length === 0} onClick={clearAll}>
                        {!isMobile && t('pages.clientActivity.clearButton')}
                      </Button>
                    </Space>
                  }
                >
                  {/* The privacy note is deliberate: this data is the browsing
                      record of proxy users and is worth keeping in view. */}
                  <Alert
                    type="warning"
                    showIcon
                    style={{ marginBottom: 12 }}
                    message={t('pages.clientActivity.privacyNotice')}
                  />
                  {!enabled && (
                    <Alert
                      type="info"
                      showIcon
                      style={{ marginBottom: 12 }}
                      message={t('pages.clientActivity.disabledNotice')}
                    />
                  )}
                  <Table<ClientActivitySummary>
                    rowKey="email"
                    size="small"
                    columns={columns}
                    dataSource={filtered}
                    loading={loading}
                    locale={{ emptyText: <Empty description={t('pages.clientActivity.empty')} /> }}
                    pagination={{ pageSize: 20, hideOnSinglePage: true, responsive: true }}
                    scroll={{ x: 'max-content' }}
                  />
                </Card>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <ClientActivityDetailModal
          open={detailOpen}
          loading={detailLoading}
          detail={detail}
          onClose={() => setDetailOpen(false)}
        />
      </Layout>
    </ConfigProvider>
  );
}
