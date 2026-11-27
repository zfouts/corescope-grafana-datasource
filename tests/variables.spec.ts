import { test, expect } from '@grafana/plugin-e2e';

test('observer and region variables load their options', async ({
  page,
  gotoDashboardPage,
  readProvisionedDashboard,
}) => {
  const dashboard = await readProvisionedDashboard({ fileName: 'corescope-overview.json' });
  await gotoDashboardPage({ uid: dashboard.uid });

  const observer = page.getByRole('combobox', { name: 'Observer', exact: true });
  await expect(observer).toBeVisible();
  await observer.click();
  // The list is virtualized: filter it so the option is rendered.
  await observer.fill('lutin');
  await expect(page.getByRole('option', { name: 'lutin-observer-1' })).toBeVisible();
  await page.keyboard.press('Escape');

  const region = page.getByRole('combobox', { name: 'Region', exact: true });
  await region.click();
  for (const r of ['MRY', 'OAK', 'SFO', 'SJC']) {
    await expect(page.getByRole('option', { name: r, exact: true })).toBeVisible();
  }
});
