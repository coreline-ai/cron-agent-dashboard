package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

// SeededExample summarizes what SeedExample created (or found already present).
type SeededExample struct {
	Workspace  store.Workspace
	MainAgent  store.Agent
	Worker     []store.Agent
	AlreadyHad bool // true when the example workspace already existed and nothing was created.
}

// SeedExample provisions a small example workspace that demonstrates the
// hub-PM auto-chain pattern: a main "Lead" agent with hub guard instructions,
// two worker agents, and auto-chain enabled with defaults. It is idempotent —
// calling it twice with the same store leaves the workspace exactly once.
//
// This is intended for fresh clones and demo runs so README quick start
// instructions can finish with `cron-agent-dashboard seed`.
func SeedExample(ctx context.Context, st *store.Store) (SeededExample, error) {
	if st == nil {
		return SeededExample{}, errors.New("seed: store is nil")
	}

	const slug = "demo-studio"

	if ws, mainAgent, err := st.GetWorkspace(ctx, slug); err == nil {
		// Workspace already exists; collect the worker roster but do not
		// recreate anything.
		agents, listErr := st.ListAgents(ctx, ws.ID)
		if listErr != nil {
			return SeededExample{}, fmt.Errorf("seed: list existing agents: %w", listErr)
		}
		workers := make([]store.Agent, 0, len(agents))
		for _, a := range agents {
			if !a.IsMain {
				workers = append(workers, a)
			}
		}
		return SeededExample{Workspace: ws, MainAgent: mainAgent, Worker: workers, AlreadyHad: true}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return SeededExample{}, fmt.Errorf("seed: probe workspace: %w", err)
	}

	ws, mainAgent, err := st.CreateWorkspaceWithMainAgent(ctx, store.CreateWorkspaceInput{
		Name:             "데모 협업 스튜디오",
		Slug:             slug,
		IdentifierPrefix: "DEMO",
		Description:      "Lead hub-PM과 2명의 worker가 한 이슈를 순차로 처리하는 데모 워크스페이스. cron-agent-dashboard seed로 생성됩니다.",
		AutoChainEnabled: true,
		MainAgent: store.CreateAgentInput{
			Name:    "Lead",
			Runtime: "codex",
			Summary: "PM hub — 결과 도착 전까지 통합 문서 작성 금지, 한 번에 한 멘션",
			Tags:    "demo,hub-pm",
			Instructions: strings.TrimSpace(`
당신은 데모 협업 스튜디오의 Lead PM입니다. 새 이슈가 들어오면:

1. 이슈 요지를 한 줄 요약하고
2. 분배 계획표(어떤 worker에 무엇을 의뢰할지)를 작성하고
3. 마지막 줄에 다음 단계 worker 한 명만 멘션합니다 (예: @Writer).

운영 규칙(위반 시 결과 반려):
- worker 결과가 이슈에 도착하기 전에는 통합 문서를 작성하지 마라. placeholder/추정/"v0.1"도 금지.
- auto-chain은 결과 댓글의 첫 멘션만 dispatch합니다. 한 댓글에 여러 worker를 동시에 멘션하지 마라.
- 모든 worker 결과가 도착하면 통합 문서 v1.0을 작성하고, 그 댓글에는 어떤 worker도 멘션하지 말 것(체인 종료).

main agent(Lead) 재진입은 chain_depth를 증가시키지 않으므로 max_depth가 작아도 풀체인이 가능합니다.
`),
		},
	})
	if err != nil {
		return SeededExample{}, fmt.Errorf("seed: create workspace: %w", err)
	}

	workerSpecs := []store.CreateAgentInput{
		{
			Name:         "Writer",
			Runtime:      "codex",
			Summary:      "한국어 초안 작성",
			Tags:         "demo,writer",
			Instructions: "Lead가 위임한 주제로 한국어 markdown 초안을 작성한다. 마지막 줄에 다음 단계가 누구인지 잘 모르겠으면 @Lead로 보고한다.",
		},
		{
			Name:         "Reviewer",
			Runtime:      "codex",
			Summary:      "사실 확인 + 톤 점검",
			Tags:         "demo,reviewer",
			Instructions: "Writer 초안을 받아 사실 확인과 톤 점검을 수행하고 결과를 한국어 markdown으로 정리한다. 마지막 줄은 @Lead로 보고한다.",
		},
	}
	workers := make([]store.Agent, 0, len(workerSpecs))
	for _, spec := range workerSpecs {
		w, err := st.CreateAgent(ctx, ws.ID, spec)
		if err != nil {
			return SeededExample{}, fmt.Errorf("seed: create worker %s: %w", spec.Name, err)
		}
		workers = append(workers, w)
	}

	return SeededExample{Workspace: ws, MainAgent: mainAgent, Worker: workers, AlreadyHad: false}, nil
}
