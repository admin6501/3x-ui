import { useMemo, useState } from 'react';
import { Card, Collapse, ConfigProvider, Input, Layout, Tag, Typography } from 'antd';
import type { CollapseProps } from 'antd';
import { useTranslation } from 'react-i18next';
import {
  ApiOutlined,
  AppstoreOutlined,
  BgColorsOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  GlobalOutlined,
  ReadOutlined,
  RocketOutlined,
  SafetyOutlined,
  SearchOutlined,
  SolutionOutlined,
  TagsOutlined,
  TeamOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import './TutorialsPage.css';

const { Title, Paragraph, Text } = Typography;

// Sections rendered on the Tutorials page, in reading order: first steps, then
// the day-to-day objects, then the features built on top of them, then the
// admin/maintenance topics. Each id maps to
// `pages.tutorials.sections.<id>.title` / `.body` in every locale JSON.
const SECTIONS: { id: string; icon: ReactNode }[] = [
  { id: 'overview', icon: <RocketOutlined /> },
  { id: 'dashboard', icon: <DashboardOutlined /> },
  { id: 'inbounds', icon: <ApiOutlined /> },
  { id: 'clients', icon: <TeamOutlined /> },
  { id: 'plans', icon: <SolutionOutlined /> },
  { id: 'groups', icon: <TagsOutlined /> },
  { id: 'hosts', icon: <GlobalOutlined /> },
  { id: 'subscription', icon: <CloudServerOutlined /> },
  { id: 'branding', icon: <BgColorsOutlined /> },
  { id: 'limits', icon: <ThunderboltOutlined /> },
  { id: 'cleanup', icon: <DeleteOutlined /> },
  { id: 'nodes', icon: <ClusterOutlined /> },
  { id: 'backup', icon: <DatabaseOutlined /> },
  { id: 'xray', icon: <AppstoreOutlined /> },
  { id: 'admins', icon: <SolutionOutlined /> },
  { id: 'security', icon: <SafetyOutlined /> },
];

// Bodies are authored as plain text so translators work with prose, not markup.
// A line starting with "- " becomes a bullet; everything else is a paragraph.
// A translation that stays a single line still renders correctly.
function renderBody(body: string): ReactNode {
  const blocks: ReactNode[] = [];
  let bullets: string[] = [];

  const flushBullets = () => {
    if (bullets.length === 0) return;
    blocks.push(
      <ul className="tutorial-bullets" key={`ul-${blocks.length}`}>
        {bullets.map((item, i) => <li key={i}>{item}</li>)}
      </ul>,
    );
    bullets = [];
  };

  for (const rawLine of body.split('\n')) {
    const line = rawLine.trim();
    if (!line) continue;
    if (line.startsWith('- ')) {
      bullets.push(line.slice(2).trim());
      continue;
    }
    flushBullets();
    blocks.push(<p className="tutorial-paragraph" key={`p-${blocks.length}`}>{line}</p>);
  }
  flushBullets();
  return blocks;
}

/**
 * TutorialsPage renders a fully-translated guide covering every major area of
 * the panel. Content lives in the i18n resources so it is available in all 13
 * supported UI languages. Visible in the sidebar to super_admins only (see
 * AppSidebar role gating).
 */
export default function TutorialsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [query, setQuery] = useState('');

  const pageClass = useMemo(() => {
    const classes = ['tutorials-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  // Searching across title *and* body lets an admin find the section that
  // mentions a setting without knowing which chapter it lives in.
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const withText = SECTIONS.map((section) => ({
      ...section,
      title: t(`pages.tutorials.sections.${section.id}.title`),
      body: t(`pages.tutorials.sections.${section.id}.body`),
    }));
    if (!needle) return withText;
    return withText.filter(
      (s) => s.title.toLowerCase().includes(needle) || s.body.toLowerCase().includes(needle),
    );
  }, [query, t]);

  const items = useMemo<CollapseProps['items']>(
    () =>
      visible.map((section) => ({
        key: section.id,
        label: (
          <span className="tutorial-section-label">
            <span className="tutorial-section-icon">{section.icon}</span>
            {section.title}
          </span>
        ),
        children: <div className="tutorial-body">{renderBody(section.body)}</div>,
      })),
    [visible],
  );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Card className="tutorials-card">
              <div className="tutorials-header">
                <Typography>
                  <Title level={3} style={{ marginTop: 0, marginBottom: 4 }}>
                    <ReadOutlined style={{ marginInlineEnd: 8 }} />
                    {t('pages.tutorials.title')}
                  </Title>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    {t('pages.tutorials.subtitle')}
                  </Paragraph>
                </Typography>
                <Input
                  allowClear
                  className="tutorials-search"
                  prefix={<SearchOutlined />}
                  placeholder={t('pages.tutorials.searchPlaceholder')}
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
              </div>

              {items && items.length > 0 ? (
                <Collapse
                  items={items}
                  defaultActiveKey={['overview']}
                  className="tutorials-collapse"
                  bordered={false}
                />
              ) : (
                <div className="tutorials-empty">
                  <Text type="secondary">{t('pages.tutorials.noResults')}</Text>
                </div>
              )}

              <div className="tutorials-footer">
                <Tag color="blue">{t('pages.tutorials.footerTip')}</Tag>
              </div>
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
