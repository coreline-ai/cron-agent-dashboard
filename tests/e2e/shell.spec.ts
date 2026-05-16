import { expect, test } from '@playwright/test';
import { clearAllWorkspaces, createWorkspaceFixture } from './fixtures';

test.describe('Phase 1 — Shell & Layout', () => {
  test.beforeEach(async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  test('TC-1.1 — HomePage renders and 대시보드 nav is active', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { name: '운영 현황' })).toBeVisible();
    const activeNav = page.locator('nav.nav-list a.active', { hasText: '대시보드' });
    await expect(activeNav).toBeVisible();
  });

  test('TC-1.2 — switching workspace via switcher navigates to /w/<slug>/board', async ({ page, request }) => {
    const first = await createWorkspaceFixture(request, { slugPrefix: 'shell-a' });
    const second = await createWorkspaceFixture(request, { slugPrefix: 'shell-b' });

    await page.goto(`/w/${first.slug}/board`);
    await expect(page.getByRole('heading', { name: '이슈 보드' })).toBeVisible();

    await page.locator('.workspace-switcher-trigger').click();
    const dialog = page.getByRole('dialog', { name: '워크스페이스 선택' });
    await expect(dialog).toBeVisible();
    await dialog.locator('.workspace-option', { hasText: second.slug }).click();

    await expect(page).toHaveURL(new RegExp(`/w/${second.slug}/board`));
  });

  test('TC-1.3 — theme toggle persists across reloads via localStorage', async ({ page, request }) => {
    await createWorkspaceFixture(request, { slugPrefix: 'shell-theme' });
    await page.goto('/');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

    await page.getByRole('button', { name: /light 테마로 전환/i }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    const stored = await page.evaluate(() => window.localStorage.getItem('corn-agent-dashboard-theme'));
    expect(stored).toBe('light');
  });

  test('TC-1.4 — workspace nav items are disabled when no workspace exists', async ({ page, request }) => {
    await clearAllWorkspaces(request);

    await page.goto('/');
    const issuesDisabled = page.locator('.nav-disabled', { hasText: '이슈 보드' });
    const agentsDisabled = page.locator('.nav-disabled', { hasText: '에이전트' });
    const autopilotDisabled = page.locator('.nav-disabled', { hasText: '오토파일럿' });
    await expect(issuesDisabled).toHaveAttribute('aria-disabled', 'true');
    await expect(agentsDisabled).toHaveAttribute('aria-disabled', 'true');
    await expect(autopilotDisabled).toHaveAttribute('aria-disabled', 'true');
  });

  test('TC-1.E1 — invalid /w/zzz/board falls back to first workspace', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'shell-fallback' });

    await page.goto('/w/does-not-exist-zz/board');
    await expect(page.locator('.sidebar-status')).toContainText(`/${fixture.slug}`, { timeout: 15_000 });
  });
});
