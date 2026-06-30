import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  ConfigProvider,
  Form,
  Input,
  InputNumber,
  Layout,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  DeleteOutlined,
  EditOutlined,
  KeyOutlined,
  PlusOutlined,
  ReloadOutlined,
  StopOutlined,
  CheckCircleOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import '@/styles/page-shell.css';

const ROLES = ['super_admin', 'manager', 'reseller', 'readonly'] as const;
type Role = (typeof ROLES)[number];

interface AdminRow {
  id: number;
  username: string;
  role: string;
  allowedInbounds: string;
  trafficQuotaGB: number;
  clientQuota: number;
  clientsCreatedTotal: number;
  disabled?: boolean;
}

interface ResellerStat {
  trafficUsedBytes: number;
  currentClients: number;
  clientsCreatedTotal: number;
  trafficQuotaGB: number;
  clientQuota: number;
}

interface AuditRow {
  id: number;
  actor: string;
  action: string;
  target: string;
  details: string;
  createdAt: number;
}

interface InboundOption {
  id: number;
  remark: string;
  protocol: string;
  port: number;
}

const ROLE_COLORS: Record<string, string> = {
  super_admin: 'red',
  manager: 'blue',
  reseller: 'green',
  readonly: 'default',
};

function csvToIds(csv: string): number[] {
  if (!csv) return [];
  return csv
    .split(',')
    .map((s) => parseInt(s.trim(), 10))
    .filter((n) => Number.isFinite(n) && n > 0);
}

interface FormValues {
  username: string;
  password?: string;
  role: Role;
  allowedInbounds: number[];
  trafficQuotaGB?: number;
  clientQuota?: number;
}

export default function AdminsPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { isDark, isUltra, antdThemeConfig } = useTheme();

  const pageClass = useMemo(() => {
    const classes = ['admins-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const adminsQ = useQuery({
    queryKey: ['admins'],
    queryFn: async () => (await HttpUtil.get<AdminRow[]>('/panel/api/admin/list', undefined, { silent: true })).obj ?? [],
  });
  const auditQ = useQuery({
    queryKey: ['adminAudit'],
    queryFn: async () => (await HttpUtil.get<AuditRow[]>('/panel/api/admin/auditLog', undefined, { silent: true })).obj ?? [],
  });
  const inboundsQ = useQuery({
    queryKey: ['inboundOptions'],
    queryFn: async () => (await HttpUtil.get<InboundOption[]>('/panel/api/inbounds/options', undefined, { silent: true })).obj ?? [],
  });
  const statsQ = useQuery({
    queryKey: ['resellerStats'],
    queryFn: async () =>
      (await HttpUtil.get<Record<string, ResellerStat>>('/panel/api/admin/resellerStats', undefined, { silent: true })).obj ?? {},
    refetchInterval: 30000,
  });

  const inboundOptions = useMemo(
    () =>
      (inboundsQ.data ?? []).map((ib) => ({
        value: ib.id,
        label: `#${ib.id} · ${ib.remark || ib.protocol} (:${ib.port})`,
      })),
    [inboundsQ.data],
  );

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['admins'] });
    qc.invalidateQueries({ queryKey: ['adminAudit'] });
    qc.invalidateQueries({ queryKey: ['resellerStats'] });
  };

  // ---- create / edit modal state ----
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<AdminRow | null>(null);
  const [form] = Form.useForm<FormValues>();
  const watchRole = Form.useWatch('role', form);

  const openAdd = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ role: 'manager', allowedInbounds: [], trafficQuotaGB: 0, clientQuota: 0 });
    setModalOpen(true);
  };
  const openEdit = (row: AdminRow) => {
    setEditing(row);
    form.setFieldsValue({
      username: row.username,
      role: row.role as Role,
      allowedInbounds: csvToIds(row.allowedInbounds),
      trafficQuotaGB: row.trafficQuotaGB ?? 0,
      clientQuota: row.clientQuota ?? 0,
    });
    setModalOpen(true);
  };

  const saveMutation = useMutation({
    mutationFn: async (values: FormValues) => {
      const isReseller = values.role === 'reseller';
      const allowed = isReseller ? (values.allowedInbounds ?? []).join(',') : '';
      const trafficQuotaGB = isReseller ? (values.trafficQuotaGB ?? 0) : 0;
      const clientQuota = isReseller ? (values.clientQuota ?? 0) : 0;
      if (editing) {
        return HttpUtil.post(`/panel/api/admin/update/${editing.id}`, {
          username: values.username,
          role: values.role,
          allowedInbounds: allowed,
          trafficQuotaGB,
          clientQuota,
        });
      }
      return HttpUtil.post('/panel/api/admin/add', {
        username: values.username,
        password: values.password,
        role: values.role,
        allowedInbounds: allowed,
        trafficQuotaGB,
        clientQuota,
      });
    },
    onSuccess: (res) => {
      if (res.success) {
        setModalOpen(false);
        refresh();
      }
    },
  });

  // ---- reset password modal ----
  const [pwOpen, setPwOpen] = useState(false);
  const [pwTarget, setPwTarget] = useState<AdminRow | null>(null);
  const [pwForm] = Form.useForm<{ password: string }>();
  const openResetPw = (row: AdminRow) => {
    setPwTarget(row);
    pwForm.resetFields();
    setPwOpen(true);
  };
  const resetPwMutation = useMutation({
    mutationFn: async (values: { password: string }) => {
      if (!pwTarget) return null;
      return HttpUtil.post(`/panel/api/admin/resetPassword/${pwTarget.id}`, { password: values.password });
    },
    onSuccess: (res) => {
      if (res?.success) {
        setPwOpen(false);
        refresh();
      }
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/admin/delete/${id}`),
    onSuccess: () => refresh(),
  });

  const setEnabledMutation = useMutation({
    mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) =>
      HttpUtil.post(`/panel/api/admin/setEnabled/${id}`, { enabled }),
    onSuccess: () => refresh(),
  });

  const resetTrafficMutation = useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/admin/resetResellerTraffic/${id}`),
    onSuccess: () => refresh(),
  });

  const roleLabel = (r: string) => t(`pages.admins.roles.${r}`, r);

  const columns: ColumnsType<AdminRow> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: t('pages.admins.username'),
      dataIndex: 'username',
      render: (username: string, row: AdminRow) => (
        <Space size={6}>
          <span>{username}</span>
          {row.disabled && <Tag color="red">{t('pages.admins.disabled')}</Tag>}
        </Space>
      ),
    },
    {
      title: t('pages.admins.role'),
      dataIndex: 'role',
      render: (role: string) => <Tag color={ROLE_COLORS[role] ?? 'default'}>{roleLabel(role)}</Tag>,
    },
    {
      title: t('pages.admins.allowedInbounds'),
      dataIndex: 'allowedInbounds',
      render: (csv: string, row: AdminRow) =>
        row.role === 'reseller'
          ? csvToIds(csv).length > 0
            ? csvToIds(csv).map((id) => <Tag key={id}>#{id}</Tag>)
            : <Typography.Text type="secondary">{t('pages.admins.noInbound')}</Typography.Text>
          : <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: t('pages.admins.usage'),
      key: 'usage',
      width: 260,
      render: (_: unknown, row: AdminRow) => {
        if (row.role !== 'reseller') return <Typography.Text type="secondary">—</Typography.Text>;
        const st = statsQ.data?.[String(row.id)];
        const used = st?.trafficUsedBytes ?? 0;
        const quotaGB = row.trafficQuotaGB ?? 0;
        const current = st?.currentClients ?? 0;
        const clientQuota = row.clientQuota ?? 0;
        const total = st?.clientsCreatedTotal ?? row.clientsCreatedTotal ?? 0;
        return (
          <Space direction="vertical" size={2} data-testid={`reseller-usage-${row.id}`}>
            <Typography.Text>
              {t('pages.admins.trafficUsed')}: <b>{SizeFormatter.sizeFormat(used)}</b>
              {quotaGB > 0 ? ` / ${quotaGB} GB` : ` / ${t('pages.admins.unlimited')}`}
            </Typography.Text>
            <Typography.Text>
              {t('pages.admins.currentClients')}: <b>{current}</b>
              {clientQuota > 0 ? ` / ${clientQuota}` : ` / ${t('pages.admins.unlimited')}`}
            </Typography.Text>
            <Typography.Text type="secondary">
              {t('pages.admins.totalCreated')}: {total}
            </Typography.Text>
          </Space>
        );
      },
    },
    {
      title: t('pages.admins.actions'),
      key: 'actions',
      width: 320,
      render: (_: unknown, row: AdminRow) => (
        <Space wrap>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(row)}>
            {t('edit')}
          </Button>
          <Button size="small" icon={<KeyOutlined />} onClick={() => openResetPw(row)}>
            {t('pages.admins.resetPassword')}
          </Button>
          {row.role === 'reseller' && (
            <Popconfirm
              title={t('pages.admins.confirmResetTraffic')}
              onConfirm={() => resetTrafficMutation.mutate(row.id)}
              okText={t('confirm')}
              cancelText={t('cancel')}
            >
              <Button size="small" icon={<ReloadOutlined />} loading={resetTrafficMutation.isPending}>
                {t('pages.admins.resetTraffic')}
              </Button>
            </Popconfirm>
          )}
          {row.disabled ? (
            <Button
              size="small"
              icon={<CheckCircleOutlined />}
              onClick={() => setEnabledMutation.mutate({ id: row.id, enabled: true })}
              loading={setEnabledMutation.isPending}
            >
              {t('pages.admins.enable')}
            </Button>
          ) : (
            <Popconfirm
              title={t('pages.admins.confirmDisable')}
              onConfirm={() => setEnabledMutation.mutate({ id: row.id, enabled: false })}
              okText={t('confirm')}
              cancelText={t('cancel')}
            >
              <Button size="small" danger icon={<StopOutlined />} loading={setEnabledMutation.isPending}>
                {t('pages.admins.disable')}
              </Button>
            </Popconfirm>
          )}
          <Popconfirm
            title={t('pages.admins.confirmDelete')}
            onConfirm={() => deleteMutation.mutate(row.id)}
            okText={t('confirm')}
            cancelText={t('cancel')}
          >
            <Button size="small" danger icon={<DeleteOutlined />}>
              {t('delete')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const auditColumns: ColumnsType<AuditRow> = [
    {
      title: t('pages.admins.when'),
      dataIndex: 'createdAt',
      width: 200,
      render: (ms: number) => (ms ? new Date(ms).toLocaleString() : '—'),
    },
    { title: t('pages.admins.actor'), dataIndex: 'actor', render: (v: string) => v || '—' },
    { title: t('pages.admins.action'), dataIndex: 'action', render: (v: string) => <Tag>{v}</Tag> },
    { title: t('pages.admins.target'), dataIndex: 'target', render: (v: string) => v || '—' },
    { title: t('pages.admins.details'), dataIndex: 'details', render: (v: string) => v || '—' },
  ];

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card
        title={
          <div>
            <Typography.Title level={4} style={{ margin: 0 }}>
              {t('pages.admins.title')}
            </Typography.Title>
            <Typography.Text type="secondary">{t('pages.admins.subtitle')}</Typography.Text>
          </div>
        }
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={refresh}>
              {t('pages.admins.refresh')}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>
              {t('pages.admins.addAdmin')}
            </Button>
          </Space>
        }
      >
        <Table<AdminRow>
          rowKey="id"
          size="middle"
          loading={adminsQ.isLoading}
          dataSource={adminsQ.data ?? []}
          columns={columns}
          pagination={false}
          scroll={{ x: 'max-content' }}
        />
      </Card>

      <Card title={t('pages.admins.auditLog')}>
        <Table<AuditRow>
          rowKey="id"
          size="small"
          loading={auditQ.isLoading}
          dataSource={auditQ.data ?? []}
          columns={auditColumns}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
          scroll={{ x: 'max-content' }}
        />
      </Card>

      <Modal
        title={editing ? t('pages.admins.editAdmin') : t('pages.admins.addAdmin')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMutation.isPending}
        okText={t('confirm')}
        cancelText={t('cancel')}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveMutation.mutate(v)} preserve={false}>
          <Form.Item
            name="username"
            label={t('pages.admins.username')}
            rules={[{ required: true }]}
          >
            <Input autoComplete="off" />
          </Form.Item>
          {!editing && (
            <Form.Item
              name="password"
              label={t('pages.admins.password')}
              rules={[{ required: true }]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
          )}
          <Form.Item name="role" label={t('pages.admins.role')} rules={[{ required: true }]}>
            <Select
              options={ROLES.map((r) => ({ value: r, label: roleLabel(r) }))}
            />
          </Form.Item>
          {watchRole === 'reseller' && (
            <Form.Item
              name="allowedInbounds"
              label={t('pages.admins.allowedInbounds')}
              extra={t('pages.admins.allowedInboundsDesc')}
            >
              <Select
                mode="multiple"
                allowClear
                placeholder={t('pages.admins.selectInbounds')}
                options={inboundOptions}
                loading={inboundsQ.isLoading}
                optionFilterProp="label"
              />
            </Form.Item>
          )}
          {watchRole === 'reseller' && (
            <Space size="large" style={{ display: 'flex' }}>
              <Form.Item
                name="trafficQuotaGB"
                label={t('pages.admins.trafficQuota')}
                extra={t('pages.admins.quotaDesc')}
                style={{ flex: 1 }}
              >
                <InputNumber min={0} style={{ width: '100%' }} data-testid="admin-traffic-quota-input" />
              </Form.Item>
              <Form.Item
                name="clientQuota"
                label={t('pages.admins.clientQuota')}
                extra={t('pages.admins.quotaDesc')}
                style={{ flex: 1 }}
              >
                <InputNumber min={0} style={{ width: '100%' }} data-testid="admin-client-quota-input" />
              </Form.Item>
            </Space>
          )}
        </Form>
      </Modal>

      <Modal
        title={`${t('pages.admins.resetPassword')}${pwTarget ? ` — ${pwTarget.username}` : ''}`}
        open={pwOpen}
        onCancel={() => setPwOpen(false)}
        onOk={() => pwForm.submit()}
        confirmLoading={resetPwMutation.isPending}
        okText={t('confirm')}
        cancelText={t('cancel')}
        destroyOnHidden
      >
        <Form form={pwForm} layout="vertical" onFinish={(v) => resetPwMutation.mutate(v)} preserve={false}>
          <Form.Item name="password" label={t('pages.admins.newPassword')} rules={[{ required: true }]}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
