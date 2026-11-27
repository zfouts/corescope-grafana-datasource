import { test, expect } from '@grafana/plugin-e2e';

test('overview dashboard renders without panel errors', async ({ gotoDashboardPage, readProvisionedDashboard }) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'corescope-overview.json' });
  const dashboardPage = await gotoDashboardPage({ uid: dashboard.uid });
  // Wait for data before asserting there are no errors.
  await expect(dashboardPage.getPanelByTitle('Payload types').locator).toContainText('GRP_TXT');
  await expect(dashboardPage).not.toHavePanelErrors();
});

test('Nodes stat shows 200 and Payload types lists GRP_TXT', async ({ page, gotoDashboardPage, readProvisionedDashboard }) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'corescope-overview.json' });
  const dashboardPage = await gotoDashboardPage({ uid: dashboard.uid });
  // Two panels are titled "Nodes" (the stat and the inventory table); the stat comes first.
  await expect(page.getByTestId(/^data-testid Panel header Nodes$/).first()).toContainText('200');
  await expect(dashboardPage.getPanelByTitle('Payload types').locator).toContainText('GRP_TXT');
});
