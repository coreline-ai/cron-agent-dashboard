package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// MultiAgentLabOptions controls how the multi-workspace development lab is
// materialized. Empty fields keep safe local defaults.
type MultiAgentLabOptions struct {
	// WorkingDir is the repository path that lab agents should operate on.
	// When empty, the worker store's default per-slug workdir is used at run time.
	WorkingDir string
	// Runtime is the CLI runtime assigned to all seeded agents. Defaults to codex.
	Runtime string
}

// SeededMultiAgentLab summarizes the workspaces created or found by
// SeedMultiAgentLab.
type SeededMultiAgentLab struct {
	Workspaces []SeededLabWorkspace
}

// SeededLabWorkspace summarizes one lab workspace and its initial project issue.
type SeededLabWorkspace struct {
	Workspace         store.Workspace
	MainAgent         store.Agent
	Worker            []store.Agent
	Issues            []store.Issue
	Project           string
	AlreadyHad        bool
	CreatedAgentCount int
	CreatedIssueCount int
}

type labWorkspaceSpec struct {
	Name        string
	Slug        string
	Prefix      string
	Role        string
	Project     string
	Description string
	MainName    string
	MainSummary string
	Workers     []labAgentSpec
	OwnedPaths  []string
	SelfTests   []string
	Deliverable []string
}

type labAgentSpec struct {
	Name    string
	Summary string
	Tags    string
}

// SeedMultiAgentLab provisions the seven-workspace development lab used to
// validate 5+ workspace / multi-agent operation. It is intentionally
// idempotent: existing lab workspaces are reused, missing worker agents are
// topped up, and the initial [LAB] project issue is created only when absent.
func SeedMultiAgentLab(ctx context.Context, st *store.Store, opts MultiAgentLabOptions) (SeededMultiAgentLab, error) {
	if st == nil {
		return SeededMultiAgentLab{}, errors.New("seed multi-agent lab: store is nil")
	}
	runtime := strings.TrimSpace(opts.Runtime)
	if runtime == "" {
		runtime = "codex"
	}
	workingDir := strings.TrimSpace(opts.WorkingDir)

	specs := multiAgentLabSpecs(runtime)
	result := SeededMultiAgentLab{Workspaces: make([]SeededLabWorkspace, 0, len(specs))}
	for _, spec := range specs {
		seeded, err := seedLabWorkspace(ctx, st, spec, workingDir, runtime)
		if err != nil {
			return SeededMultiAgentLab{}, err
		}
		result.Workspaces = append(result.Workspaces, seeded)
	}
	return result, nil
}

func seedLabWorkspace(ctx context.Context, st *store.Store, spec labWorkspaceSpec, workingDir, runtime string) (SeededLabWorkspace, error) {
	workspace, mainAgent, err := st.GetWorkspace(ctx, spec.Slug)
	createdWorkspace := false
	if errors.Is(err, store.ErrNotFound) {
		runLimit := 12
		autoClose := false
		workspace, mainAgent, err = st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
			Name:                     spec.Name,
			Slug:                     spec.Slug,
			IdentifierPrefix:         spec.Prefix,
			Description:              spec.Description,
			WorkingDir:               workingDir,
			DefaultTimeoutSeconds:    900,
			AutoChainEnabled:         true,
			AutoChainMaxDepth:        3,
			AutoChainDailyRunLimit:   &runLimit,
			AutoChainDailyCostMicros: 0,
			AutoChainDryRun:          false,
			AutoCloseOnRunDone:       &autoClose,
			PerRunWorktree:           true,
			MainAgent: store.CreateAgentInput{
				Name:         spec.MainName,
				Runtime:      runtime,
				Summary:      spec.MainSummary,
				Tags:         "lab,lead," + spec.Slug,
				Instructions: labLeadInstructions(spec),
			},
		})
		if err != nil {
			return SeededLabWorkspace{}, fmt.Errorf("seed multi-agent lab: create workspace %s: %w", spec.Slug, err)
		}
		createdWorkspace = true
	} else if err != nil {
		return SeededLabWorkspace{}, fmt.Errorf("seed multi-agent lab: probe workspace %s: %w", spec.Slug, err)
	}

	workers, createdAgents, err := ensureLabWorkers(ctx, st, workspace.ID, spec, runtime)
	if err != nil {
		return SeededLabWorkspace{}, err
	}
	issues, createdIssues, err := ensureLabIssues(ctx, st, workspace.ID, mainAgent.ID, spec)
	if err != nil {
		return SeededLabWorkspace{}, err
	}

	return SeededLabWorkspace{
		Workspace:         workspace,
		MainAgent:         mainAgent,
		Worker:            workers,
		Issues:            issues,
		Project:           spec.Project,
		AlreadyHad:        !createdWorkspace,
		CreatedAgentCount: createdAgents,
		CreatedIssueCount: createdIssues,
	}, nil
}

func ensureLabWorkers(ctx context.Context, st *store.Store, workspaceID string, spec labWorkspaceSpec, runtime string) ([]store.Agent, int, error) {
	agents, err := st.ListAgents(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("seed multi-agent lab: list agents for %s: %w", spec.Slug, err)
	}
	byName := make(map[string]store.Agent, len(agents))
	for _, agent := range agents {
		byName[strings.ToLower(agent.Name)] = agent
	}

	workers := make([]store.Agent, 0, len(spec.Workers))
	created := 0
	for _, workerSpec := range spec.Workers {
		if existing, ok := byName[strings.ToLower(workerSpec.Name)]; ok {
			workers = append(workers, existing)
			continue
		}
		agent, err := st.CreateAgent(ctx, workspaceID, store.CreateAgentInput{
			Name:         workerSpec.Name,
			Runtime:      runtime,
			Summary:      workerSpec.Summary,
			Tags:         workerSpec.Tags + ",lab," + spec.Slug,
			Instructions: labWorkerInstructions(spec, workerSpec),
		})
		if err != nil {
			return nil, created, fmt.Errorf("seed multi-agent lab: create agent %s/%s: %w", spec.Slug, workerSpec.Name, err)
		}
		created++
		workers = append(workers, agent)
	}
	return workers, created, nil
}

func ensureLabIssues(ctx context.Context, st *store.Store, workspaceID, mainAgentID string, spec labWorkspaceSpec) ([]store.Issue, int, error) {
	title := labIssueTitle(spec)
	issues, err := st.ListIssues(ctx, workspaceID, store.ListIssuesFilter{Query: title, Limit: 200})
	if err != nil {
		return nil, 0, fmt.Errorf("seed multi-agent lab: list issues for %s: %w", spec.Slug, err)
	}
	for _, issue := range issues {
		if issue.Title == title {
			return []store.Issue{issue}, 0, nil
		}
	}
	issue, _, err := st.CreateIssueWithInitialRun(ctx, workspaceID, store.CreateIssueInput{
		Title:           title,
		Body:            labIssueBody(spec),
		AssigneeAgentID: mainAgentID,
		CreatedBy:       "user",
	})
	if err != nil {
		return nil, 0, fmt.Errorf("seed multi-agent lab: create issue for %s: %w", spec.Slug, err)
	}
	return []store.Issue{issue}, 1, nil
}

func labIssueTitle(spec labWorkspaceSpec) string {
	return fmt.Sprintf("[LAB] %s — %s", spec.Slug, spec.Project)
}

func labLeadInstructions(spec labWorkspaceSpec) string {
	return strings.TrimSpace(fmt.Sprintf(`당신은 %s 워크스페이스의 리드 에이전트 %s입니다.

역할:
- 프로젝트 목표를 이해하고 worker에게 작게 쪼개어 위임합니다.
- 한 댓글에는 worker 한 명만 멘션합니다. 여러 worker를 동시에 멘션하지 마세요.
- worker 결과가 도착하기 전에는 통합 결과를 확정하지 마세요.
- 파일 소유 경계를 넘는 변경이 필요하면 pm-command-hub 리더 승인을 요청하세요.

프로젝트:
%s

소유 파일/영역:
%s

진행 규칙:
- 시작 전 git status --short로 현재 상태를 확인합니다.
- main 브랜치에 직접 push/force-push 하지 않습니다.
- 실험 branch는 lab/%s 를 기준으로 보고합니다.
- 완료 보고에는 Changed files / Tests / Risks / Next action을 반드시 포함합니다.
- 실패 시 원인, 재현 명령, 축소 제안을 먼저 남깁니다.
`, spec.Name, spec.MainName, spec.Project, markdownList(spec.OwnedPaths), spec.Slug))
}

func labWorkerInstructions(spec labWorkspaceSpec, worker labAgentSpec) string {
	return strings.TrimSpace(fmt.Sprintf(`당신은 %s 워크스페이스의 worker 에이전트 %s입니다.

담당:
%s

작업 원칙:
- 리드가 지시한 범위만 구현합니다.
- 아래 소유 파일/영역 밖의 변경은 리드 승인 없이 수행하지 않습니다.
%s
- 변경 후 가능한 가장 좁은 테스트부터 실행합니다.
- 완료 보고에는 변경 파일, 실행 테스트, 남은 리스크를 짧게 정리합니다.
- 다음 단계가 필요하면 마지막 줄에 @%s 로 보고합니다.
`, spec.Name, worker.Name, worker.Summary, markdownList(spec.OwnedPaths), spec.MainName))
}

func labIssueBody(spec labWorkspaceSpec) string {
	return strings.TrimSpace(fmt.Sprintf(`## 목표

%s

## 워크스페이스 역할

%s

## Branch / 작업 경계

- branch: lab/%s
- workspace slug: %s
- leader: @%s

## 소유 파일/영역

%s

## 권장 위임 순서

%s

## 산출물

%s

## 자체 테스트

%s

## 완료 보고 형식

- Changed files:
- Tests:
- Risks:
- Next action:

## 운영 규칙

- 병렬 실행 중에도 main 직접 수정/force-push 금지.
- 파일 경계를 넘는 수정은 pm-command-hub의 @Integrator 리뷰 후 진행.
- run이 실패하면 재시도 전에 terminal_reason/failure_kind/stdout log를 먼저 요약.
- worktree 디스크 사용량과 GC 결과는 Settings에서 확인.
`, spec.Project, spec.Role, spec.Slug, spec.Slug, spec.MainName, markdownList(spec.OwnedPaths), workerMentionPlan(spec.Workers), markdownList(spec.Deliverable), markdownList(spec.SelfTests)))
}

func workerMentionPlan(workers []labAgentSpec) string {
	if len(workers) == 0 {
		return "- 리드가 직접 수행"
	}
	lines := make([]string, 0, len(workers))
	for idx, worker := range workers {
		lines = append(lines, fmt.Sprintf("- %d. @%s — %s", idx+1, worker.Name, worker.Summary))
	}
	return strings.Join(lines, "\n")
}

func markdownList(items []string) string {
	if len(items) == 0 {
		return "- 없음"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
}

func multiAgentLabSpecs(runtime string) []labWorkspaceSpec {
	_ = runtime // runtime is accepted by the caller so future specs can vary per runtime without API churn.
	return []labWorkspaceSpec{
		{
			Name:        "PM Command Hub",
			Slug:        "pm-command-hub",
			Prefix:      "PMH",
			Role:        "리더/통합 워크스페이스 — 전체 분배, triage, 최종 통합 체크리스트를 담당합니다.",
			Project:     "전체 작업 분배, 진행 상황 triage, 최종 통합 체크리스트",
			Description: "5+ 워크스페이스 실개발 테스트를 지휘하는 리더 워크스페이스입니다.",
			MainName:    "ProgramLead",
			MainSummary: "멀티 워크스페이스 실험 PM 리더",
			Workers: []labAgentSpec{
				{Name: "Integrator", Summary: "workspace별 diff와 테스트 결과를 통합 순서로 정리", Tags: "integration,review"},
				{Name: "ReleaseCaptain", Summary: "최종 검증/릴리스 dry-run 체크리스트 관리", Tags: "release,qa"},
			},
			OwnedPaths:  []string{"dev-plan/", "TODO.md", "CHANGELOG.md", "docs/MULTI_AGENT_LAB.md"},
			SelfTests:   []string{"git diff --check", "문서 링크/체크박스 검토"},
			Deliverable: []string{"workspace별 진행표", "통합 순서", "blocker/후속 backlog 목록"},
		},
		{
			Name:        "Dashboard Design Lab",
			Slug:        "dashboard-design-lab",
			Prefix:      "DSG",
			Role:        "프론트엔드 디자인 실행 워크스페이스 — demo와 dashboard의 최신 레이아웃을 검증합니다.",
			Project:     "CoreMCP 스타일 데모/대시보드 레이아웃 최신화 + screenshot 갱신",
			Description: "CoreMCP 스타일의 developer command-center 레이아웃을 dashboard demo에 적용하는 실험 공간입니다.",
			MainName:    "DesignLead",
			MainSummary: "제품 UI 리디자인 리드",
			Workers: []labAgentSpec{
				{Name: "FrontendPolish", Summary: "React/CSS 레이아웃과 토큰 구현", Tags: "frontend,css"},
				{Name: "VisualQA", Summary: "screenshots, contrast, responsive QA", Tags: "qa,visual"},
			},
			OwnedPaths:  []string{"web/src/styles/", "web/src/pages/HomePage.tsx", "web/src/components/", "docs/screenshots/"},
			SelfTests:   []string{"pnpm --filter web build", "pnpm --filter web test", "make screenshots"},
			Deliverable: []string{"CoreMCP-style dashboard visual update", "updated docs/screenshots", "visual QA notes"},
		},
		{
			Name:        "Auth Realtime Lab",
			Slug:        "auth-realtime-lab",
			Prefix:      "RLT",
			Role:        "Realtime/API 실행 워크스페이스 — SSE token mode와 fallback 안정성을 검증합니다.",
			Project:     "SSE token mode/realtime stream 회귀 검증과 UX 보강",
			Description: "Issue detail SSE와 token 인증 경계를 실사용 흐름에서 검증하는 실험 공간입니다.",
			MainName:    "RealtimeLead",
			MainSummary: "SSE/API 리드",
			Workers: []labAgentSpec{
				{Name: "BackendAuth", Summary: "SSE/token auth handler와 Go tests 점검", Tags: "backend,auth"},
				{Name: "FrontendSSE", Summary: "fetch SSE client와 UX fallback 점검", Tags: "frontend,sse"},
				{Name: "QAEngineer", Summary: "Playwright/API 회귀 시나리오 작성", Tags: "qa,e2e"},
			},
			OwnedPaths:  []string{"internal/httpapi/handlers_run.go", "internal/httpapi/*sse*", "web/src/lib/api.ts", "web/src/pages/IssueDetailPage.tsx", "tests/e2e/"},
			SelfTests:   []string{"go test ./internal/httpapi/...", "pnpm --filter web test", "make e2e-smoke"},
			Deliverable: []string{"token mode SSE 회귀 결과", "fallback UX 개선안 또는 테스트", "known risk list"},
		},
		{
			Name:        "Worktree Ops Lab",
			Slug:        "worktree-ops-lab",
			Prefix:      "OPS",
			Role:        "운영/GC 실행 워크스페이스 — per-run worktree 안전장치를 스트레스 검증합니다.",
			Project:     "per-run worktree disk report/GC 스트레스 검증과 운영 문서화",
			Description: "병렬 run 후 worktree 디스크 사용량과 GC 보호 규칙을 검증하는 실험 공간입니다.",
			MainName:    "OpsLead",
			MainSummary: "운영 안정성 리드",
			Workers: []labAgentSpec{
				{Name: "BackendGC", Summary: "terminal/orphan/queued/running GC fixture 검증", Tags: "backend,gc"},
				{Name: "OpsQA", Summary: "Settings/OPERATIONS 문서와 runbook 검증", Tags: "ops,docs"},
			},
			OwnedPaths:  []string{"internal/app/worktree.go", "internal/app/maintenance*.go", "internal/app/*worktree*_test.go", "web/src/pages/SettingsPage.tsx", "docs/OPERATIONS.md"},
			SelfTests:   []string{"go test ./internal/app/...", "go test ./...", "git diff --check"},
			Deliverable: []string{"GC stress test 결과", "queued/running 보호 확인", "운영 문서 업데이트"},
		},
		{
			Name:        "History Import Lab",
			Slug:        "history-import-lab",
			Prefix:      "IMP",
			Role:        "데이터 이동 실행 워크스페이스 — history import/export materialization을 검증합니다.",
			Project:     "workspace history import materialization round-trip fixture/e2e 강화",
			Description: "workspace export/import v2의 history 복원 신뢰성을 검증하는 실험 공간입니다.",
			MainName:    "DataLead",
			MainSummary: "데이터 round-trip 리드",
			Workers: []labAgentSpec{
				{Name: "ImportTester", Summary: "history 포함 export/import fixture와 count 비교", Tags: "backend,data"},
				{Name: "CLIQA", Summary: "workspace-export/import CLI smoke와 docs 검증", Tags: "cli,qa"},
			},
			OwnedPaths:  []string{"internal/app/workspace_io*.go", "cmd/cron-agent-dashboard/main.go", "docs/API.md", "docs/OPERATIONS.md"},
			SelfTests:   []string{"go test ./internal/app/...", "go test ./cmd/...", "workspace-export/import smoke"},
			Deliverable: []string{"round-trip fixture 결과", "metadata 복원 gap 목록", "CLI smoke log"},
		},
		{
			Name:        "Release CI Lab",
			Slug:        "release-ci-lab",
			Prefix:      "REL",
			Role:        "릴리스/CI 실행 워크스페이스 — Homebrew dry-run과 e2e-full gate를 점검합니다.",
			Project:     "Homebrew tap publish dry-run + e2e-full CI gate 리허설",
			Description: "릴리스 자동화와 CI 필수 신호를 실제 명령으로 재검증하는 실험 공간입니다.",
			MainName:    "ReleaseLead",
			MainSummary: "릴리스/CI 리드",
			Workers: []labAgentSpec{
				{Name: "CIEngineer", Summary: "e2e-full workflow와 artifact 업로드 검증", Tags: "ci,e2e"},
				{Name: "DocsQA", Summary: "Homebrew 운영 문서와 release smoke 절차 검증", Tags: "docs,release"},
			},
			OwnedPaths:  []string{".github/workflows/", "scripts/", "docs/homebrew/", "Makefile", "CHANGELOG.md"},
			SelfTests:   []string{"make release-smoke", "make e2e-full", "git diff --check"},
			Deliverable: []string{"release dry-run 결과", "Homebrew formula render 확인", "CI gate risk 목록"},
		},
		{
			Name:        "Agent Orchestration Lab",
			Slug:        "agent-orchestration-lab",
			Prefix:      "ORC",
			Role:        "오케스트레이션 실행 워크스페이스 — 5+ workspace 실험을 재현 가능한 seed/runbook으로 만듭니다.",
			Project:     "5+ 워크스페이스 멀티 에이전트 데모 seed/runbook 작성",
			Description: "멀티 에이전트 실험 자체를 제품 기능과 문서로 고정하는 실험 공간입니다.",
			MainName:    "OrchestrationLead",
			MainSummary: "멀티 에이전트 구성 리드",
			Workers: []labAgentSpec{
				{Name: "SeedBuilder", Summary: "seed-lab CLI와 fixture/test 구현", Tags: "go,seed"},
				{Name: "RunbookWriter", Summary: "MULTI_AGENT_LAB 문서와 README 연결", Tags: "docs,runbook"},
			},
			OwnedPaths:  []string{"internal/app/seed*.go", "cmd/cron-agent-dashboard/main.go", "docs/MULTI_AGENT_LAB.md", "README.md", "dev-plan/"},
			SelfTests:   []string{"go test ./internal/app/...", "go test ./...", "git diff --check"},
			Deliverable: []string{"idempotent seed-lab command", "7-workspace runbook", "테스트 결과 요약"},
		},
	}
}
