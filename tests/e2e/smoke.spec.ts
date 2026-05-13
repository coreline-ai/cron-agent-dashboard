import { expect, test } from '@playwright/test';

test('local MVP browser smoke: workspace, issue, detail, comment', async ({ page, request }) => {
  const suffix = Date.now().toString(36);
  const slug = `smoke-${suffix}`;

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

  await expect(page.getByRole('link', { name: /SMK-1/ })).toBeVisible();
  await page.getByRole('link', { name: /SMK-1/ }).click();

  await expect(page.getByRole('heading', { name: 'Smoke issue' })).toBeVisible();
  await expect(page.locator('.markdown-content').filter({ hasText: '<script>alert(1)</script>' })).toBeVisible();
  expect(dialogOpened).toBe(false);

  await page.getByPlaceholder('@AgentName 멘션으로 위임할 수 있습니다.').fill('hello from e2e smoke');
  await page.getByRole('button', { name: '댓글 등록' }).click();
  await expect(page.locator('.comment-block').filter({ hasText: 'hello from e2e smoke' })).toBeVisible();
});
