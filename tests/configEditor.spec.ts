import { test, expect } from '@grafana/plugin-e2e';

const DS_TYPE = 'zfouts-corescope-datasource';

test('Save & test succeeds against the CoreScope server', async ({ createDataSourceConfigPage, page }) => {
  const configPage = await createDataSourceConfigPage({ type: DS_TYPE });
  await page.getByRole('textbox', { name: 'URL' }).fill('http://corescope:3000');
  await expect(configPage.saveAndTest()).toBeOK();
  await expect(configPage).toHaveAlert('success', { hasText: 'Connected to CoreScope v3.18.0: 200 nodes, 31 observers' });
});

test('a URL with a user name and password is rejected', async ({ createDataSourceConfigPage, page }) => {
  const configPage = await createDataSourceConfigPage({ type: DS_TYPE });
  await page.getByRole('textbox', { name: 'URL' }).fill('http://user:pass@corescope:3000');
  await expect(page.getByText('Do not put a user name or password in the URL.')).toBeVisible();
  await expect(configPage.saveAndTest()).not.toBeOK();
  await expect(configPage).toHaveAlert('error');
});

test('a URL that is not a CoreScope server fails with a useful message', async ({
  createDataSourceConfigPage,
  page,
}) => {
  const configPage = await createDataSourceConfigPage({ type: DS_TYPE });
  // Grafana itself, reached from the Grafana container: HTTP works but it is not CoreScope.
  await page.getByRole('textbox', { name: 'URL' }).fill('http://localhost:3000');
  await expect(configPage.saveAndTest()).not.toBeOK();
  await expect(configPage).toHaveAlert('error', { hasText: /CoreScope/ });
});

test('auth type switches between none, basic and bearer', async ({ createDataSourceConfigPage, page }) => {
  await createDataSourceConfigPage({ type: DS_TYPE });
  const user = page.getByRole('textbox', { name: 'User', exact: true });
  const password = page.locator('#config-editor-basic-password');
  const token = page.locator('#config-editor-bearer-token');

  await expect(user).toBeHidden();
  await expect(password).toBeHidden();
  await expect(token).toBeHidden();

  await page.getByRole('radio', { name: 'Basic auth' }).check({ force: true });
  await expect(user).toBeVisible();
  await expect(password).toBeVisible();
  await expect(token).toBeHidden();

  await page.getByRole('radio', { name: 'Bearer token' }).check({ force: true });
  await expect(user).toBeHidden();
  await expect(password).toBeHidden();
  await expect(token).toBeVisible();

  await page.getByRole('radio', { name: 'None' }).check({ force: true });
  await expect(user).toBeHidden();
  await expect(password).toBeHidden();
  await expect(token).toBeHidden();
});

test('Skip TLS verification shows its warning', async ({ createDataSourceConfigPage, page }) => {
  await createDataSourceConfigPage({ type: DS_TYPE });
  await expect(page.getByText('Certificates are not checked')).toBeHidden();
  await page.getByRole('switch', { name: /Skip TLS verification/i }).check({ force: true });
  await expect(page.getByText('Certificates are not checked')).toBeVisible();
  await page.getByRole('switch', { name: /Skip TLS verification/i }).uncheck({ force: true });
  await expect(page.getByText('Certificates are not checked')).toBeHidden();
});
