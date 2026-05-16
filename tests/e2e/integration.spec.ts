import { expect, test } from '@playwright/test';
import { createWorkspaceFixture, seedSecondAgent, uniqueSuffix } from './fixtures';

test.describe('Phase 8 — Cross-page integration & resilience', () => {
  test.beforeEach(async ({ page }) => {
    page.on('dialog', async (dlg) => dlg.dismiss());
    await page.context().clearCookies();
    await page.goto('/');
    await page.evaluate(() => window.localStorage.clear());
  });

  test('TC-8.1 (A) — workspace → second agent → board issue → issue detail flow', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'int-a' });
    await seedSecondAgent(request, fixture.slug, 'Writer');

    // Board → create issue
    await page.goto(`/w/${fixture.slug}/board`);
    await page.getByRole('button', { name: '새 이슈', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '새 이슈' });
    await dialog.getByPlaceholder('제목').fill('A-flow root');
    await dialog.getByRole('button', { name: '이슈 생성' }).click();
    await expect(page.getByRole('link', { name: /TST-1/ })).toBeVisible({ timeout: 15_000 });

    // Issue detail
    await page.getByRole('link', { name: /TST-1/ }).click();
    await expect(page.getByRole('heading', { name: 'A-flow root' })).toBeVisible();

    // Add a regular comment
    await page.getByPlaceholder('@AgentName 멘션으로 위임할 수 있습니다.').fill('handing off');
    const commentResponse = page.waitForResponse(
      (resp) => /\/api\/issues\/[^/]+\/comments/.test(resp.url()) && resp.request().method() === 'POST'
    );
    await page.getByRole('button', { name: '댓글 등록' }).click();
    await commentResponse;
    await expect(page.locator('.comment-block', { hasText: 'handing off' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-8.2 (B) — autopilot trigger creates issue → lineage graph node visible', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'int-b' });
    const ruleName = `Auto ${uniqueSuffix().slice(-4)}`;
    const ruleRes = await request.post(`/api/workspaces/${fixture.slug}/autopilot`, {
      data: {
        name: ruleName,
        cron_expr: '0 9 * * *',
        issue_title_template: 'Auto generated',
        issue_body_template: '',
        enabled: true
      }
    });
    expect(ruleRes.ok()).toBeTruthy();

    await page.goto(`/w/${fixture.slug}/autopilot`);
    const row = page.locator('.data-row', { hasText: ruleName });
    await expect(row).toBeVisible({ timeout: 15_000 });
    const triggerResp = page.waitForResponse(
      (resp) => /\/api\/autopilot\/[^/]+\/trigger/.test(resp.url()) && resp.request().method() === 'POST'
    );
    await row.getByRole('button', { name: '지금 실행' }).click();
    await triggerResp;

    // Go to board and click newly created issue
    await page.goto(`/w/${fixture.slug}/board`);
    const issueLink = page.getByRole('link', { name: /Auto generated/ });
    await expect(issueLink).toBeVisible({ timeout: 20_000 });
    await issueLink.click();

    // Lineage graph panel exists
    await expect(page.locator('.issue-detail-main h2', { hasText: '흐름 그래프' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-8.E1 — invalid identifier surfaces "이슈 로드 실패" alert', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'int-404' });
    await page.goto(`/w/${fixture.slug}/issues/TST-9999`);

    await expect(page.locator('.mutation-error-alert, .error-alert', { hasText: /이슈 로드 실패|이슈를 불러오지 못/ }).first()).toBeVisible({ timeout: 15_000 });
  });

  test('TC-8.E2 — Sub-issue creation appears on parent lineage list', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'int-sub' });
    const res = await request.post(`/api/workspaces/${fixture.slug}/issues`, { data: { title: 'Parent issue', body: '' } });
    expect(res.ok()).toBeTruthy();

    await page.goto(`/w/${fixture.slug}/issues/TST-1`);
    await page.getByPlaceholder('하위 이슈 제목').fill('Child from integration');
    await page.getByRole('button', { name: '하위 이슈 생성' }).click();

    await expect(page.locator('.subissue-card', { hasText: 'Child from integration' })).toBeVisible({ timeout: 15_000 });
  });

  test('TC-8.E3 — XSS body and comment text both render plain without alert', async ({ page, request }) => {
    const fixture = await createWorkspaceFixture(request, { slugPrefix: 'int-xss' });
    const res = await request.post(`/api/workspaces/${fixture.slug}/issues`, {
      data: { title: 'XSS hardening', body: '<script>alert("body")</script>' }
    });
    expect(res.ok()).toBeTruthy();

    let dialogTriggered = false;
    page.on('dialog', () => {
      dialogTriggered = true;
    });

    await page.goto(`/w/${fixture.slug}/issues/TST-1`);
    await expect(page.locator('.markdown-content').filter({ hasText: '<script>alert("body")</script>' })).toBeVisible();

    await page.getByPlaceholder('@AgentName 멘션으로 위임할 수 있습니다.').fill('<script>alert("comment")</script>');
    await page.getByRole('button', { name: '댓글 등록' }).click();
    await expect(page.locator('.comment-block').filter({ hasText: '<script>alert("comment")</script>' })).toBeVisible({ timeout: 15_000 });

    expect(dialogTriggered).toBe(false);
  });
});
