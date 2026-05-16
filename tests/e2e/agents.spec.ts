import { expect, test } from '@playwright/test';
import { createWorkspaceFixture, seedSecondAgent, uniqueSuffix } from './fixtures';

test.describe('Phase 5 — Agents & AgentDetail', () => {
  test.beforeEach(async ({ page }) => {
    page.on('dialog', async (dlg) => dlg.dismiss());
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  test('TC-5.1 — create new agent via dialog appears in list and increments count', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'agents-1' });
    await page.goto(`/w/${fixture.slug}/agents`);
    await expect(page.getByRole('heading', { name: '에이전트', exact: true })).toBeVisible();

    await page.getByRole('button', { name: '에이전트 추가', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '에이전트 추가' });
    await expect(dialog).toBeVisible();
    const newName = `Bot${uniqueSuffix().slice(-4).toUpperCase()}`;
    await dialog.locator('input[placeholder="이름"]').fill(newName);
    await dialog.locator('select').first().selectOption('codex');
    await dialog.locator('textarea[placeholder="지시문"]').fill('Agent dialog instructions.');

    await dialog.getByRole('button', { name: '에이전트 추가' }).click();

    await expect(page.locator('.data-row', { hasText: newName })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-5.2 — search filters agents list by name', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'agents-2' });
    await seedSecondAgent(request, fixture.slug, 'AlphaBot');
    await seedSecondAgent(request, fixture.slug, 'BetaBot');

    await page.goto(`/w/${fixture.slug}/agents`);
    await expect(page.locator('.data-row', { hasText: 'AlphaBot' })).toBeVisible();
    await expect(page.locator('.data-row', { hasText: 'BetaBot' })).toBeVisible();

    await page.locator('input.toolbar-search').fill('Alpha');
    await expect(page.locator('.data-row', { hasText: 'AlphaBot' })).toBeVisible();
    await expect(page.locator('.data-row', { hasText: 'BetaBot' })).toHaveCount(0);
  });

  test('TC-5.3 — promote secondary agent to main demotes the previous main', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'agents-3' });
    const second = await seedSecondAgent(request, fixture.slug, 'Promoted');
    const secondId = second.agent?.id ?? second.id;

    await page.goto(`/w/${fixture.slug}/agents/${secondId}`);
    await expect(page.getByRole('heading', { name: '@Promoted' })).toBeVisible();
    const promoteResponse = page.waitForResponse(
      (resp) => resp.url().includes(`/api/agents/${secondId}/promote`) && resp.request().method() === 'POST'
    );
    await page.getByRole('button', { name: '메인으로 승격' }).click();
    await promoteResponse;

    // server-confirm promotion via API to avoid relying on UI refetch timing
    await expect
      .poll(
        async () => {
          const res = await request.get(`/api/agents/${secondId}`);
          if (!res.ok()) return false;
          const body = await res.json();
          return (body.agent ?? body).is_main === true;
        },
        { timeout: 15_000 }
      )
      .toBe(true);

    await page.reload();
    await expect(page.getByRole('button', { name: '메인으로 승격' })).toBeDisabled({ timeout: 15_000 });
  });

  test('TC-5.4 — backoff_seconds field saves and persists', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'agents-4' });
    const second = await seedSecondAgent(request, fixture.slug, 'BackoffBot');
    const secondId = second.agent?.id ?? second.id;

    await page.goto(`/w/${fixture.slug}/agents/${secondId}`);
    const backoffInput = page.locator('input[placeholder="예: 10,60,300"]');
    await backoffInput.fill('10,60,300');
    const saveResponse = page.waitForResponse(
      (resp) => resp.url().includes(`/api/agents/${secondId}`) && resp.request().method() === 'PUT'
    );
    await page.getByRole('button', { name: '저장' }).click();
    await saveResponse;

    await page.reload();
    const reloadedInput = page.locator('input[placeholder="예: 10,60,300"]');
    await expect(reloadedInput).toBeVisible({ timeout: 15_000 });
    await expect(reloadedInput).toHaveValue('10,60,300');
  });

  test('TC-5.5 — editing instructions creates new version in history', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'agents-5' });
    const second = await seedSecondAgent(request, fixture.slug, 'VersionBot');
    const secondId = second.agent?.id ?? second.id;

    await page.goto(`/w/${fixture.slug}/agents/${secondId}`);
    const textarea = page.locator('textarea').filter({ hasText: '' }).first();
    await textarea.fill('Updated instructions version 2');
    await page.getByRole('button', { name: '저장' }).click();

    const versionList = page.locator('.instruction-history-list .instruction-history-card');
    await expect(versionList).toHaveCount(2, { timeout: 15_000 });
  });

  test('TC-5.E1 — main agent delete button is disabled', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'agents-e1' });

    // navigate to main agent detail by clicking its row
    await page.goto(`/w/${fixture.slug}/agents`);
    await page.locator('.data-row', { hasText: 'main' }).first().click();

    await expect(page.getByRole('button', { name: '삭제' })).toBeDisabled();
    await expect(page.getByRole('button', { name: '메인으로 승격' })).toBeDisabled();
  });

  test('TC-5.E2 — duplicate agent name returns server error', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'agents-e2', mainAgentName: 'DupCheck' });

    await page.goto(`/w/${fixture.slug}/agents`);
    await page.getByRole('button', { name: '에이전트 추가', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '에이전트 추가' });
    await dialog.locator('input[placeholder="이름"]').fill('dupcheck'); // case-insensitive collision
    await dialog.locator('textarea[placeholder="지시문"]').fill('dup test');

    await dialog.getByRole('button', { name: '에이전트 추가' }).click();
    await expect(dialog.locator('.error-text', { hasText: '에이전트 추가에 실패' })).toBeVisible({ timeout: 10_000 });
  });
});
