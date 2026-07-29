import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router';

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
const ShopPage = lazy(() => import('@/pages/shop/ShopPage'));
const ApiDocsPage = lazy(() => import('@/pages/api-docs/ApiDocsPage'));
const TutorialsPage = lazy(() => import('@/pages/tutorials/TutorialsPage'));

function withSuspense(node: React.ReactNode) {
  return <Suspense fallback={null}>{node}</Suspense>;
}

// The panel-wide dashboard shows stats a reseller is not allowed to see, so it
// is hidden from their sidebar; send them to their own dashboard instead.
function IndexRoute() {
  const role = (typeof window !== 'undefined' && window.X_UI_ROLE) || 'super_admin';
  if (role === 'reseller') {
    return <Navigate to="/usage" replace />;
  }
  return withSuspense(<IndexPage />);
}

// Tutorials is a super_admin-only page. Even though the sidebar hides the entry
// for other roles, guard the route so a non-super_admin hitting /panel/tutorials
// directly (typed URL / bookmark) is redirected to their landing page instead.
function TutorialsRoute() {
  const role = (typeof window !== 'undefined' && window.X_UI_ROLE) || 'super_admin';
  if (role !== 'super_admin') {
    return <Navigate to="/" replace />;
  }
  return withSuspense(<TutorialsPage />);
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
      { path: 'shop', element: withSuspense(<ShopPage />) },
      { path: 'usage', element: withSuspense(<ResellerDashboardPage />) },
      { path: 'outbound', element: withSuspense(<XrayPage />) },
      { path: 'routing', element: withSuspense(<XrayPage />) },
      { path: 'api-docs', element: withSuspense(<ApiDocsPage />) },
      { path: 'tutorials', element: <TutorialsRoute /> },
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
