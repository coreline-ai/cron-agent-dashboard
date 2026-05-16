import { expect, test } from '@playwright/test';
import { createWorkspaceFixture, seedSecondAgent } from './fixtures';

async function createIssue(
  request: import('@playwright/test').APIRequestContext,
  slug: string,
  title: string,
  body = ''
) {
  const res = await request.post(`/api/workspaces/${slug}/issues`, { data: { title, body } });
  expect(res.ok()).toBeTruthy();
  const payload = await res.json();
  return payload.issue ?? payload;
}

test.describe('Phase 4 — Issue Detail page', () => {
  test.beforeEach(async ({ page }) => {
    page.on('dialog', async (dlg) => dlg.dismiss());
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  test('TC-4.1 — comment submission persists in thread', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'detail-1' });
    await createIssue(request, fixture.slug, 'Detail comment test');

    await page.goto(`/w/${fixture.slug}/issues/TST-1`);
    await expect(page.getByRole('heading', { name: 'Detail comment test' })).toBeVisible();

    await page.getByPlaceholder('@AgentName 멘션으로 위임할 수 있습니다.').fill('hello from comment test');
    await page.getByRole('button', { name: '댓글 등록' }).click();

    await expect(page.locator('.comment-block').filter({ hasText: 'hello from comment test' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-4.2 — mention autocomplete suggests agents on "@"', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'detail-mention' });
    await seedSecondAgent(request, fixture.slug, 'Writer');
    await createIssue(request, fixture.slug, 'Mention test');

    await page.goto(`/w/${fixture.slug}/issues/TST-1`);
    // wait for agents query to load (textarea + autocomplete need it)
    await expect(page.locator('select option', { hasText: 'Writer' }).first()).toBeAttached({ timeout: 15_000 });

    const textarea = page.getByPlaceholder('@AgentName 멘션으로 위임할 수 있습니다.');
    await textarea.click();
    await textarea.pressSequentially('@Wri', { delay: 60 });

    const dropdown = page.locator('.mention-suggestions');
    await expect(dropdown).toBeVisible({ timeout: 10_000 });
    await expect(dropdown).toContainText('Writer');
  });

  test('TC-4.3 — sub-issue creation appears in 하위 이슈 list', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'detail-sub' });
    await createIssue(request, fixture.slug, 'Parent for sub');

    await page.goto(`/w/${fixture.slug}/issues/TST-1`);
    await page.getByPlaceholder('하위 이슈 제목').fill('Child issue from e2e');
    const subResp = page.waitForResponse(
      (resp) => /\/api\/issues\/[^/]+\/subissues/.test(resp.url()) && resp.request().method() === 'POST'
    );
    await page.getByRole('button', { name: '하위 이슈 생성' }).click();
    await subResp;

    await expect(page.locator('.subissue-card', { hasText: 'Child issue from e2e' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-4.E1 — XSS body renders as plain text without triggering alert', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'detail-xss' });
    await createIssue(request, fixture.slug, 'XSS issue', '<script>alert(1)</script>');

    let dialogOpened = false;
    page.on('dialog', () => {
      dialogOpened = true;
    });

    await page.goto(`/w/${fixture.slug}/issues/TST-1`);
    await expect(page.locator('.markdown-content').filter({ hasText: '<script>alert(1)</script>' })).toBeVisible();
    expect(dialogOpened).toBe(false);
  });

  test('TC-4.E2 — Issue Summary Rail shows status and run history container', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'detail-rail' });
    await createIssue(request, fixture.slug, 'Rail test');

    await page.goto(`/w/${fixture.slug}/issues/TST-1`);
    await expect(page.locator('.issue-detail-main h2', { hasText: '본문' })).toBeVisible();
    await expect(page.locator('.issue-detail-main h2', { hasText: 'Run 이력' })).toBeVisible();
    await expect(page.locator('.issue-detail-main h2', { hasText: '댓글 스레드' })).toBeVisible();
  });
});
