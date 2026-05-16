import { expect, test } from '@playwright/test';
import { createWorkspaceFixture, uniqueSuffix } from './fixtures';

test.describe('Phase 2 — Workspace lifecycle', () => {
  test.beforeEach(async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  test('TC-2.1 — create workspace via dialog → board route + sidebar entry', async ({ page }) => {
    const suffix = uniqueSuffix();
    const slug = `ui-${suffix}`;

    await page.goto('/');
    await page.locator('.workspace-create-link', { hasText: '새 워크스페이스' }).first().click();
    const dialog = page.getByRole('dialog', { name: /워크스페이스 생성/ });
    await expect(dialog).toBeVisible();

    await dialog.locator('input[placeholder="이름"]').fill(`UI ${suffix}`);
    await dialog.locator('input[placeholder="slug (예: ai-news)"]').fill(slug);
    await dialog.locator('input[placeholder="Prefix"]').fill('UIT');
    await dialog.locator('input[placeholder="메인 에이전트 이름"]').fill('MainBot');
    await dialog.locator('select').first().selectOption('codex');
    await dialog.locator('textarea[placeholder="에이전트 지시문"]').fill('UI dialog smoke instructions.');

    await dialog.getByRole('button', { name: '워크스페이스 생성' }).click();

    await expect(page).toHaveURL(new RegExp(`/w/${slug}/board`));
    await expect(page.locator('.sidebar-status')).toContainText(`/${slug}`);
  });

  test('TC-2.2 — empty instructions blocks submission (HTML required)', async ({ page }) => {
    await page.goto('/');
    await page.locator('.workspace-create-link').first().click();
    const dialog = page.getByRole('dialog', { name: /워크스페이스 생성/ });
    await dialog.locator('input[placeholder="이름"]').fill('NoInstr');
    await dialog.locator('input[placeholder="slug (예: ai-news)"]').fill(`no-instr-${uniqueSuffix()}`);
    await dialog.locator('input[placeholder="Prefix"]').fill('NOI');
    await dialog.locator('textarea[placeholder="에이전트 지시문"]').fill('');

    await dialog.getByRole('button', { name: '워크스페이스 생성' }).click();

    // dialog stays open because the textarea is required
    await expect(dialog).toBeVisible();
  });

  test('TC-2.3 — duplicate slug returns server error and keeps dialog open', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'dup' });

    await page.goto('/');
    await page.locator('.workspace-create-link').first().click();
    const dialog = page.getByRole('dialog', { name: /워크스페이스 생성/ });
    await dialog.locator('input[placeholder="이름"]').fill('Duplicate');
    await dialog.locator('input[placeholder="slug (예: ai-news)"]').fill(fixture.slug);
    await dialog.locator('input[placeholder="Prefix"]').fill('DUP');
    await dialog.locator('textarea[placeholder="에이전트 지시문"]').fill('dup test');

    await dialog.getByRole('button', { name: '워크스페이스 생성' }).click();

    await expect(dialog.locator('.error-text', { hasText: '워크스페이스 생성에 실패' })).toBeVisible();
    await expect(dialog).toBeVisible();
  });

  test('TC-2.4 — HomePage "더 보기" toggle when issues exceed 5', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'home-toggle' });
    // create 7 issues via API
    for (let i = 1; i <= 7; i += 1) {
      const res = await request.post(`/api/workspaces/${fixture.slug}/issues`, {
        data: { title: `Issue ${i}`, body: '' }
      });
      expect(res.ok()).toBeTruthy();
    }

    await page.goto('/');
    const toggle = page.locator('button.inline-link', { hasText: /더 보기/ });
    await expect(toggle).toBeVisible();

    const rows = page.locator('.dashboard-issue-row');
    await expect(rows).toHaveCount(5);

    await toggle.click();
    await expect(rows).toHaveCount(7);

    const collapse = page.locator('button.inline-link', { hasText: '접기' });
    await expect(collapse).toBeVisible();
    await collapse.click();
    await expect(rows).toHaveCount(5);
  });

  test('TC-2.E1 — invalid slug returns server error', async ({ page }) => {
    await page.goto('/');
    await page.locator('.workspace-create-link').first().click();
    const dialog = page.getByRole('dialog', { name: /워크스페이스 생성/ });
    await dialog.locator('input[placeholder="이름"]').fill('BadSlug');
    await dialog.locator('input[placeholder="slug (예: ai-news)"]').fill('UPPER_BAD'); // uppercase + underscore — invalid
    await dialog.locator('input[placeholder="Prefix"]').fill('BAD');
    await dialog.locator('textarea[placeholder="에이전트 지시문"]').fill('bad slug test');

    await dialog.getByRole('button', { name: '워크스페이스 생성' }).click();

    await expect(dialog.locator('.error-text', { hasText: '워크스페이스 생성에 실패' })).toBeVisible();
  });
});
