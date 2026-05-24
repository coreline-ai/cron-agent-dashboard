package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// SeededDevTeam summarizes the single-workspace AI dev-team seed.
type SeededDevTeam struct {
	Workspace         store.Workspace
	Agents            []store.Agent
	Skills            []store.Skill
	AssignmentCount   int
	AlreadyHad        bool
	CreatedAgentCount int
}

type devTeamAgentSpec struct {
	Name         string
	Runtime      string
	Summary      string
	Tags         string
	Instructions string
	IsMain       bool
}

type devTeamSkillSpec struct {
	Name        string
	Description string
	Content     string
	Targets     []string
}

// SeedDevTeam provisions one hub-PM style workspace with seven role agents and
// eight always-on skills. The slug defaults to "ai-dev-team" when omitted.
func SeedDevTeam(ctx context.Context, st *store.Store, slug, workingDir string) (SeededDevTeam, error) {
	if st == nil {
		return SeededDevTeam{}, errors.New("seed dev-team: store is nil")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = "ai-dev-team"
	}
	workingDir = strings.TrimSpace(workingDir)

	agentSpecs := devTeamAgentSpecs()
	workspace, mainAgent, err := st.GetWorkspace(ctx, slug)
	createdWorkspace := false
	if errors.Is(err, store.ErrNotFound) {
		runLimit := 24
		autoClose := false
		workspace, mainAgent, err = st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
			Name:                     "AI Dev Team",
			Slug:                     slug,
			IdentifierPrefix:         "DEVTEAM",
			Description:              "PM·디자이너·백엔드·프론트엔드·DB·QA·DevOps 7개 역할 에이전트가 hub-PM 패턴으로 협업하는 개발팀 워크스페이스입니다.",
			WorkingDir:               workingDir,
			DefaultTimeoutSeconds:    900,
			AutoChainEnabled:         true,
			AutoChainMaxDepth:        8,
			AutoChainDailyRunLimit:   &runLimit,
			AutoChainDailyCostMicros: 0,
			AutoCloseOnRunDone:       &autoClose,
			PerRunWorktree:           true,
			MainAgent: store.CreateAgentInput{
				Name:         agentSpecs[0].Name,
				Runtime:      agentSpecs[0].Runtime,
				Summary:      agentSpecs[0].Summary,
				Tags:         agentSpecs[0].Tags,
				Instructions: agentSpecs[0].Instructions,
			},
		})
		if err != nil {
			return SeededDevTeam{}, fmt.Errorf("seed dev-team: create workspace: %w", err)
		}
		createdWorkspace = true
	} else if err != nil {
		return SeededDevTeam{}, fmt.Errorf("seed dev-team: probe workspace %s: %w", slug, err)
	}

	agents, createdAgents, err := ensureDevTeamAgents(ctx, st, workspace.ID, mainAgent, agentSpecs)
	if err != nil {
		return SeededDevTeam{}, err
	}
	skills, assignments, err := ensureDevTeamSkills(ctx, st, workspace.ID, agents, devTeamSkillSpecs())
	if err != nil {
		return SeededDevTeam{}, err
	}
	return SeededDevTeam{Workspace: workspace, Agents: agents, Skills: skills, AssignmentCount: assignments, AlreadyHad: !createdWorkspace, CreatedAgentCount: createdAgents}, nil
}

func ensureDevTeamAgents(ctx context.Context, st *store.Store, workspaceID string, mainAgent store.Agent, specs []devTeamAgentSpec) ([]store.Agent, int, error) {
	existing, err := st.ListAgents(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("seed dev-team: list agents: %w", err)
	}
	byName := make(map[string]store.Agent, len(existing))
	for _, agent := range existing {
		byName[strings.ToLower(agent.Name)] = agent
	}
	out := []store.Agent{mainAgent}
	created := 0
	for _, spec := range specs {
		if spec.IsMain {
			continue
		}
		if agent, ok := byName[strings.ToLower(spec.Name)]; ok {
			out = append(out, agent)
			continue
		}
		agent, err := st.CreateAgent(ctx, workspaceID, store.CreateAgentInput{Name: spec.Name, Runtime: spec.Runtime, Summary: spec.Summary, Tags: spec.Tags, Instructions: spec.Instructions})
		if err != nil {
			return nil, created, fmt.Errorf("seed dev-team: create agent %s: %w", spec.Name, err)
		}
		created++
		out = append(out, agent)
	}
	return out, created, nil
}

func ensureDevTeamSkills(ctx context.Context, st *store.Store, workspaceID string, agents []store.Agent, specs []devTeamSkillSpec) ([]store.Skill, int, error) {
	agentByName := make(map[string]store.Agent, len(agents))
	for _, agent := range agents {
		agentByName[strings.ToLower(agent.Name)] = agent
	}
	allAgents := make([]string, 0, len(agents))
	for _, agent := range agents {
		allAgents = append(allAgents, agent.Name)
	}
	skills := make([]store.Skill, 0, len(specs))
	assignments := 0
	enabled := true
	for _, spec := range specs {
		skill, err := st.UpsertSkill(ctx, workspaceID, store.UpsertSkillInput{Name: spec.Name, Description: spec.Description, Content: spec.Content, SourceType: "manual", TrustLevel: "local", Enabled: &enabled})
		if err != nil {
			return nil, assignments, fmt.Errorf("seed dev-team: upsert skill %s: %w", spec.Name, err)
		}
		skills = append(skills, skill)
		targets := spec.Targets
		if len(targets) == 1 && targets[0] == "*" {
			targets = allAgents
		}
		for idx, target := range targets {
			agent, ok := agentByName[strings.ToLower(target)]
			if !ok {
				return nil, assignments, fmt.Errorf("seed dev-team: skill %s references unknown agent %s", spec.Name, target)
			}
			if _, err := st.AssignAgentSkill(ctx, agent.ID, store.AssignAgentSkillInput{SkillID: skill.ID, ActivationMode: "always", Priority: 10 + idx, Enabled: &enabled}); err != nil {
				return nil, assignments, fmt.Errorf("seed dev-team: assign skill %s -> %s: %w", spec.Name, agent.Name, err)
			}
			assignments++
		}
	}
	return skills, assignments, nil
}

func devTeamAgentSpecs() []devTeamAgentSpec {
	return []devTeamAgentSpec{
		{Name: "Lead", Runtime: "claude", Summary: "hub PM — 작업 분해, 순차 멘션, QA verdict 기반 종료", Tags: "dev-team,lead,hub", IsMain: true, Instructions: devTeamLeadInstructions()},
		{Name: "Designer", Runtime: "gemini", Summary: "UI/UX spec, layout, design token 결정", Tags: "dev-team,design", Instructions: devTeamWorkerInstructions("Designer", "UI/UX 스펙과 디자인 토큰을 markdown으로 작성합니다.")},
		{Name: "Backend", Runtime: "codex", Summary: "API, 비즈니스 로직, Go 테스트", Tags: "dev-team,backend", Instructions: devTeamWorkerInstructions("Backend", "API 엔드포인트와 Go 비즈니스 로직을 구현하고 Go 테스트를 실행합니다.")},
		{Name: "Frontend", Runtime: "codex", Summary: "React UI 구현, build/test", Tags: "dev-team,frontend", Instructions: devTeamWorkerInstructions("Frontend", "React/TypeScript UI를 구현하고 pnpm build/test를 실행합니다.")},
		{Name: "DB", Runtime: "codex", Summary: "migration, schema, idempotency", Tags: "dev-team,database", Instructions: devTeamWorkerInstructions("DB", "forward-only migration과 데이터 모델 변경을 검증합니다.")},
		{Name: "QA", Runtime: "claude", Summary: "회귀 테스트와 QA-PASS/FAIL verdict", Tags: "dev-team,qa", Instructions: devTeamWorkerInstructions("QA", "회귀 테스트를 실행하고 ## QA-PASS 또는 ## QA-FAIL verdict를 남깁니다.")},
		{Name: "DevOps", Runtime: "codex", Summary: "CI, release, deployment automation", Tags: "dev-team,devops", Instructions: devTeamWorkerInstructions("DevOps", "CI workflow, release smoke, 배포 자동화 변경을 검증합니다.")},
	}
}

func devTeamLeadInstructions() string {
	return strings.TrimSpace(`당신은 AI Dev Team의 Lead hub-PM입니다.

규칙:
- 직접 코드를 크게 수정하지 말고 작업을 작게 분해해 한 번에 한 worker만 멘션합니다.
- worker 결과가 도착하기 전에는 다음 결론을 확정하지 않습니다.
- QA가 ## QA-PASS를 남기면 최종 요약을 작성하고 체인을 종료합니다.
- QA가 ## QA-FAIL을 남기면 실패 원인에 맞는 worker 한 명에게만 재작업을 지시합니다.
- 완료 보고는 Changed files / Tests / Risks / Next action 형식으로 작성합니다.`)
}

func devTeamWorkerInstructions(name, responsibility string) string {
	return strings.TrimSpace(fmt.Sprintf(`당신은 AI Dev Team의 %s 역할입니다.

책임:
- %s
- Lead가 지시한 범위만 수행합니다.
- 변경 전후 git status --short를 확인합니다.
- 가능한 가장 좁은 테스트부터 실행하고 결과를 보고합니다.
- 완료 댓글은 ## RESULT: <CODE> 헤더로 시작합니다.
- 추가 조정이 필요하면 마지막 줄에 @Lead 로 보고합니다.`, name, responsibility))
}

func devTeamSkillSpecs() []devTeamSkillSpec {
	return []devTeamSkillSpec{
		{Name: "result-protocol", Description: "역할별 결과 댓글 헤더와 테스트 보고 형식", Targets: []string{"*"}, Content: `모든 결과 댓글은 다음 중 하나로 시작한다.

- ## RESULT: PLAN
- ## RESULT: BUILD-PASS
- ## RESULT: BUILD-FAIL
- ## RESULT: QA-PASS
- ## RESULT: QA-FAIL

본문에는 Changed files, Tests, Risks, Next action을 포함한다.`},
		{Name: "hub-routing", Description: "Lead의 순차 멘션과 QA verdict 처리 규칙", Targets: []string{"Lead"}, Content: `Lead는 한 댓글에 한 worker만 멘션한다. 직접 구현보다 분해/검수/통합을 우선한다. QA-PASS 수신 시 최종 요약 후 체인을 종료하고, QA-FAIL 수신 시 원인에 맞는 worker에게 재작업을 지시한다.`},
		{Name: "ux-spec", Description: "Designer의 UX spec 출력 형식", Targets: []string{"Designer"}, Content: `Designer는 목표, 사용자 흐름, 레이아웃, 색상/타입/간격 토큰, 접근성 체크리스트를 markdown으로 정리한다. 구현자가 바로 적용할 수 있는 컴포넌트 단위 acceptance criteria를 포함한다.`},
		{Name: "backend-patterns", Description: "Backend 구현/검증 패턴", Targets: []string{"Backend"}, Content: `Backend는 기존 Go 패키지 경계를 유지한다. store/httpapi/app 중 변경 위치를 먼저 밝히고, sentinel error와 contract test를 우선한다. 최소 go test ./internal/...를 실행한다.`},
		{Name: "frontend-patterns", Description: "Frontend 구현/검증 패턴", Targets: []string{"Frontend"}, Content: `Frontend는 기존 React Query/API client 패턴을 따른다. 텍스트 회귀를 피하고 접근성 label을 유지한다. pnpm --filter web build와 관련 vitest를 실행한다.`},
		{Name: "db-migrations", Description: "DB migration 안전 규칙", Targets: []string{"DB"}, Content: `마이그레이션은 forward-only로 작성한다. SQLite table rebuild가 필요하면 FK/인덱스/기존 데이터 보존을 명시한다. 두 번 부팅해도 idempotent해야 한다.`},
		{Name: "qa-verdict", Description: "QA verdict와 재현 보고 규칙", Targets: []string{"QA"}, Content: `QA는 재현 명령, 기대/실제 결과, 실패 로그를 포함한다. 통과 시 ## QA-PASS, 실패 시 ## QA-FAIL과 함께 재작업 대상 agent를 마지막 줄에 한 명만 멘션한다.`},
		{Name: "devops-ci", Description: "CI/release 자동화 검증 규칙", Targets: []string{"DevOps"}, Content: `DevOps는 GitHub Actions, Makefile, release scripts 변경 시 dry-run 또는 로컬 대응 명령을 실행한다. secret이 필요한 단계는 dry-run으로 대체하고 필요한 secret 이름만 문서화한다.`},
	}
}
