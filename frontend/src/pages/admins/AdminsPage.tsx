import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Button,
  Card,
  Col,
  ConfigProvider,
  Dropdown,
  Form,
  Input,
  InputNumber,
  Layout,
  Modal,
  Progress,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  DeleteOutlined,
  EditOutlined,
  KeyOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  StopOutlined,
  CheckCircleOutlined,
  TeamOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import RolesCard, { customRoleKey, parseCustomRoleId, useCustomRoles } from './RolesCard';
import '@/styles/page-shell.css';
import './AdminsPage.css';
import OptionButtons from '@/components/form/OptionButtons';

const BUILTIN_ROLES = ['super_admin', 'manager', 'reseller', 'readonly'] as const;
// A role is either one of the built-ins or "custom:<id>", so the select can
// offer both from a single field.
type Role = string;

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

// Quota bars turn amber then red as a reseller approaches their ceiling.
function quotaColor(pct: number): string | undefined {
  if (pct >= 90) return '#ff4d4f';
  if (pct >= 70) return '#faad14';
  return undefined;
}

const GB = 1024 * 1024 * 1024;

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

  const rolesQ = useCustomRoles();
  const customRoles = useMemo(() => rolesQ.data ?? [], [rolesQ.data]);

  // Which roles behave like a reseller — the built-in one plus any custom role
  // holding inbounds.scoped. The inbound/quota fields follow this, not the
  // literal role name.
  const scopedRoles = useMemo(() => {
    const set = new Set<string>(['reseller']);
    for (const r of customRoles) {
      if ((r.permissions || '').split(',').includes('inbounds.scoped')) set.add(customRoleKey(r.id));
    }
    return set;
  }, [customRoles]);

  // Flat rather than grouped under "built-in" / "your roles": the picker is a
  // row of buttons, which has nowhere to put a group header. Built-ins come
  // first and custom roles carry their own names, so the split stays legible
  // without the headings.
  const roleOptions = useMemo<{ value: string; label: string }[]>(() => {
    const builtin = BUILTIN_ROLES.map((r) => ({
      value: r as Role as string,
      label: t(`pages.admins.roles.${r}`, r),
    }));
    return [
      ...builtin,
      ...customRoles.map((r) => ({ value: customRoleKey(r.id), label: r.name })),
    ];
  }, [customRoles, t]);

  // How many admins hold each custom role, so the roles card can warn before a
  // delete that is going to be refused.
  const roleUsage = useMemo(() => {
    const out: Record<number, number> = {};
    for (const row of adminsQ.data ?? []) {
      const id = parseCustomRoleId(row.role);
      if (id) out[id] = (out[id] ?? 0) + 1;
    }
    return out;
  }, [adminsQ.data]);

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
    qc.invalidateQueries({ queryKey: ['adminRoles'] });
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
      const isReseller = scopedRoles.has(values.role);
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

  // Destructive actions moved into the overflow menu, so their confirmation is
  // a modal rather than a popconfirm anchored to a menu item that has already
  // closed by the time the user answers.
  const [confirm, setConfirm] = useState<{ kind: 'disable' | 'delete' | 'resetTraffic'; row: AdminRow } | null>(null);
  const confirmText: Record<string, string> = {
    disable: t('pages.admins.confirmDisable'),
    delete: t('pages.admins.confirmDelete'),
    resetTraffic: t('pages.admins.confirmResetTraffic'),
  };
  const runConfirm = () => {
    if (!confirm) return;
    const { kind, row } = confirm;
    if (kind === 'disable') setEnabledMutation.mutate({ id: row.id, enabled: false });
    if (kind === 'delete') deleteMutation.mutate(row.id);
    if (kind === 'resetTraffic') resetTrafficMutation.mutate(row.id);
    setConfirm(null);
  };

  const roleLabel = (r: string) => {
    const id = parseCustomRoleId(r);
    if (id) return customRoles.find((row) => row.id === id)?.name ?? r;
    return t(`pages.admins.roles.${r}`, r);
  };
  const roleColor = (r: string) => (parseCustomRoleId(r) ? 'purple' : ROLE_COLORS[r] ?? 'default');

  const columns: ColumnsType<AdminRow> = [
    { title: 'ID', dataIndex: 'id', width: 64 },
    {
      title: t('pages.admins.username'),
      dataIndex: 'username',
      render: (username: string) => (
        <Space size={8}>
          <span className="admin-avatar"><UserOutlined /></span>
          <b>{username}</b>
        </Space>
      ),
    },
    {
      title: t('pages.admins.role'),
      dataIndex: 'role',
      render: (role: string) => <Tag color={roleColor(role)}>{roleLabel(role)}</Tag>,
    },
    {
      title: t('pages.admins.status'),
      key: 'status',
      width: 110,
      render: (_: unknown, row: AdminRow) =>
        row.disabled
          ? <Tag color="red">{t('pages.admins.disabled')}</Tag>
          : <Tag color="green">{t('pages.admins.active')}</Tag>,
    },
    {
      title: t('pages.admins.allowedInbounds'),
      dataIndex: 'allowedInbounds',
      render: (csv: string, row: AdminRow) =>
        scopedRoles.has(row.role)
          ? csvToIds(csv).length > 0
            ? <Space size={[4, 4]} wrap>{csvToIds(csv).map((id) => <Tag key={id}>#{id}</Tag>)}</Space>
            : <Typography.Text type="secondary">{t('pages.admins.noInbound')}</Typography.Text>
          : <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: t('pages.admins.usage'),
      key: 'usage',
      width: 260,
      render: (_: unknown, row: AdminRow) => {
        if (!scopedRoles.has(row.role)) return <Typography.Text type="secondary">—</Typography.Text>;
        const st = statsQ.data?.[String(row.id)];
        const used = st?.trafficUsedBytes ?? 0;
        const quotaGB = row.trafficQuotaGB ?? 0;
        const current = st?.currentClients ?? 0;
        const clientQuota = row.clientQuota ?? 0;
        const total = st?.clientsCreatedTotal ?? row.clientsCreatedTotal ?? 0;
        const trafficPct = quotaGB > 0 ? Math.min(100, Math.round((used / (quotaGB * GB)) * 100)) : 0;
        const clientPct = clientQuota > 0 ? Math.min(100, Math.round((current / clientQuota) * 100)) : 0;
        return (
          <div className="admin-usage" data-testid={`reseller-usage-${row.id}`}>
            <div className="admin-usage-row">
              <span className="admin-usage-label">{t('pages.admins.trafficUsed')}</span>
              <span>
                <b>{SizeFormatter.sizeFormat(used)}</b>
                {quotaGB > 0 ? ` / ${quotaGB} GB` : ` / ${t('pages.admins.unlimited')}`}
              </span>
            </div>
            {quotaGB > 0 && <Progress percent={trafficPct} size="small" strokeColor={quotaColor(trafficPct)} />}
            <div className="admin-usage-row">
              <span className="admin-usage-label">{t('pages.admins.currentClients')}</span>
              <span>
                <b>{current}</b>
                {clientQuota > 0 ? ` / ${clientQuota}` : ` / ${t('pages.admins.unlimited')}`}
              </span>
            </div>
            {clientQuota > 0 && <Progress percent={clientPct} size="small" strokeColor={quotaColor(clientPct)} />}
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('pages.admins.totalCreated')}: {total}
            </Typography.Text>
          </div>
        );
      },
    },
    {
      title: t('pages.admins.actions'),
      key: 'actions',
      width: 150,
      align: 'right',
      render: (_: unknown, row: AdminRow) => {
        // Only the two everyday actions get a button; the rest live behind the
        // overflow menu so the column stops wrapping onto three lines.
        const menuItems = [
          { key: 'password', icon: <KeyOutlined />, label: t('pages.admins.resetPassword') },
          ...(scopedRoles.has(row.role)
            ? [{ key: 'resetTraffic', icon: <ReloadOutlined />, label: t('pages.admins.resetTraffic') }]
            : []),
          row.disabled
            ? { key: 'enable', icon: <CheckCircleOutlined />, label: t('pages.admins.enable') }
            : { key: 'disable', icon: <StopOutlined />, label: t('pages.admins.disable'), danger: true },
          { type: 'divider' as const },
          { key: 'delete', icon: <DeleteOutlined />, label: t('delete'), danger: true },
        ];
        const onAction = ({ key }: { key: string }) => {
          switch (key) {
            case 'password':
              openResetPw(row);
              break;
            case 'resetTraffic':
              setConfirm({ kind: 'resetTraffic', row });
              break;
            case 'enable':
              setEnabledMutation.mutate({ id: row.id, enabled: true });
              break;
            case 'disable':
              setConfirm({ kind: 'disable', row });
              break;
            case 'delete':
              setConfirm({ kind: 'delete', row });
              break;
          }
        };
        return (
          <Space size={4}>
            <Tooltip title={t('edit')}>
              <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(row)} />
            </Tooltip>
            <Dropdown menu={{ items: menuItems, onClick: onAction }} trigger={['click']}>
              <Button size="small" icon={<MoreOutlined />} data-testid={`admin-actions-${row.id}`} />
            </Dropdown>
          </Space>
        );
      },
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
            <div className="admins-shell">
      <div className="admins-header">
        <div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            {t('pages.admins.title')}
          </Typography.Title>
          <Typography.Text type="secondary">{t('pages.admins.subtitle')}</Typography.Text>
        </div>
        <Space wrap>
          <Button icon={<ReloadOutlined />} onClick={refresh}>
            {t('pages.admins.refresh')}
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>
            {t('pages.admins.addAdmin')}
          </Button>
        </Space>
      </div>

      <Row gutter={[16, 16]}>
        {([
          ['total', <TeamOutlined key="t" />, t('pages.admins.totalAdmins'), (adminsQ.data ?? []).length, 'blue'],
          ['scoped', <UserOutlined key="s" />, t('pages.admins.scopedAccounts'),
            (adminsQ.data ?? []).filter((r) => scopedRoles.has(r.role)).length, 'green'],
          ['disabled', <StopOutlined key="d" />, t('pages.admins.disabled'),
            (adminsQ.data ?? []).filter((r) => r.disabled).length, 'red'],
          ['roles', <SafetyCertificateOutlined key="r" />, t('pages.admins.customRoles'), customRoles.length, 'purple'],
        ] as const).map(([key, icon, label, value, color]) => (
          <Col xs={12} md={6} key={key}>
            <Card className={`admins-stat is-${color}`} data-testid={`admins-stat-${key}`}>
              <span className="admins-stat-icon">{icon}</span>
              <div>
                <div className="admins-stat-value">{value}</div>
                <div className="admins-stat-label">{label}</div>
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      <Card>
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

      <RolesCard roles={customRoles} loading={rolesQ.isLoading} usageById={roleUsage} />

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
            <OptionButtons options={roleOptions} data-testid="admin-role-select" />
          </Form.Item>
          {scopedRoles.has(watchRole ?? '') && (
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
          {scopedRoles.has(watchRole ?? '') && (
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
        title={confirm ? `${confirm.row.username}` : ''}
        open={!!confirm}
        onCancel={() => setConfirm(null)}
        onOk={runConfirm}
        okText={t('confirm')}
        cancelText={t('cancel')}
        okButtonProps={{ danger: confirm?.kind !== 'resetTraffic' }}
        data-testid="admin-confirm-modal"
        destroyOnHidden
      >
        <Typography.Text>{confirm ? confirmText[confirm.kind] : ''}</Typography.Text>
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
