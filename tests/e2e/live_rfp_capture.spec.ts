import { test } from '@playwright/test';
import path from 'node:path';
import { mkdirSync } from 'node:fs';

const OUT = path.resolve(__dirname, '../../docs/screenshots');
mkdirSync(OUT, { recursive: true });

test.skip(!process.env.CAPTURE_RFP_LIVE, 'set CAPTURE_RFP_LIVE=1 to capture live RFP-1');

test.use({ viewport: { width: 1440, height: 900 }, baseURL: 'http://127.0.0.1:8080' });

test('capture RFP-1 issue detail with pipeline + activity strip', async ({ page }) => {
  await page.goto('/w/rfp-studio/board');
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(800);
  await page.screenshot({ path: path.join(OUT, '10-rfp-board.png'), fullPage: true });

  await page.goto('/w/rfp-studio/issues/RFP-1');
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(OUT, '11-rfp-issue.png'), fullPage: false });
});
