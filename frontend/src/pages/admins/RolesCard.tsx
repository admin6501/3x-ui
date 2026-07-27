import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, Checkbox, Empty, Form, Input, Modal, Popconfirm, Space, Table, Tag, Tooltip, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, EditOutlined, PlusOutlined, SafetyCertificateOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { PERMISSIONS, type Permission } from '@/lib/permissions';

export interface CustomRole {
  id: number;
  name: string;
  permissions: string;
  createdAt: number;
}

/** Role key stored on a user for a custom role. */
export function customRoleKey(id: number): string {
  return `custom:${id}`;
}

export function parseCustomRoleId(role: string): number | null {
  if (!role?.startsWith('custom:')) return null;
  const id = parseInt(role.slice('custom:'.length), 10);
  return Number.isFinite(id) && id > 0 ? id : null;
}

function permList(role: CustomRole): string[] {
  return (role.permissions || '').split(',').map((s) => s.trim()).filter(Boolean);
}

interface Props {
  roles: CustomRole[];
  loading: boolean;
  /** How many admin accounts hold each custom role, keyed by role id. */
  usageById: Record<number, number>;
}

export default function RolesCard({ roles, loading, usageById }: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();

  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<CustomRole | null>(null);
  const [form] = Form.useForm<{ name: string; permissions: Permission[] }>();

  const permLabel = (p: string) => t(`pages.admins.permissionNames.${p}`, p);
  const permHint = (p: string) => {
    const key = `pages.admins.permissionHints.${p}`;
    const text = t(key, '');
    return text === key ? '' : text;
  };

  const openAdd = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ name: '', permissions: [] });
    setOpen(true);
  };

  const openEdit = (role: CustomRole) => {
    setEditing(role);
    form.setFieldsValue({ name: role.name, permissions: permList(role) as Permission[] });
    setOpen(true);
  };

  const refresh = () => qc.invalidateQueries({ queryKey: ['adminRoles'] });

  const saveMut = useMutation({
    mutationFn: async (values: { name: string; permissions: Permission[] }) => {
      const body = { name: values.name, permissions: values.permissions ?? [] };
      return editing
        ? HttpUtil.post(`/panel/api/roles/update/${editing.id}`, body)
        : HttpUtil.post('/panel/api/roles/add', body);
    },
    onSuccess: (res) => {
      if (res.success) {
        setOpen(false);
        refresh();
      }
    },
  });

  const deleteMut = useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/roles/del/${id}`),
    onSuccess: () => refresh(),
  });

  const columns: ColumnsType<CustomRole> = useMemo(() => [
    {
      title: t('pages.admins.roleName'),
      dataIndex: 'name',
      render: (name: string, row) => (
        <Space direction="vertical" size={0}>
          <Tag color="purple">{name}</Tag>
          {(usageById[row.id] ?? 0) > 0 && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('pages.admins.roleInUse', { count: usageById[row.id] })}
            </Typography.Text>
          )}
        </Space>
      ),
    },
    {
      title: t('pages.admins.permissions'),
      dataIndex: 'permissions',
      render: (_: string, row) => {
        const list = permList(row);
        if (list.length === 0) return <Typography.Text type="secondary">—</Typography.Text>;
        return (
          <Space size={[4, 4]} wrap>
            {list.map((p) => (
              <Tag key={p} color={p === 'inbounds.scoped' ? 'gold' : undefined}>
                {permLabel(p)}
              </Tag>
            ))}
          </Space>
        );
      },
    },
    {
      title: t('pages.admins.actions'),
      key: 'actions',
      width: 150,
      align: 'right',
      render: (_: unknown, row) => (
        <Space size={4}>
          <Tooltip title={t('edit')}>
            <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(row)} />
          </Tooltip>
          <Popconfirm
            title={t('pages.admins.confirmDeleteRole')}
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [t, usageById, deleteMut.isPending]);

  return (
    <Card
      data-testid="roles-card"
      title={
        <Space>
          <SafetyCertificateOutlined />
          <span>{t('pages.admins.customRoles')}</span>
        </Space>
      }
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openAdd} data-testid="add-role">
          {t('pages.admins.addRole')}
        </Button>
      }
    >
      <Typography.Paragraph type="secondary" style={{ marginTop: -8 }}>
        {t('pages.admins.customRolesDesc')}
      </Typography.Paragraph>

      {roles.length > 0 ? (
        <Table<CustomRole>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={roles}
          columns={columns}
          pagination={false}
          scroll={{ x: 'max-content' }}
        />
      ) : (
        <Empty description={t('pages.admins.noRoles')} image={Empty.PRESENTED_IMAGE_SIMPLE} />
      )}

      <Modal
        title={editing ? t('pages.admins.editRole') : t('pages.admins.addRole')}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMut.isPending}
        okText={t('confirm')}
        cancelText={t('cancel')}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveMut.mutate(v)} preserve={false}>
          <Form.Item name="name" label={t('pages.admins.roleName')} rules={[{ required: true }]}>
            <Input placeholder={t('pages.admins.roleNamePlaceholder')} autoComplete="off" data-testid="role-name-input" />
          </Form.Item>
          <Form.Item name="permissions" label={t('pages.admins.permissions')}>
            <Checkbox.Group style={{ width: '100%' }}>
              <Space direction="vertical" size={6} style={{ width: '100%' }}>
                {PERMISSIONS.map((p) => (
                  <Checkbox key={p} value={p} data-testid={`perm-${p}`}>
                    {permLabel(p)}
                    {permHint(p) && (
                      <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12 }}>
                        {permHint(p)}
                      </Typography.Text>
                    )}
                  </Checkbox>
                ))}
              </Space>
            </Checkbox.Group>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}

/** Shared query for the custom-role list, used by the page and the card. */
export function useCustomRoles() {
  return useQuery({
    queryKey: ['adminRoles'],
    queryFn: async () =>
      (await HttpUtil.get<CustomRole[]>('/panel/api/roles/list', undefined, { silent: true })).obj ?? [],
  });
}
