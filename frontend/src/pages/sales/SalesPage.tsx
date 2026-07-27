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
  Form,
  Input,
  InputNumber,
  Layout,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
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
  EditOutlined,
  InboxOutlined,
  PlusOutlined,
  ReloadOutlined,
  ShoppingOutlined,
  TeamOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { useAllSettings } from '@/api/queries/useAllSettings';
import '@/styles/page-shell.css';
import './SalesPage.css';

interface ResellerPackage {
  id: number;
  name: string;
  description: string;
  price: number;
  trafficGB: number;
  clientQuota: number;
  durationDays: number;
  allowedInbounds: string;
  enable: boolean;
  sortOrder: number;
}

interface ResellerOrder {
  id: number;
  telegramId: number;
  telegramName: string;
  packageName: string;
  price: number;
  kind: string;
  status: string;
  receiptFileId: string;
  panelUsername: string;
  note: string;
  createdAt: number;
}

interface SalesStats {
  revenue: number;
  approvedOrders: number;
  pendingReview: number;
  rejectedOrders: number;
  buyers: number;
  resellers: number;
}

interface InboundOption {
  id: number;
  remark: string;
  protocol: string;
  port: number;
}

const STATUS_COLORS: Record<string, string> = {
  pending: 'default',
  review: 'gold',
  approved: 'green',
  rejected: 'red',
};

function csvToIds(csv: string): number[] {
  if (!csv) return [];
  return csv.split(',').map((s) => parseInt(s.trim(), 10)).filter((n) => Number.isFinite(n) && n > 0);
}

interface PackageForm {
  name: string;
  description?: string;
  price: number;
  trafficGB: number;
  clientQuota: number;
  durationDays: number;
  allowedInbounds: number[];
  enable: boolean;
  sortOrder: number;
}

export default function SalesPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { allSetting } = useAllSettings();
  const currency = allSetting.salesBotCurrency || '';

  const pageClass = useMemo(() => {
    const classes = ['sales-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const packagesQ = useQuery({
    queryKey: ['salesPackages'],
    queryFn: async () =>
      (await HttpUtil.get<ResellerPackage[]>('/panel/api/sales/packages', undefined, { silent: true })).obj ?? [],
  });
  const ordersQ = useQuery({
    queryKey: ['salesOrders'],
    queryFn: async () =>
      (await HttpUtil.get<ResellerOrder[]>('/panel/api/sales/orders', undefined, { silent: true })).obj ?? [],
    refetchInterval: 30000,
  });
  const statsQ = useQuery({
    queryKey: ['salesStats'],
    queryFn: async () =>
      (await HttpUtil.get<SalesStats>('/panel/api/sales/stats', undefined, { silent: true })).obj,
    refetchInterval: 30000,
  });
  const inboundsQ = useQuery({
    queryKey: ['inboundOptions'],
    queryFn: async () =>
      (await HttpUtil.get<InboundOption[]>('/panel/api/inbounds/options', undefined, { silent: true })).obj ?? [],
  });

  const inboundOptions = useMemo(
    () => (inboundsQ.data ?? []).map((ib) => ({
      value: ib.id,
      label: `#${ib.id} · ${ib.remark || ib.protocol} (:${ib.port})`,
    })),
    [inboundsQ.data],
  );

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['salesPackages'] });
    qc.invalidateQueries({ queryKey: ['salesOrders'] });
    qc.invalidateQueries({ queryKey: ['salesStats'] });
  };

  // ---- package modal ----
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<ResellerPackage | null>(null);
  const [form] = Form.useForm<PackageForm>();

  const openAdd = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({
      name: '', price: 0, trafficGB: 0, clientQuota: 0, durationDays: 30,
      allowedInbounds: [], enable: true, sortOrder: 0,
    });
    setModalOpen(true);
  };
  const openEdit = (row: ResellerPackage) => {
    setEditing(row);
    form.setFieldsValue({
      name: row.name,
      description: row.description,
      price: row.price,
      trafficGB: row.trafficGB,
      clientQuota: row.clientQuota,
      durationDays: row.durationDays,
      allowedInbounds: csvToIds(row.allowedInbounds),
      enable: row.enable,
      sortOrder: row.sortOrder,
    });
    setModalOpen(true);
  };

  const saveMut = useMutation({
    mutationFn: async (values: PackageForm) => {
      const body = { ...values, allowedInbounds: (values.allowedInbounds ?? []).join(',') };
      return editing
        ? HttpUtil.post(`/panel/api/sales/packages/update/${editing.id}`, body)
        : HttpUtil.post('/panel/api/sales/packages/add', body);
    },
    onSuccess: (res) => {
      if (res.success) {
        setModalOpen(false);
        refresh();
      }
    },
  });

  const deleteMut = useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/sales/packages/del/${id}`),
    onSuccess: () => refresh(),
  });

  // ---- order decisions ----
  const [credentials, setCredentials] = useState<{ username: string; password: string } | null>(null);

  const approveMut = useMutation({
    mutationFn: async (id: number) =>
      HttpUtil.post<{ username: string; password: string; isNew: boolean }>(`/panel/api/sales/orders/approve/${id}`),
    onSuccess: (res) => {
      refresh();
      if (res.success && res.obj?.isNew && res.obj.password) {
        setCredentials({ username: res.obj.username, password: res.obj.password });
      }
    },
  });
  const rejectMut = useMutation({
    mutationFn: async ({ id, note }: { id: number; note: string }) =>
      HttpUtil.post(`/panel/api/sales/orders/reject/${id}`, { note }),
    onSuccess: () => refresh(),
  });

  const [rejecting, setRejecting] = useState<ResellerOrder | null>(null);
  const [rejectNote, setRejectNote] = useState('');

  const money = (n: number) => `${(n ?? 0).toLocaleString()} ${currency}`;
  const quota = (n: number, unit: string) => (n > 0 ? `${n} ${unit}` : t('pages.sales.unlimited'));

  const packageColumns: ColumnsType<ResellerPackage> = [
    {
      title: t('pages.sales.package'),
      dataIndex: 'name',
      render: (name: string, row) => (
        <div className="sales-cell">
          <span className="sales-cell-title">{name}</span>
          {row.description && <span className="sales-cell-meta">{row.description}</span>}
        </div>
      ),
    },
    { title: t('pages.sales.price'), dataIndex: 'price', width: 160, render: (v: number) => <b>{money(v)}</b> },
    {
      title: t('pages.sales.traffic'),
      dataIndex: 'trafficGB',
      width: 130,
      render: (v: number) => quota(v, 'GB'),
    },
    {
      title: t('pages.sales.clients'),
      dataIndex: 'clientQuota',
      width: 130,
      render: (v: number) => quota(v, t('pages.sales.clientsUnit')),
    },
    {
      title: t('pages.sales.inbounds'),
      dataIndex: 'allowedInbounds',
      render: (csv: string) =>
        csvToIds(csv).length > 0
          ? <Space size={[4, 4]} wrap>{csvToIds(csv).map((id) => <Tag key={id}>#{id}</Tag>)}</Space>
          : <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: t('pages.sales.status'),
      dataIndex: 'enable',
      width: 110,
      render: (enable: boolean) =>
        enable ? <Tag color="green">{t('enabled')}</Tag> : <Tag color="red">{t('disabled')}</Tag>,
    },
    {
      title: t('pages.sales.actions'),
      key: 'actions',
      width: 120,
      align: 'right',
      render: (_: unknown, row) => (
        <Space size={4}>
          <Tooltip title={t('edit')}>
            <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(row)} />
          </Tooltip>
          <Popconfirm
            title={t('pages.sales.confirmDeletePackage')}
            onConfirm={() => deleteMut.mutate(row.id)}
            okText={t('confirm')}
            cancelText={t('cancel')}
          >
            <Tooltip title={t('delete')}>
              <Button size="small" danger icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const orderColumns: ColumnsType<ResellerOrder> = [
    { title: '#', dataIndex: 'id', width: 64 },
    {
      title: t('pages.sales.buyer'),
      dataIndex: 'telegramName',
      render: (name: string, row) => (
        <div className="sales-cell">
          <span className="sales-cell-title">{name || '—'}</span>
          <span className="sales-cell-meta">{row.telegramId}</span>
        </div>
      ),
    },
    {
      title: t('pages.sales.package'),
      dataIndex: 'packageName',
      render: (name: string, row) => (
        <div className="sales-cell">
          <span className="sales-cell-title">{name}</span>
          <span className="sales-cell-meta">
            {row.kind === 'renew' ? t('pages.sales.kindRenew') : t('pages.sales.kindNew')}
          </span>
        </div>
      ),
    },
    { title: t('pages.sales.price'), dataIndex: 'price', width: 150, render: (v: number) => money(v) },
    {
      title: t('pages.sales.status'),
      dataIndex: 'status',
      width: 140,
      render: (status: string, row) => (
        <Space direction="vertical" size={0}>
          <Tag color={STATUS_COLORS[status] ?? 'default'}>{t(`pages.sales.statuses.${status}`, status)}</Tag>
          {row.panelUsername && <Typography.Text type="secondary" style={{ fontSize: 12 }}>{row.panelUsername}</Typography.Text>}
        </Space>
      ),
    },
    {
      title: t('pages.sales.actions'),
      key: 'actions',
      width: 150,
      align: 'right',
      render: (_: unknown, row) => {
        if (row.status === 'approved' || row.status === 'rejected') {
          return <Typography.Text type="secondary">—</Typography.Text>;
        }
        return (
          <Space size={4}>
            <Popconfirm
              title={t('pages.sales.confirmApprove')}
              onConfirm={() => approveMut.mutate(row.id)}
              okText={t('confirm')}
              cancelText={t('cancel')}
            >
              <Tooltip title={t('pages.sales.approve')}>
                <Button size="small" type="primary" icon={<CheckOutlined />} loading={approveMut.isPending} />
              </Tooltip>
            </Popconfirm>
            <Tooltip title={t('pages.sales.reject')}>
              <Button
                size="small"
                danger
                icon={<CloseOutlined />}
                onClick={() => { setRejecting(row); setRejectNote(''); }}
              />
            </Tooltip>
          </Space>
        );
      },
    },
  ];

  const stats = statsQ.data;

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div className="sales-shell">
              <div className="sales-header">
                <div>
                  <Typography.Title level={3} style={{ margin: 0 }}>{t('pages.sales.title')}</Typography.Title>
                  <Typography.Text type="secondary">{t('pages.sales.subtitle')}</Typography.Text>
                </div>
                <Space wrap>
                  <Button icon={<ReloadOutlined />} onClick={refresh}>{t('pages.sales.refresh')}</Button>
                  <Button type="primary" icon={<PlusOutlined />} onClick={openAdd} data-testid="add-package">
                    {t('pages.sales.addPackage')}
                  </Button>
                </Space>
              </div>

              {!allSetting.salesBotEnable && (
                <Alert type="warning" showIcon message={t('pages.sales.botOff')} />
              )}

              <Row gutter={[16, 16]}>
                {([
                  ['revenue', <DollarOutlined key="r" />, t('pages.sales.revenue'), money(stats?.revenue ?? 0), 'green'],
                  ['approved', <ShoppingOutlined key="a" />, t('pages.sales.soldOrders'), String(stats?.approvedOrders ?? 0), 'blue'],
                  ['pending', <InboxOutlined key="p" />, t('pages.sales.pending'), String(stats?.pendingReview ?? 0), 'orange'],
                  ['resellers', <TeamOutlined key="t" />, t('pages.sales.resellers'), String(stats?.resellers ?? 0), 'purple'],
                ] as const).map(([key, icon, label, value, color]) => (
                  <Col xs={12} md={6} key={key}>
                    <Card className={`sales-stat is-${color}`} data-testid={`sales-stat-${key}`}>
                      <span className="sales-stat-icon">{icon}</span>
                      <div>
                        <div className="sales-stat-value">{value}</div>
                        <div className="sales-stat-label">{label}</div>
                      </div>
                    </Card>
                  </Col>
                ))}
              </Row>

              <Card title={<span><ShoppingOutlined /> {t('pages.sales.packages')}</span>}>
                {(packagesQ.data?.length ?? 0) > 0 ? (
                  <Table<ResellerPackage>
                    rowKey="id"
                    size="small"
                    loading={packagesQ.isLoading}
                    dataSource={packagesQ.data ?? []}
                    columns={packageColumns}
                    pagination={false}
                    scroll={{ x: 'max-content' }}
                  />
                ) : (
                  <Empty description={t('pages.sales.noPackages')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>

              <Card title={<span><InboxOutlined /> {t('pages.sales.orders')}</span>}>
                {(ordersQ.data?.length ?? 0) > 0 ? (
                  <Table<ResellerOrder>
                    rowKey="id"
                    size="small"
                    loading={ordersQ.isLoading}
                    dataSource={ordersQ.data ?? []}
                    columns={orderColumns}
                    pagination={{ pageSize: 20, hideOnSinglePage: true }}
                    scroll={{ x: 'max-content' }}
                  />
                ) : (
                  <Empty description={t('pages.sales.noOrders')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>
            </div>

            <Modal
              title={editing ? t('pages.sales.editPackage') : t('pages.sales.addPackage')}
              open={modalOpen}
              onCancel={() => setModalOpen(false)}
              onOk={() => form.submit()}
              confirmLoading={saveMut.isPending}
              okText={t('confirm')}
              cancelText={t('cancel')}
              destroyOnHidden
            >
              <Form form={form} layout="vertical" onFinish={(v) => saveMut.mutate(v)} preserve={false}>
                <Form.Item name="name" label={t('pages.sales.packageName')} rules={[{ required: true }]}>
                  <Input autoComplete="off" data-testid="package-name" />
                </Form.Item>
                <Form.Item name="description" label={t('pages.sales.packageDesc')}>
                  <Input.TextArea rows={2} />
                </Form.Item>
                <Row gutter={12}>
                  <Col span={12}>
                    <Form.Item name="price" label={`${t('pages.sales.price')} (${currency})`}>
                      <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item name="durationDays" label={t('pages.sales.durationDays')} extra={t('pages.sales.durationDesc')}>
                      <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                  </Col>
                </Row>
                <Row gutter={12}>
                  <Col span={12}>
                    <Form.Item name="trafficGB" label={t('pages.sales.trafficGB')} extra={t('pages.sales.zeroUnlimited')}>
                      <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item name="clientQuota" label={t('pages.sales.clientQuota')} extra={t('pages.sales.zeroUnlimited')}>
                      <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                  </Col>
                </Row>
                <Form.Item name="allowedInbounds" label={t('pages.sales.inbounds')} extra={t('pages.sales.inboundsDesc')}>
                  <Select
                    mode="multiple"
                    allowClear
                    options={inboundOptions}
                    loading={inboundsQ.isLoading}
                    optionFilterProp="label"
                  />
                </Form.Item>
                <Row gutter={12}>
                  <Col span={12}>
                    <Form.Item name="sortOrder" label={t('pages.sales.sortOrder')}>
                      <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                  </Col>
                  <Col span={12}>
                    <Form.Item name="enable" label={t('pages.sales.onSale')} valuePropName="checked">
                      <Switch />
                    </Form.Item>
                  </Col>
                </Row>
              </Form>
            </Modal>

            <Modal
              title={t('pages.sales.reject')}
              open={!!rejecting}
              onCancel={() => setRejecting(null)}
              onOk={() => {
                if (rejecting) rejectMut.mutate({ id: rejecting.id, note: rejectNote });
                setRejecting(null);
              }}
              okText={t('confirm')}
              cancelText={t('cancel')}
              okButtonProps={{ danger: true }}
              destroyOnHidden
            >
              <Typography.Paragraph>{t('pages.sales.rejectNoteDesc')}</Typography.Paragraph>
              <Input.TextArea rows={3} value={rejectNote} onChange={(e) => setRejectNote(e.target.value)} />
            </Modal>

            <Modal
              title={t('pages.sales.credentialsTitle')}
              open={!!credentials}
              onCancel={() => setCredentials(null)}
              onOk={() => setCredentials(null)}
              okText={t('confirm')}
              cancelText={t('cancel')}
              destroyOnHidden
            >
              <Alert type="success" showIcon message={t('pages.sales.credentialsSent')} style={{ marginBottom: 12 }} />
              <Typography.Paragraph>
                <b>{t('pages.admins.username')}:</b> <Typography.Text code copyable>{credentials?.username}</Typography.Text>
              </Typography.Paragraph>
              <Typography.Paragraph>
                <b>{t('pages.admins.password')}:</b> <Typography.Text code copyable>{credentials?.password}</Typography.Text>
              </Typography.Paragraph>
            </Modal>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
