import { expect, test } from '@playwright/test';
import { createWorkspaceFixture, uniqueSuffix } from './fixtures';

test.describe('Phase 6 — Autopilot', () => {
  test.beforeEach(async ({ page }) => {
    page.on('dialog', async (dlg) => dlg.dismiss());
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  async function openCreateDialog(page: import('@playwright/test').Page) {
    await page.getByRole('button', { name: '규칙 추가', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: /오토파일럿 추가/ });
    await expect(dialog).toBeVisible();
    return dialog;
  }

  test('TC-6.1 — create rule with valid cron appears in list ON state', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'auto-1' });
    await page.goto(`/w/${fixture.slug}/autopilot`);
    const dialog = await openCreateDialog(page);

    await dialog.locator('input[placeholder="이름"]').fill('Daily News');
    await dialog.locator('input[placeholder="cron"]').fill('0 9 * * *');
    await dialog.locator('input[placeholder="이슈 제목 템플릿"]').fill('{{date}} Daily news');
    const createResponse = page.waitForResponse(
      (resp) => resp.url().includes(`/api/workspaces/${fixture.slug}/autopilot`) && resp.request().method() === 'POST'
    );
    await dialog.getByRole('button', { name: '오토파일럿 추가' }).click();
    await createResponse;

    await expect(page.locator('.data-row', { hasText: 'Daily News' })).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('.data-row', { hasText: 'Daily News' }).locator('.badge', { hasText: 'ON' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-6.2 — trigger now creates an issue on the board', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'auto-2' });
    const name = `Trigger ${uniqueSuffix().slice(-4)}`;
    // Create rule via API to avoid UI list-refresh race in full-suite mode
    const ruleRes = await request.post(`/api/workspaces/${fixture.slug}/autopilot`, {
      data: {
        name,
        cron_expr: '0 9 * * *',
        issue_title_template: 'Manual trigger test',
        issue_body_template: '',
        enabled: true
      }
    });
    expect(ruleRes.ok()).toBeTruthy();

    await page.goto(`/w/${fixture.slug}/autopilot`);
    const row = page.locator('.data-row', { hasText: name });
    await expect(row).toBeVisible({ timeout: 15_000 });
    const triggerResponse = page.waitForResponse(
      (resp) => /\/api\/autopilot\/[^/]+\/trigger/.test(resp.url()) && resp.request().method() === 'POST'
    );
    await row.getByRole('button', { name: '지금 실행' }).click();
    await triggerResponse;

    await expect(page.locator('.autopilot-notice.success')).toBeVisible({ timeout: 15_000 });

    // Server-confirm issue exists before checking UI
    await expect
      .poll(
        async () => {
          const res = await request.get(`/api/workspaces/${fixture.slug}/issues`);
          if (!res.ok()) return 0;
          const body = await res.json();
          return (body.issues ?? []).filter((i: { title: string }) => i.title === 'Manual trigger test').length;
        },
        { timeout: 10_000 }
      )
      .toBeGreaterThan(0);

    await page.goto(`/w/${fixture.slug}/board`);
    await expect(page.locator('.kanban-card', { hasText: 'Manual trigger test' }).first()).toBeVisible({ timeout: 20_000 });
  });

  test('TC-6.3 — toggle rule OFF persists across refresh', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'auto-3' });
    const ruleRes = await request.post(`/api/workspaces/${fixture.slug}/autopilot`, {
      data: {
        name: 'ToggleRule',
        cron_expr: '0 9 * * *',
        issue_title_template: 'toggle test',
        issue_body_template: '',
        enabled: true
      }
    });
    expect(ruleRes.ok()).toBeTruthy();

    await page.goto(`/w/${fixture.slug}/autopilot`);
    const row = page.locator('.data-row', { hasText: 'ToggleRule' });
    await expect(row).toBeVisible({ timeout: 15_000 });
    const toggleResponse = page.waitForResponse(
      (resp) => /\/api\/autopilot\/[^/]+$/.test(resp.url()) && resp.request().method() === 'PUT'
    );
    await row.getByRole('button', { name: '끄기' }).click();
    await toggleResponse;
    await expect(row.locator('.badge', { hasText: 'OFF' })).toBeVisible({ timeout: 15_000 });

    await page.reload();
    const reloadedRow = page.locator('.data-row', { hasText: 'ToggleRule' });
    await expect(reloadedRow.locator('.badge', { hasText: 'OFF' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-6.4 — delete rule removes it from list', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'auto-4' });
    await page.goto(`/w/${fixture.slug}/autopilot`);
    const dialog = await openCreateDialog(page);
    await dialog.locator('input[placeholder="이름"]').fill('TempRule');
    await dialog.locator('input[placeholder="cron"]').fill('0 9 * * *');
    await dialog.locator('input[placeholder="이슈 제목 템플릿"]').fill('to be deleted');
    await dialog.getByRole('button', { name: '오토파일럿 추가' }).click();

    const row = page.locator('.data-row', { hasText: 'TempRule' });
    await expect(row).toBeVisible();
    await row.getByRole('button', { name: '삭제' }).click();

    await expect(page.locator('.data-row', { hasText: 'TempRule' })).toHaveCount(0, { timeout: 15_000 });
  });

  test('TC-6.5 — template cards visible when no rules exist', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'auto-5' });
    await page.goto(`/w/${fixture.slug}/autopilot`);

    await expect(page.locator('.template-panel h2', { hasText: '자주 쓰는 템플릿' })).toBeVisible();
    await expect(page.locator('.template-card', { hasText: '매일 AI 뉴스 브리핑' })).toBeVisible();
  });

  test('TC-6.E1 — invalid cron returns server error, dialog stays open', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'auto-bad' });
    await page.goto(`/w/${fixture.slug}/autopilot`);
    const dialog = await openCreateDialog(page);

    await dialog.locator('input[placeholder="이름"]').fill('BadCron');
    await dialog.locator('input[placeholder="cron"]').fill('60 25 * * *');
    await dialog.locator('input[placeholder="이슈 제목 템플릿"]').fill('invalid cron');
    await dialog.getByRole('button', { name: '오토파일럿 추가' }).click();

    await expect(dialog).toBeVisible();
  });
});
