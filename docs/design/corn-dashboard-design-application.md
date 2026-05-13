# Corn Agent Dashboard 디자인 적용 가이드

작성 일시: `2026-05-13 19:39 KST`

이 문서는 Multica 디자인 시스템을 Corn Agent Dashboard에 적용할 때의 구현 기준이다.

## 1. UX Intent

| 항목 | 정의 |
|---|---|
| 사용자 목표 | 개인 워크스페이스의 AI agent 작업 상태를 빠르게 만들고, 읽고, 재실행/취소한다. |
| Primary action | 이슈 생성, 댓글 멘션 위임, 실행 상태 확인 |
| 감정 톤 | 조용하고 신뢰감 있는 local-first 운영 콘솔 |
| 우선순위 | 상태/실행 흐름 가독성 > 장식성 > 브랜드 표현 |

## 2. Design system

### 2.1 CSS token

`web/src/styles/global.css`는 다음 token 계층을 가진다.

- Base: `--background`, `--foreground`, `--card`, `--muted`, `--border`, `--ring`
- Sidebar: `--sidebar`, `--sidebar-foreground`, `--sidebar-accent`
- Semantic: `--brand`, `--success`, `--warning`, `--destructive`, `--info`
- Derived: `--radius`, `--shadow-card`, `--shadow-raised`

### 2.2 Typography

| Selector | 역할 | 크기/굵기 |
|---|---|---|
| `body` | 기본 UI | 14px / 400 |
| `h1` | page title | 16px / 500 |
| `h2` | section title | 14px / 500 |
| `.eyebrow`, `.meta`, badge | metadata | 12px / 500 |
| `.issue-id`, code | 식별자 | mono / 12px |

### 2.3 Layout

```text
┌───────────────────────┬───────────────────────────────────────┐
│ Sidebar 256px          │ Page header 48px                      │
│ - Brand                ├───────────────────────────────────────┤
│ - Product nav          │ Scrollable content                    │
│ - Workspace nav        │ panel / board / forms                 │
│ - Local status footer  │                                       │
└───────────────────────┴───────────────────────────────────────┘
```

### 2.4 Component rules

| Component | 기준 |
|---|---|
| Sidebar item | 32px height, muted foreground, active background + foreground + medium |
| Panel/card | card surface, 1px border, radius-lg, subtle shadow |
| Issue card | identifier first, title second, metadata third |
| Button | 32px height, radius-lg, 500 weight, active 1px translate |
| Input | 36px min-height, token border, focus ring |
| Badge | 20px height, rounded-full, semantic tint |
| Markdown | card/muted surface, no raw HTML, code/table tokenized |

## 3. 적용 파일

| 파일 | 변경 방향 |
|---|---|
| `web/src/styles/global.css` | Multica token + component style 전면 적용 |
| `web/src/layouts/DashboardLayout.tsx` | grouped sidebar, brand mark, local status footer |
| `web/src/components/PageHeader.tsx` | compact header class 유지 |
| `web/src/pages/*.tsx` | 대부분 기존 class 유지, status metadata만 보강 가능 |

## 4. 접근성 체크리스트

- 모든 button/input/textarea/select는 `focus-visible` ring을 가진다.
- hover만으로 의미를 전달하지 않는다.
- 상태는 텍스트(`queued`, `running`, `done`)와 색을 함께 표시한다.
- contrast는 dark mode에서 foreground/muted/semantic token으로 통제한다.

## 5. 회귀 검증

- `pnpm web:build`
- `make check`
- `make e2e-smoke`
- smoke test의 `<script>alert(1)</script>` markdown XSS regression 유지

## 6. 후속 개선 후보

- nav icon 도입은 lucide-react 추가가 필요하므로 이번 범위 제외
- 실제 browser screenshot 기반 visual QA는 별도 phase에서 수행
- light mode toggle은 token상 가능하지만 제품은 현재 dark console로 고정
