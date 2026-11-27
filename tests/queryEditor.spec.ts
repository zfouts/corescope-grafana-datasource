import { test, expect } from '@grafana/plugin-e2e';
import type { Page } from '@playwright/test';

const OBSERVER_ID = '1F445B75F5EB65EEC244A17A1E8660812A2529727A2329DDA37C44684D8841EA';

async function chooseType(page: Page, label: string) {
  const combo = page.locator('#query-editor-type');
  await combo.click();
  // The option list is virtualized: filter it so the option is rendered.
  await combo.fill(label);
  await page.getByRole('option', { name: label }).first().click();
}

test.beforeEach(async ({ panelEditPage, readProvisionedDataSource }) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);
  await panelEditPage.setVisualization('Table');
});

test('Packets per hour returns data', async ({ panelEditPage, page }) => {
  await chooseType(page, 'Packets per hour');
  await expect(panelEditPage.refreshPanel()).toBeOK();
  await expect(panelEditPage.panel.fieldNames).toContainText(['time', 'packets']);
});

test('Observer radio metrics: picker, data and an invalid id', async ({ panelEditPage, page }) => {
  await chooseType(page, 'Observer radio metrics');
  const picker = page.locator('#query-editor-observer');
  await expect(picker).toBeVisible();

  await picker.click();
  await picker.fill('lutin');
  await page.getByRole('option', { name: /lutin-observer-1/ }).first().click();
  await expect(panelEditPage.refreshPanel()).toBeOK();
  await expect(panelEditPage.panel.fieldNames.getByText('noiseFloor', { exact: true })).toBeVisible();

  await picker.click();
  // A well-formed but unknown id is just empty data; a malformed one is a query error.
  await picker.fill('bad/id');
  await page.getByRole('option', { name: /bad\/id/ }).first().click();
  await expect(panelEditPage.panel.getErrorIcon()).toBeVisible();
});

test('Nodes shows role, limit and search and returns a table', async ({ panelEditPage, page }) => {
  await chooseType(page, 'Nodes');
  await expect(page.locator('#query-editor-role')).toBeVisible();
  await expect(page.locator('#query-editor-limit')).toBeVisible();
  await expect(page.locator('#query-editor-search')).toBeVisible();
  await expect(page.locator('#query-editor-observer')).toBeHidden();
  await expect(panelEditPage.refreshPanel()).toBeOK();
  await expect(panelEditPage.panel.fieldNames).toContainText(['name', 'role']);
});

test('Server stats returns totalNodes 200', async ({ panelEditPage, page }) => {
  await chooseType(page, 'Server stats');
  await expect(panelEditPage.refreshPanel()).toBeOK();
  await expect(panelEditPage.panel.fieldNames.getByText('totalNodes', { exact: true })).toBeVisible();
  // Column order: totalPackets, totalTransmissions, totalObservations, totalNodes.
  await expect(panelEditPage.panel.data.nth(3)).toHaveText('200');
});
