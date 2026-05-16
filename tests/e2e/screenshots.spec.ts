import { test, type APIRequestContext, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import path from 'node:path';

const OUT_DIR = path.resolve(__dirname, '../../docs/screenshots');
const VIEWPORT = { width: 1440, height: 900 };

mkdirSync(OUT_DIR, { recursive: true });

type SeededWorkspace = {
  slug: string;
  prefix: string;
  mainAgentId: string;
  writerAgentId: string;
  issues: Array<{ id: string; identifier: string }>;
  ruleId?: string;
};

async function postJSON(request: APIRequestContext, url: string, data: unknown) {
  const res = await request.post(url, { data });
  if (!res.ok()) {
    throw new Error(`POST ${url} failed: ${res.status()} ${await res.text()}`);
  }
  return res.json();
}

async function seedDemoWorkspace(request: APIRequestContext): Promise<SeededWorkspace> {
  const slug = 'ai-news';
  const wsPayload = await postJSON(request, '/api/workspaces', {
    name: 'AI 뉴스 큐레이션',
    slug,
    description: '매일 AI 분야 핵심 뉴스를 5개로 요약하고 후속 글을 작성하는 워크스페이스',
    identifier_prefix: 'NEWS',
    main_agent: {
      name: 'NewsLead',
      runtime: 'codex',
      model: 'gpt-4o-mini',
      summary: '리서치 리드 에이전트',
      tags: 'research,curation',
      instructions: 'Reddit r/MachineLearning 등에서 오늘 가장 화제가 된 5개를 요약합니다. 출처 링크와 영향도를 함께 작성하세요.'
    }
  });
  const workspace = wsPayload.workspace ?? wsPayload;
  const mainAgent = wsPayload.main_agent ?? wsPayload.agent;

  const writerPayload = await postJSON(request, `/api/workspaces/${slug}/agents`, {
    name: 'Writer',
    runtime: 'claude',
    model: 'claude-sonnet-4-5',
    summary: '한국어 블로그 글 작성 전문',
    tags: 'writing,korean',
    instructions: 'NewsLead의 요약을 받아 가독성 좋은 한국어 블로그 글로 다듬습니다. 마크다운 H2/H3 구조를 사용합니다.'
  });
  await postJSON(request, `/api/workspaces/${slug}/agents`, {
    name: 'Publisher',
    runtime: 'gemini',
    model: 'gemini-1.5-pro',
    summary: '게시 자동화 + SEO 메타데이터',
    tags: 'seo,publish',
    instructions: 'Writer가 완성한 글에 SEO 메타데이터를 붙여 발행 큐에 등록합니다.'
  });

  const issuesData = [
    { title: '오늘 AI 뉴스 5개 정리', body: '아침 9시 기준으로 핫한 5개를 골라 요약해줘.' },
    { title: '주말 모아보기', body: '주말 새로 올라온 논문/제품/기업 동향 정리.' },
    { title: '어제 실패한 모델 점검', body: '어제 NewsLead 실행이 timeout으로 실패한 원인 정리.' },
    { title: '주간 작업 회고', body: '이번 주 완료/실패/진행 정리.' }
  ];
  const seededIssues: SeededWorkspace['issues'] = [];
  for (const data of issuesData) {
    const payload = await postJSON(request, `/api/workspaces/${slug}/issues`, data);
    const issue = payload.issue ?? payload;
    seededIssues.push({ id: issue.id, identifier: issue.identifier });
  }

  // Mark one issue done to populate stats
  await request.put(`/api/issues/${seededIssues[1].id}`, { data: { status: 'done' } });

  // Add a system-style comment to issue[0] to show comment thread
  await postJSON(request, `/api/issues/${seededIssues[0].id}/comments`, {
    content: '검토 후 결과 댓글 정리 부탁드립니다.\n\n- 5개 후보 중 임팩트 상위 3개만 본문에 강조\n- 출처 링크는 footnote로 정리'
  });
  // Sub-issue under issue[0]
  await postJSON(request, `/api/issues/${seededIssues[0].id}/subissues`, {
    title: '후속 글로 풀어쓰기',
    body: 'NewsLead 요약을 받아 Writer가 한국어 블로그 글로 풀어 씁니다.'
  });

  // Autopilot rule
  const rulePayload = await postJSON(request, `/api/workspaces/${slug}/autopilot`, {
    name: '매일 09:00 뉴스 브리핑',
    cron_expr: '0 9 * * *',
    issue_title_template: '{{date}} AI 뉴스 브리핑',
    issue_body_template: '오늘의 AI 주요 뉴스를 5개로 요약하고, 출처 링크와 영향도를 정리하세요.',
    assignee_agent_id: '',
    enabled: true
  });
  const rule = rulePayload.rule ?? rulePayload;
  await postJSON(request, `/api/workspaces/${slug}/autopilot`, {
    name: '매주 일요일 회고',
    cron_expr: '0 18 * * 0',
    issue_title_template: '{{date}} 주간 회고',
    issue_body_template: '이번 주 완료/실패/진행 작업을 분류하고 다음 액션을 제안합니다.',
    enabled: false
  });

  return {
    slug,
    prefix: workspace.identifier_prefix,
    mainAgentId: mainAgent.id,
    writerAgentId: (writerPayload.agent ?? writerPayload).id,
    issues: seededIssues,
    ruleId: rule.id
  };
}

async function capture(page: Page, name: string) {
  await page.waitForLoadState('networkidle', { timeout: 10_000 }).catch(() => undefined);
  await page.waitForTimeout(400);
  await page.screenshot({ path: path.join(OUT_DIR, name), fullPage: true });
}

test.describe.configure({ mode: 'serial' });

test.skip(!process.env.GENERATE_SCREENSHOTS, 'set GENERATE_SCREENSHOTS=1 to run capture');

test.describe('Generate README screenshots', () => {
  test.use({ viewport: VIEWPORT });

  test('capture all main screens', async ({ page, request }) => {
    test.setTimeout(120_000);

    // Clear all existing workspaces so the screenshots show only the demo
    const existing = await request.get('/api/workspaces');
    if (existing.ok()) {
      const body = await existing.json();
      const items: Array<{ slug: string }> = Array.isArray(body) ? body : body.workspaces ?? [];
      for (const ws of items) {
        await request.delete(`/api/workspaces/${ws.slug}`);
      }
    }

    const seed = await seedDemoWorkspace(request);

    // 1. Home / dashboard
    await page.goto('/');
    await page.waitForSelector('text=운영 현황');
    await capture(page, '01-home.png');

    // 2. Board (default board view)
    await page.goto(`/w/${seed.slug}/board`);
    await page.waitForSelector('text=이슈 보드');
    await capture(page, '02-board.png');

    // 3. Board list view (filter applied)
    await page.goto(`/w/${seed.slug}/board?view=list&status=open`);
    await page.waitForSelector('.data-table.issue-table');
    await capture(page, '03-board-list.png');

    // 4. Issue Detail (with sub-issue + comments)
    const firstIssue = seed.issues[0];
    await page.goto(`/w/${seed.slug}/issues/${firstIssue.identifier}`);
    await page.waitForSelector('text=흐름 그래프');
    await capture(page, '04-issue-detail.png');

    // 5. Agents list
    await page.goto(`/w/${seed.slug}/agents`);
    await page.waitForSelector('text=에이전트 목록');
    await capture(page, '05-agents.png');

    // 6. Agent Detail (Writer agent — secondary)
    await page.goto(`/w/${seed.slug}/agents/${seed.writerAgentId}`);
    await page.waitForSelector('text=Instructions 버전 이력');
    await capture(page, '06-agent-detail.png');

    // 7. Autopilot
    await page.goto(`/w/${seed.slug}/autopilot`);
    await page.waitForSelector('text=자동화 규칙');
    await capture(page, '07-autopilot.png');

    // 8. Settings
    await page.goto('/settings');
    await page.waitForSelector('text=서버 설정');
    await capture(page, '08-settings.png');

    // 9. Light theme variant (home)
    await page.evaluate(() => window.localStorage.setItem('corn-agent-dashboard-theme', 'light'));
    await page.goto('/');
    await page.waitForSelector('text=운영 현황');
    await capture(page, '09-home-light.png');
  });
});
