import { expect, test } from '@playwright/test';
import { createWorkspaceFixture } from './fixtures';

test.describe('Phase 7 — Settings', () => {
  test.beforeEach(async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  test('TC-7.1 — workspace default_timeout changes persist after reload', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'set-timeout' });

    await page.goto('/settings');
    const row = page.locator('.setting-action', { hasText: fixture.slug });
    await expect(row).toBeVisible({ timeout: 20_000 });
    const timeoutInput = row.locator('input[type="number"]').first();
    await timeoutInput.fill('1200');
    const saveResponse = page.waitForResponse(
      (resp) => resp.url().includes(`/api/workspaces/${fixture.slug}`) && resp.request().method() === 'PUT'
    );
    await row.getByRole('button', { name: '저장' }).click();
    await saveResponse;

    // Verify server state directly to avoid UI race
    const serverState = await request.get(`/api/workspaces/${fixture.slug}`);
    expect(serverState.ok()).toBeTruthy();
    const body = await serverState.json();
    expect((body.workspace ?? body).default_timeout_seconds).toBe(1200);

    // Navigate away and back to force fresh page state
    await page.goto('/');
    await page.goto('/settings');
    const reloadedRow = page.locator('.setting-action', { hasText: fixture.slug });
    await expect(reloadedRow).toBeVisible({ timeout: 20_000 });
    await expect(reloadedRow.locator('input[type="number"]').first()).toHaveValue('1200');
  });

  test('TC-7.2 — auto-chain enable exposes guard inputs', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'set-chain' });

    await page.goto('/settings');
    const row = page.locator('.setting-action', { hasText: fixture.slug });
    await expect(row).toBeVisible({ timeout: 20_000 });

    // auto-chain max depth input + cost limit input should already be present
    await expect(row.getByText(/최대 chain depth/)).toBeVisible();
    await expect(row.getByText(/24시간 자동 chain run 제한/)).toBeVisible();
    await expect(row.getByText(/24시간 자동 chain 비용 제한/)).toBeVisible();
    await expect(row.getByText(/dry-run: 감지하되 실행 등록 안 함/)).toBeVisible();
  });

  test('TC-7.3 — DB backup action returns success message', async ({ page, request }) => {
    await createWorkspaceFixture(request, { slugPrefix: 'set-backup' });
    await page.goto('/settings');

    await page.getByRole('button', { name: 'DB 백업' }).click();
    await expect(page.locator('.settings-message', { hasText: '백업 완료' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-7.4 — log cleanup action returns success message', async ({ page, request }) => {
    await createWorkspaceFixture(request, { slugPrefix: 'set-cleanup' });
    await page.goto('/settings');

    await page.getByRole('button', { name: '로그 정리' }).click();
    await expect(page.locator('.settings-message', { hasText: '로그 정리 완료' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-7.5 — Token save updates localStorage', async ({ page, request }) => {
    await createWorkspaceFixture(request, { slugPrefix: 'set-token' });
    await page.goto('/settings');

    await page.locator('input[placeholder="Bearer token"]').fill('demo-token');
    await page.getByRole('button', { name: '토큰 저장' }).click();

    const stored = await page.evaluate(() => window.localStorage.getItem('corn_agent_dashboard_token'));
    expect(stored).toBe('demo-token');

    // cleanup so subsequent tests don't carry auth header
    await page.getByRole('button', { name: '토큰 삭제' }).click();
  });

  test('TC-7.6 — Usage dashboard renders 7d/30d cards', async ({ page, request }) => {
    await createWorkspaceFixture(request, { slugPrefix: 'set-usage' });
    await page.goto('/settings');

    await expect(page.locator('.setting-action', { hasText: '최근 7일' })).toBeVisible();
    await expect(page.locator('.setting-action', { hasText: '최근 30일' })).toBeVisible();
  });

  test('TC-7.E1 — Vacuum action surfaces success message', async ({ page, request }) => {
    await createWorkspaceFixture(request, { slugPrefix: 'set-vacuum' });
    await page.goto('/settings');

    await page.getByRole('button', { name: 'Vacuum' }).click();
    await expect(page.locator('.settings-message', { hasText: 'Vacuum 완료' })).toBeVisible({ timeout: 15_000 });
  });
});
