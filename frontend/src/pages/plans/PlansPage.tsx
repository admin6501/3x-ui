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
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import '@/styles/page-shell.css';

interface Plan {
  id: number;
  name: string;
  durationDays: number;
  totalGB: number;
  limitIp: number;
  reset: number;
  enable: boolean;
  remark: string;
  sortOrder: number;
}

const emptyPlan: Plan = {
  id: 0, name: '', durationDays: 30, totalGB: 0, limitIp: 0, reset: 0, enable: true, remark: '', sortOrder: 0,
};

export default function PlansPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { isDark, isUltra, antdThemeConfig } = useTheme();

  const pageClass = useMemo(() => {
    const classes = ['admins-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Plan | null>(null);
  const [draft, setDraft] = useState<Plan>(emptyPlan);

  const plansQuery = useQuery({
    queryKey: ['plans'],
    queryFn: async () => (await HttpUtil.get<Plan[]>('/panel/api/plans/list', undefined, { silent: true })).obj ?? [],
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: ['plans'] });

  const saveMutation = useMutation({
    mutationFn: async (p: Plan) => {
      const path = p.id ? `/panel/api/plans/update/${p.id}` : '/panel/api/plans/add';
      return HttpUtil.post(path, p);
    },
    onSuccess: () => { setModalOpen(false); invalidate(); },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/plans/del/${id}`),
    onSuccess: invalidate,
  });

  const openCreate = () => { setEditing(null); setDraft(emptyPlan); setModalOpen(true); };
  const openEdit = (p: Plan) => { setEditing(p); setDraft({ ...p }); setModalOpen(true); };

  const columns: ColumnsType<Plan> = [
    { title: t('pages.plans.name'), dataIndex: 'name', key: 'name' },
    {
      title: t('pages.plans.duration'), dataIndex: 'durationDays', key: 'durationDays',
      render: (d: number) => (d > 0 ? `${d} ${t('pages.plans.days')}` : t('pages.plans.noExpiry')),
    },
    {
      title: t('pages.plans.totalGB'), dataIndex: 'totalGB', key: 'totalGB',
      render: (g: number) => (g > 0 ? `${g} GB` : t('pages.plans.unlimited')),
    },
    {
      title: t('pages.plans.limitIp'), dataIndex: 'limitIp', key: 'limitIp',
      render: (n: number) => (n > 0 ? n : t('pages.plans.unlimited')),
    },
    {
      title: t('pages.plans.enable'), dataIndex: 'enable', key: 'enable',
      render: (e: boolean) => <Tag color={e ? 'green' : 'default'}>{e ? t('enabled') : t('disabled')}</Tag>,
    },
    {
      title: t('pages.plans.actions'), key: 'actions', align: 'center', width: 120,
      render: (_: unknown, row: Plan) => (
        <Space>
          <Button data-testid={`plan-edit-${row.id}`} type="text" icon={<EditOutlined />} onClick={() => openEdit(row)} />
          <Popconfirm title={t('pages.plans.confirmDelete')} onConfirm={() => deleteMutation.mutate(row.id)}
            okText={t('confirm')} cancelText={t('cancel')}>
            <Button data-testid={`plan-delete-${row.id}`} type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <Card data-testid="plans-page">
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                  <div>
                    <Typography.Title level={4} style={{ margin: 0 }}>{t('pages.plans.title')}</Typography.Title>
                    <Typography.Text type="secondary">{t('pages.plans.subtitle')}</Typography.Text>
                  </div>
                  <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => invalidate()}>{t('pages.plans.refresh')}</Button>
                    <Button data-testid="plan-add-button" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                      {t('pages.plans.addPlan')}
                    </Button>
                  </Space>
                </div>
                <Table
                  data-testid="plans-table"
                  rowKey="id"
                  loading={plansQuery.isLoading}
                  dataSource={plansQuery.data || []}
                  columns={columns}
                  pagination={false}
                  size="middle"
                />
              </Card>
            </div>
          </Layout.Content>
        </Layout>
      </Layout>

      <Modal
        data-testid="plan-modal"
        open={modalOpen}
        title={editing ? t('pages.plans.editPlan') : t('pages.plans.addPlan')}
        onCancel={() => setModalOpen(false)}
        onOk={() => saveMutation.mutate(draft)}
        confirmLoading={saveMutation.isPending}
        okText={t('confirm')}
        cancelText={t('cancel')}
      >
        <Form layout="vertical">
          <Form.Item label={t('pages.plans.name')} required>
            <Input data-testid="plan-name-input" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </Form.Item>
          <Form.Item label={t('pages.plans.duration')} tooltip={t('pages.plans.durationDesc')}>
            <InputNumber data-testid="plan-duration-input" min={0} style={{ width: '100%' }} value={draft.durationDays}
              onChange={(v) => setDraft({ ...draft, durationDays: Number(v) || 0 })} />
          </Form.Item>
          <Form.Item label={t('pages.plans.totalGB')} tooltip={t('pages.plans.unlimitedHint')}>
            <InputNumber data-testid="plan-totalgb-input" min={0} style={{ width: '100%' }} value={draft.totalGB}
              onChange={(v) => setDraft({ ...draft, totalGB: Number(v) || 0 })} />
          </Form.Item>
          <Form.Item label={t('pages.plans.limitIp')} tooltip={t('pages.plans.unlimitedHint')}>
            <InputNumber data-testid="plan-limitip-input" min={0} style={{ width: '100%' }} value={draft.limitIp}
              onChange={(v) => setDraft({ ...draft, limitIp: Number(v) || 0 })} />
          </Form.Item>
          <Form.Item label={t('pages.plans.reset')} tooltip={t('pages.plans.resetDesc')}>
            <InputNumber data-testid="plan-reset-input" min={0} style={{ width: '100%' }} value={draft.reset}
              onChange={(v) => setDraft({ ...draft, reset: Number(v) || 0 })} />
          </Form.Item>
          <Form.Item label={t('pages.plans.remark')}>
            <Input data-testid="plan-remark-input" value={draft.remark} onChange={(e) => setDraft({ ...draft, remark: e.target.value })} />
          </Form.Item>
          <Form.Item label={t('pages.plans.enable')}>
            <Switch data-testid="plan-enable-switch" checked={draft.enable} onChange={(v) => setDraft({ ...draft, enable: v })} />
          </Form.Item>
        </Form>
      </Modal>
    </ConfigProvider>
  );
}
