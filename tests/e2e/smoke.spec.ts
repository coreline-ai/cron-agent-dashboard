import { expect, test } from '@playwright/test';

test('local MVP browser smoke: workspace, issue, detail, comment', async ({ page, request }) => {
  const suffix = Date.now().toString(36);
  const slug = `smoke-${suffix}`;
  const cspViolations: string[] = [];

  page.on('console', (message) => {
    const text = message.text();
    if (/content security policy|violates.*directive/i.test(text)) {
      cspViolations.push(text);
    }
  });
  page.on('pageerror', (error) => {
    const text = error.message;
    if (/content security policy|violates.*directive/i.test(text)) {
      cspViolations.push(text);
    }
  });

  const home = await request.get('/');
  expect(home.ok()).toBeTruthy();
  expect(home.headers()['content-security-policy']).toContain("default-src 'self'");

  const created = await request.post('/api/workspaces', {
    data: {
      name: `Smoke ${suffix}`,
      slug,
      identifier_prefix: 'SMK',
      main_agent: {
        name: 'Noop',
        runtime: 'missing-runtime',
        instructions: 'This smoke runtime intentionally fails fast if a worker claims the run.'
      }
    }
  });
  expect(created.ok()).toBeTruthy();

  let dialogOpened = false;
  page.on('dialog', async (dialog) => {
    dialogOpened = true;
    await dialog.dismiss();
  });

  await page.goto(`/w/${slug}/board`);
  await expect(page.getByRole('heading', { name: '이슈 보드' })).toBeVisible();

  await page.getByRole('button', { name: '새 이슈', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '새 이슈' })).toBeVisible();
  await page.getByPlaceholder('제목').fill('Smoke issue');
  await page.getByPlaceholder('본문').fill('<script>alert(1)</script>');
  await page.getByRole('button', { name: '이슈 생성' }).click();

  const cardLink = page.locator('.kanban-card-link', { hasText: 'SMK-1' });
  await expect(cardLink).toBeVisible({ timeout: 15_000 });
  await cardLink.click();

  await expect(page.getByRole('heading', { name: 'Smoke issue' })).toBeVisible();
  await expect(page.locator('.markdown-content').filter({ hasText: '<script>alert(1)</script>' })).toBeVisible();
  expect(dialogOpened).toBe(false);

  await page.getByPlaceholder('@AgentName 멘션으로 위임할 수 있습니다.').fill('hello from e2e smoke');
  const commentResp = page.waitForResponse(
    (resp) => /\/api\/issues\/[^/]+\/comments/.test(resp.url()) && resp.request().method() === 'POST'
  );
  await page.getByRole('button', { name: '댓글 등록' }).click();
  await commentResp;
  await expect(page.locator('.comment-block').filter({ hasText: 'hello from e2e smoke' })).toBeVisible({ timeout: 15_000 });
  expect(cspViolations).toEqual([]);
});
