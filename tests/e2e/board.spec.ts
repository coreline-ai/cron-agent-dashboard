import { expect, test } from '@playwright/test';
import { createWorkspaceFixture } from './fixtures';

test.describe('Phase 3 — Board page', () => {
  test.beforeEach(async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  test('TC-3.1 — create issue via dialog → card appears on board', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'board-1' });
    await page.goto(`/w/${fixture.slug}/board`);
    await expect(page.getByRole('heading', { name: '이슈 보드' })).toBeVisible();

    await page.getByRole('button', { name: '새 이슈', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '새 이슈' });
    await expect(dialog).toBeVisible();
    await dialog.getByPlaceholder('제목').fill('Board create test');
    await dialog.getByRole('button', { name: '이슈 생성' }).click();

    await expect(page.getByRole('link', { name: /TST-1/ })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-3.2 — status filter syncs to URL ?status=done', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'board-2' });
    await page.goto(`/w/${fixture.slug}/board`);

    const tablist = page.locator('[role="tablist"][aria-label="이슈 상태 필터"]');
    await tablist.getByRole('button', { name: '완료', exact: true }).click();
    await expect(page).toHaveURL(/status=done/);
  });

  test('TC-3.3 — list view toggle persists to ?view=list', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'board-3' });
    await page.goto(`/w/${fixture.slug}/board`);

    const viewToolbar = page.locator('[role="tablist"][aria-label="보기 방식"]');
    await viewToolbar.getByRole('button', { name: '리스트' }).click();
    await expect(page).toHaveURL(/view=list/);
    await expect(page.locator('.data-table.issue-table')).toBeVisible();
  });

  test('TC-3.4 — search input filters by title', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'board-4' });
    for (const title of ['Alpha topic', 'Beta topic', 'Gamma news']) {
      await request.post(`/api/workspaces/${fixture.slug}/issues`, { data: { title, body: '' } });
    }
    await page.goto(`/w/${fixture.slug}/board`);
    await expect(page.getByRole('link', { name: /TST-/ }).first()).toBeVisible({ timeout: 15_000 });

    await page.locator('input.toolbar-search').fill('Gamma');
    await expect(page.getByRole('link', { name: /Gamma news/ })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole('link', { name: /Alpha topic/ })).toHaveCount(0);
  });

  test('TC-3.5 — column "+" opens dialog with status hint', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'board-5' });
    await page.goto(`/w/${fixture.slug}/board`);
    await page.locator('.kanban-column.done .icon-button').click();

    const dialog = page.getByRole('dialog', { name: '새 이슈' });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText('완료/취소 컬럼에서도 새 이슈는 안전하게');
  });

  test('TC-3.E1 — empty workspace shows "이슈 없음" empty state', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'board-empty' });
    await page.goto(`/w/${fixture.slug}/board`);
    await expect(page.getByRole('heading', { name: '이슈 없음' })).toBeVisible();
    await expect(page.getByText('새 이슈 버튼으로 첫 작업을 생성하세요')).toBeVisible();
  });

  test('TC-3.E2 — invalid status URL param falls back to all', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'board-bad' });
    await page.goto(`/w/${fixture.slug}/board?status=notreal`);
    const tablist = page.locator('[role="tablist"][aria-label="이슈 상태 필터"]');
    await expect(tablist.getByRole('button', { name: '전체', exact: true })).toHaveClass(/active/);
  });
});
