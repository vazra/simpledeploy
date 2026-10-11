import { test, expect } from '@playwright/test';
import { loginAsAdmin, getState, TEST_ADMIN } from '../helpers/auth.js';
import { apiLogin, apiRequest } from '../helpers/api.js';
import { findServiceContainer, dockerInspect } from '../helpers/docker.js';

// The only E2E coverage of the real Docker SDK calls behind /api/docker/*.
test.describe('Docker Management', () => {
  test('info, images, networks and volumes reflect the real daemon', async ({ page }) => {
    expect((await apiLogin(TEST_ADMIN.username, TEST_ADMIN.password)).status).toBe(200);
    const info = await apiRequest('GET', '/api/docker/info');
    expect(info.status).toBe(200);
    expect(info.data.containers_running).toBeGreaterThan(0);

    const nginx = dockerInspect(findServiceContainer('e2e-nginx', 'web'));
    const pg = dockerInspect(findServiceContainer('e2e-postgres', 'db'));
    const imageShortId = nginx.Image.replace('sha256:', '').slice(0, 12);
    const networkNames = Object.keys(nginx.NetworkSettings.Networks);
    const pgVolume = pg.Mounts.find((m) => m.Type === 'volume').Name;

    await loginAsAdmin(page);
    await page.goto(`${getState().baseURL}/#/docker`);
    const main = page.locator('main');
    const card = (label) => main.locator(`div:has(> div:text-is("${label}")) > div:nth-child(2)`);
    await expect(card('Version')).toHaveText(info.data.server_version);

    await main.getByRole('button', { name: 'Images', exact: true }).click();
    const imageRow = main.locator('table tr').filter({ hasText: imageShortId });
    await expect(imageRow).toHaveCount(1);
    await expect(imageRow.locator('td').first()).toHaveText(/(^|\/)nginx$/);

    await main.getByRole('button', { name: 'Networks & Volumes' }).click();
    for (const name of networkNames) {
      await expect(main.locator('table td').getByText(name, { exact: true })).toHaveCount(1);
    }
    await expect(main.locator('table td').getByText(pgVolume, { exact: true })).toHaveCount(1);
  });
});
