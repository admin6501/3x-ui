import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router-dom';

import PanelLayout from '@/layouts/PanelLayout';

const IndexPage = lazy(() => import('@/pages/index/IndexPage'));
const InboundsPage = lazy(() => import('@/pages/inbounds/InboundsPage'));
const ClientsPage = lazy(() => import('@/pages/clients/ClientsPage'));
const GroupsPage = lazy(() => import('@/pages/groups/GroupsPage'));
const NodesPage = lazy(() => import('@/pages/nodes/NodesPage'));
const HostsPage = lazy(() => import('@/pages/hosts/HostsPage'));
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'));
const XrayPage = lazy(() => import('@/pages/xray/XrayPage'));
const AdminsPage = lazy(() => import('@/pages/admins/AdminsPage'));
const PlansPage = lazy(() => import('@/pages/plans/PlansPage'));
const ResellerDashboardPage = lazy(() => import('@/pages/reseller/ResellerDashboardPage'));
const ApiDocsPage = lazy(() => import('@/pages/api-docs/ApiDocsPage'));
const TutorialsPage = lazy(() => import('@/pages/tutorials/TutorialsPage'));

function withSuspense(node: React.ReactNode) {
  return <Suspense fallback={null}>{node}</Suspense>;
}

// Resellers are scoped to the Clients section only; send them there instead of
// the dashboard (which is hidden from their sidebar and shows panel-wide stats).
function IndexRoute() {
  const role = (typeof window !== 'undefined' && window.X_UI_ROLE) || 'super_admin';
  if (role === 'reseller') {
    return <Navigate to="/clients" replace />;
  }
  return withSuspense(<IndexPage />);
}

const routes: RouteObject[] = [
  {
    path: '/',
    element: <PanelLayout />,
    children: [
      { index: true, element: <IndexRoute /> },
      { path: 'inbounds', element: withSuspense(<InboundsPage />) },
      { path: 'clients', element: withSuspense(<ClientsPage />) },
      { path: 'groups', element: withSuspense(<GroupsPage />) },
      { path: 'nodes', element: withSuspense(<NodesPage />) },
      { path: 'hosts', element: withSuspense(<HostsPage />) },
      { path: 'settings', element: withSuspense(<SettingsPage />) },
      { path: 'xray', element: withSuspense(<XrayPage />) },
      { path: 'admins', element: withSuspense(<AdminsPage />) },
      { path: 'plans', element: withSuspense(<PlansPage />) },
      { path: 'usage', element: withSuspense(<ResellerDashboardPage />) },
      { path: 'outbound', element: withSuspense(<XrayPage />) },
      { path: 'routing', element: withSuspense(<XrayPage />) },
      { path: 'api-docs', element: withSuspense(<ApiDocsPage />) },
      { path: 'tutorials', element: withSuspense(<TutorialsPage />) },
    ],
  },
];

function computeBasename() {
  const raw = (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '/';
  const trimmed = raw.replace(/\/+$/, '');
  return `${trimmed}/panel`;
}

export const router = createBrowserRouter(routes, {
  basename: computeBasename(),
});
