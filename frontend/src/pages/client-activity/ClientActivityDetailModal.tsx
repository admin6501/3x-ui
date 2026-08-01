import { useTranslation } from 'react-i18next';
import { Badge, Modal, Space, Table, Tabs, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';

import { IntlUtil } from '@/utils';
import type { ClientActivityDetail, ClientActivityIP, ClientActivityVisit } from '@/generated/types';

interface Props {
  open: boolean;
  loading: boolean;
  detail: ClientActivityDetail | null;
  onClose: () => void;
}

export default function ClientActivityDetailModal({ open, loading, detail, onClose }: Props) {
  const { t } = useTranslation();

  const ipColumns: ColumnsType<ClientActivityIP> = [
    { title: t('pages.clientActivity.col.ip'), dataIndex: 'ip', key: 'ip' },
    {
      title: t('pages.clientActivity.col.operator'),
      key: 'isp',
      render: (_: unknown, r) => r.isp || <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: t('pages.clientActivity.col.country'),
      key: 'country',
      render: (_: unknown, r) =>
        r.country ? <Tag>{r.countryCode ? `${r.countryCode} · ${r.country}` : r.country}</Tag> : '—',
    },
    { title: t('pages.clientActivity.col.hits'), dataIndex: 'hits', key: 'hits', align: 'center' },
    {
      title: t('pages.clientActivity.col.lastSeen'),
      dataIndex: 'lastSeen',
      key: 'lastSeen',
      render: (ts: number) => (ts > 0 ? IntlUtil.formatDate(ts * 1000) : '—'),
    },
  ];

  const visitColumns: ColumnsType<ClientActivityVisit> = [
    { title: t('pages.clientActivity.col.site'), dataIndex: 'dest', key: 'dest' },
    {
      title: t('pages.clientActivity.col.network'),
      dataIndex: 'network',
      key: 'network',
      align: 'center',
      render: (n: string) => <Tag>{n || 'tcp'}</Tag>,
    },
    { title: t('pages.clientActivity.col.hits'), dataIndex: 'hits', key: 'hits', align: 'center' },
    {
      title: t('pages.clientActivity.col.lastSeen'),
      dataIndex: 'lastSeen',
      key: 'lastSeen',
      render: (ts: number) => (ts > 0 ? IntlUtil.formatDate(ts * 1000) : '—'),
    },
  ];

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width={760}
      title={
        detail ? (
          <Space>
            <Badge status={detail.online ? 'success' : 'default'} />
            {detail.email}
          </Space>
        ) : (
          t('menu.clientActivity')
        )
      }
    >
      <Tabs
        items={[
          {
            key: 'ips',
            label: `${t('pages.clientActivity.tab.ips')} (${detail?.ips?.length ?? 0})`,
            children: (
              <Table<ClientActivityIP>
                rowKey="ip"
                size="small"
                loading={loading}
                columns={ipColumns}
                dataSource={detail?.ips ?? []}
                pagination={{ pageSize: 10, hideOnSinglePage: true }}
                scroll={{ x: 'max-content' }}
              />
            ),
          },
          {
            key: 'sites',
            label: `${t('pages.clientActivity.tab.sites')} (${detail?.visits?.length ?? 0})`,
            children: (
              <Table<ClientActivityVisit>
                rowKey="dest"
                size="small"
                loading={loading}
                columns={visitColumns}
                dataSource={detail?.visits ?? []}
                pagination={{ pageSize: 10, hideOnSinglePage: true }}
                scroll={{ x: 'max-content' }}
              />
            ),
          },
        ]}
      />
    </Modal>
  );
}
