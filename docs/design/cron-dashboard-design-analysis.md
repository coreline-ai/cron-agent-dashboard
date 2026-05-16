# Cron Design Reference 디자인 코드 레벨 분석

작성 일시: `2026-05-13 19:39 KST`

대상 참조: `external-design-reference`  
적용 대상: `/Users/hwanchoi/projects/core-agent-dashboard`

> 원본 Cron Design Reference 폴더는 읽기 전용으로 분석만 수행했다. 실제 산출물은 본 저장소의 `docs/design/`와 `web/src/`에 작성한다.

## 1. 분석 대상 파일

| 영역 | 참조 파일 | 핵심 내용 |
|---|---|---|
| 철학/규칙 | `docs/design.md` | neutral-first, 색은 signal, 3개 core font size, 4px spacing grid |
| 컬러 token | `packages/ui/styles/tokens.css` | OKLCh 기반 light/dark shadcn token, brand/success/warning/info/priority |
| base style | `packages/ui/styles/base.css` | scrollbar, focus outline, dark variant, subtle animation, body token 적용 |
| 버튼 | `packages/ui/components/ui/button.tsx` | 32px 기본 높이, `rounded-lg`, `font-medium`, hover/active/focus 일관성 |
| Badge | `packages/ui/components/ui/badge.tsx` | 20px height, rounded-4xl, text-xs, semantic variants |
| Sidebar | `packages/ui/components/ui/sidebar.tsx`, `packages/views/layout/app-sidebar.tsx` | 256px sidebar, grouped nav, active+hover 충돌 방지, muted foreground |
| Dashboard shell | `packages/views/layout/dashboard-layout.tsx` | fixed sidebar + `SidebarInset`, content overflow 분리 |
| Page header | `packages/views/layout/page-header.tsx` | 48px header, bottom border, compact breadcrumb |
| Issue board | `packages/views/issues/components/*.tsx` | card density, 0.5px border, subtle shadow, metadata-first hierarchy |
| Markdown | `packages/ui/markdown/markdown.css` | markdown은 token surface 위에서 overflow 안전 처리 |

## 2. 디자인 철학 요약

Cron Design Reference는 장식적인 SaaS 랜딩 스타일이 아니라 **업무 밀도 높은 도구 UI**를 지향한다.

1. **절제된 표면**  
   불필요한 gradient, glow, 과한 shadow를 제거하고 `background/card/muted/border` 계층으로 깊이를 만든다.

2. **색은 signal**  
   브랜드 blue도 넓은 영역에 쓰지 않고 badge, focus, 작은 강조에 제한한다. 상태 색은 done/warning/destructive 같은 의미를 전달할 때만 사용한다.

3. **정보 밀도**  
   페이지 제목은 `text-base` 수준, 대부분 UI는 14px, metadata는 12px이다. 큰 hero typography는 대시보드 내부와 맞지 않는다.

4. **일관된 상태 피드백**  
   hover는 `muted`, active는 `muted + foreground + font-medium`, focus는 ring token으로 통일한다.

## 3. Token 추출

### 3.1 색상

Cron Design Reference의 핵심 token은 shadcn naming을 따른다.

| Token | Light | Dark | Cron 적용 |
|---|---:|---:|---|
| `--background` | `oklch(1 0 0)` | `oklch(0.18 0.005 285.823)` | body 배경 |
| `--foreground` | `oklch(0.141 0.005 285.823)` | `oklch(0.985 0 0)` | 기본 텍스트 |
| `--card` | `oklch(1 0 0)` | `oklch(0.21 0.006 285.885)` | panel/card/sidebar 표면 |
| `--muted` | `oklch(0.967 0.001 286.375)` | `oklch(0.274 0.006 286.033)` | hover/secondary surface |
| `--border` | `oklch(0.92 0.004 286.32)` | `oklch(1 0 0 / 10%)` | border/divider |
| `--brand` | `oklch(0.55 0.16 255)` | `oklch(0.65 0.16 255)` | logo, focus accent, key badge |
| `--success` | `oklch(0.55 0.16 145)` | `oklch(0.65 0.15 145)` | 완료/ON |
| `--warning` | `oklch(0.75 0.16 85)` | `oklch(0.70 0.16 85)` | queued/running |
| `--destructive` | `oklch(0.577 0.245 27.325)` | `oklch(0.704 0.191 22.216)` | 삭제/실패/취소 |

### 3.2 Typography

| 역할 | Cron Design Reference 규칙 | Cron 적용 |
|---|---|---|
| UI/본문 | Inter + system CJK fallback | `--font-sans`로 정의 |
| 제목 | heading도 sans | hero급 대형 제목 제거, compact h1 |
| 코드/ID | Geist Mono fallback | issue id, runtime, log, path 등에 mono 사용 |
| 크기 | `16/14/12px` 중심 | `h1=16px`, `body=14px`, `meta=12px` |
| 굵기 | `400/500`만 기본 | `font-weight: 500` 위주, 800/900 제거 |

### 3.3 Spacing / radius

| 역할 | 값 | 적용 |
|---|---:|---|
| compact gap | 4~8px | nav, metadata, button row |
| group gap | 12~16px | card internals, form fields |
| section gap | 24px | page stack 큰 구획 |
| radius | `0.625rem` base | button/card/input에 `8~10px` 적용 |
| shadow | 매우 약함 | card hover에만 미세 shadow |

## 4. Component 패턴

### 4.1 Sidebar

Cron Design Reference sidebar는 다음 구조가 핵심이다.

- 고정 폭: 약 256px
- `bg-sidebar`, `text-sidebar-foreground`
- nav group label은 uppercase/작은 글씨/낮은 대비
- nav item은 28~32px height, hover는 muted, active는 active background + `font-medium`
- active item hover 시 selected state가 약해지지 않도록 composite state를 유지

Cron 적용: `DashboardLayout`의 sidebar를 `PRODUCT / WORKSPACE` 그룹으로 분리하고, current active route가 Cron Design Reference식 active state로 보이게 한다.

### 4.2 Page header

Cron Design Reference `PageHeader`는 큰 hero가 아니라 48px compact bar이다.

- `height: 48px`
- `border-bottom`
- breadcrumb/meta + page title
- page content는 별도 scrollable region

Cron 적용: 기존 대형 `h1 clamp(4.5rem)`를 제거하고 `page-header`를 compact 정보 헤더로 전환한다.

### 4.3 Cards / panels

Cron Design Reference issue card:

```tsx
rounded-lg border-[0.5px] border-border bg-card py-3 px-2.5
shadow-[0_3px_6px_-2px_rgba(0,0,0,0.02),0_1px_1px_0_rgba(0,0,0,0.04)]
transition-colors hover:border-accent hover:bg-accent
```

Cron 적용:

- `.panel`, `.metric-card`, `.issue-card`를 token 기반 surface로 통합
- 0.5~1px border, subtle hover, no transform/glow
- metadata → title → description 순서로 시각 계층 정리

### 4.4 Button / input

Cron Design Reference button:

- height 32px
- radius-lg
- `font-medium`, not bold
- focus-visible ring 3px
- active translate-y 1px
- destructive는 넓은 red fill이 아니라 red tint background

Cron 적용: `.button`, `.button.secondary`, `.button.danger`, inputs를 이 규칙에 맞춤.

## 5. 기존 Cron UI와 차이

| 항목 | 기존 Cron | Cron Design Reference 기준 | 조치 |
|---|---|---|---|
| 배경 | radial gradient + glow | neutral background | gradient 제거 |
| 제목 | 4.5rem hero | compact 16px title | h1 축소 |
| 카드 | 큰 radius/강한 shadow | subtle card | panel/card 재정의 |
| 버튼 | pill + 800 weight | 32px rounded-lg + 500 | button 재정의 |
| nav | 단일 list | grouped sidebar | markup 보강 |
| 색상 | cyan/green 다수 | neutral + semantic small signal | badge/status 재정리 |
| 폼 | 큰 padding/dark glass | compact field | input/textarea 재정의 |

## 6. Cron 적용 원칙

- Tailwind/shadcn을 새로 도입하지 않고 현재 Plain CSS + React 구조에 token만 이식한다.
- class 이름은 기존 `.panel`, `.button`, `.issue-card` 등을 유지해 변경 범위를 CSS 중심으로 제한한다.
- Cron Design Reference의 팀 기능/초대/프로젝트/인박스 UX는 가져오지 않는다.
- 단일 사용자 제품 특성에 맞춰 sidebar 항목은 최소로 유지한다.
