import { useMemo } from 'react';
import { Card, Collapse, ConfigProvider, Layout, Typography } from 'antd';
import type { CollapseProps } from 'antd';
import { useTranslation } from 'react-i18next';
import { ReadOutlined } from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';

const { Title, Paragraph } = Typography;

// Section ids rendered on the Tutorials page. Each id maps to
// `pages.tutorials.sections.<id>.title` and `.body` in every locale JSON.
const SECTION_IDS = [
  'overview',
  'dashboard',
  'inbounds',
  'clients',
  'plans',
  'groups',
  'nodes',
  'hosts',
  'subscription',
  'xray',
  'admins',
  'security',
] as const;

/**
 * TutorialsPage renders a static, fully-translated quick-start guide covering
 * every major area of the panel. Content lives in the i18n resources so it is
 * available in all 13 supported UI languages. Visible in the sidebar to
 * super_admins only (see AppSidebar role gating).
 */
export default function TutorialsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();

  const pageClass = useMemo(() => {
    const classes = ['tutorials-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const items = useMemo<CollapseProps['items']>(
    () =>
      SECTION_IDS.map((id) => ({
        key: id,
        label: t(`pages.tutorials.sections.${id}.title`),
        children: (
          <Paragraph style={{ margin: 0, whiteSpace: 'pre-line' }}>
            {t(`pages.tutorials.sections.${id}.body`)}
          </Paragraph>
        ),
      })),
    [t],
  );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Card>
              <Typography>
                <Title level={3} style={{ marginTop: 0 }}>
                  <ReadOutlined style={{ marginInlineEnd: 8 }} />
                  {t('pages.tutorials.title')}
                </Title>
                <Paragraph type="secondary">{t('pages.tutorials.subtitle')}</Paragraph>
              </Typography>
              <Collapse items={items} defaultActiveKey={['overview']} accordion />
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
