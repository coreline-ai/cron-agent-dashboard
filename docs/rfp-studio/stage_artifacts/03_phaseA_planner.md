# Stage 03 · @Planner · Phase A

시각: 2026-05-19T13:00:11.960891Z

---

기획 산출물 작성이 완료되었습니다. 파일 쓰기 권한이 필요합니다. 아래에 산출물 전문을 공유합니다.

---

# 한국정밀산업 코퍼레이트 사이트 글로벌 리뉴얼 — 기획 산출물

> **작성**: Planner | **기준일**: 2026-05-19 | **근거**: Sales 분석 보고서 + RFP 원문

---

## 1. 정보 구조(IA) — 3단계 사이트맵

```
/ (Home)
├── 회사 소개 (About)
│   ├── CEO 인사말
│   ├── 연혁 / 비전
│   ├── 사업장 안내 (수원 본사 · 대전 연구소 · 호치민 · 도쿄)
│   └── 인증 및 수상
├── 제품 (Products)
│   ├── 카테고리 목록 (컨트롤러 · 센서 등)
│   │   └── 제품 상세 (스펙 · 다운로드 · 관련 제품)
│   ├── 제품 비교
│   └── 제품 검색 / 필터
├── 채용 (Careers)
│   ├── 채용 공고 목록
│   │   └── 공고 상세 → Greenhouse 지원하기
│   └── 사내문화 · 복지
├── 뉴스 / IR (News & IR)
│   ├── 보도자료
│   ├── 공시 · 재무자료
│   └── 뉴스 상세 (PDF 첨부)
├── 문의 · 방문 예약 (Contact)
│   ├── 일반 문의 폼
│   ├── 방문 예약 폼
│   └── 지사별 연락처 · 지도
├── 통합검색 (Search)
└── 공통
    ├── 헤더 (언어 전환 · GNB)
    ├── 푸터 (회사 정보 · 개인정보처리방침 · 이용약관)
    └── 언어 프리픽스 (/ko, /en, /zh)
```

---

## 2. 사용자 스토리

| # | 우선순위 | 사용자 스토리 |
|---|:---:|---|
| US-01 | **P0** | As a **해외 바이어**, I want **영문/중문으로 제품 스펙을 필터·비교** so that **구매 의사결정을 빠르게 내릴 수 있다.** |
| US-02 | **P0** | As a **마케팅 담당자**, I want **CMS에서 직접 제품·뉴스를 등록·수정** so that **개발팀 의존 없이 콘텐츠를 관리할 수 있다.** |
| US-03 | **P0** | As a **글로벌 방문자**, I want **URL 기반 다국어 전환(/ko, /en, /zh)** so that **내 언어로 사이트를 이용할 수 있다.** |
| US-04 | **P0** | As a **잠재 고객**, I want **문의 폼으로 제품 문의를 보내면 슬랙 알림이 발송** so that **신속한 영업 응대를 받을 수 있다.** |
| US-05 | **P0** | As a **모바일 사용자**, I want **반응형 레이아웃에서 제품 카탈로그를 탐색** so that **어디서든 정보를 확인할 수 있다.** |
| US-06 | **P1** | As a **구직자**, I want **채용 공고를 확인하고 Greenhouse로 바로 지원** so that **끊김 없는 지원 경험을 가질 수 있다.** |
| US-07 | **P1** | As a **투자자/파트너**, I want **IR 페이지에서 재무자료·공시를 열람** so that **기업 신뢰도를 확인할 수 있다.** |
| US-08 | **P1** | As a **SEO 담당자**, I want **다국어 sitemap·hreflang이 자동 생성** so that **검색엔진에 각 언어 페이지가 올바르게 노출된다.** |
| US-09 | **P1** | As a **경영진**, I want **GA4 + 자체 대시보드로 월간 유입·전환 추이를 확인** so that **데이터 기반 의사결정을 할 수 있다.** |
| US-10 | **P1** | As a **방문 예약 고객**, I want **날짜·시간을 선택하여 방문 예약을 신청** so that **사전 조율 없이 방문 일정을 잡을 수 있다.** |

---

## 3. 기능 명세표

| 기능 | 우선순위 | 수용 기준 (Acceptance Criteria) | 담당 팀 |
|---|:---:|---|---|
| **다국어 시스템** | P0 | URL 프리픽스 분리, hreflang 자동 삽입, 3개 언어 동시 운영 확인 | Architect · FE |
| **제품 카탈로그** | P0 | 320 SKU 등록, 카테고리·스펙 필터 동작, 제품 비교(최대 3개) 정상, 제품별 PDF/CAD 다운로드 | Architect · FE · BE |
| **CMS (관리자)** | P0 | 비개발자가 제품·뉴스·채용 CRUD 가능, 이미지 업로드 및 다국어 필드 관리 | Architect · BE |
| **문의 · 방문 예약 폼** | P0 | 3개 언어 폼 전환, reCAPTCHA v3 적용, 슬랙 Webhook 알림 수신 확인 | FE · BE |
| **반응형 디자인** | P0 | 모바일·태블릿·데스크탑 3 breakpoint, 모바일 우선 | Designer · FE |
| **통합검색** | P0 | 한·영·중 통합 검색, 제품·뉴스·채용 결과 탭 분리, 검색어 하이라이트 | FE · BE |
| **채용 페이지** | P1 | 공고 목록·상세, Greenhouse API 연동 지원하기 버튼, 사내문화 페이지 | FE · BE |
| **뉴스 / IR** | P1 | 카테고리 필터(보도자료/공시/재무), PDF 첨부, 목록 페이지네이션 | FE · BE |
| **SEO 최적화** | P1 | 다국어 sitemap 자동 생성, 메타태그 CMS 편집, Lighthouse SEO 90+ | Architect · FE |
| **성능 · CDN** | P1 | LCP < 2.5s (글로벌 CDN), Lighthouse Performance 90+ | Architect |
| **보안** | P1 | HTTPS 전역, OWASP Top10 점검 통과, GDPR 동의 배너, 개인정보처리방침 | Architect · BE |
| **접근성 (WCAG 2.1 AA)** | P1 | axe-core 자동 점검 통과, 키보드 네비게이션, 스크린리더 호환 | Designer · FE |
| **분석 대시보드** | P1 | GA4 이벤트 세팅, 자체 대시보드에서 월간 유입·전환 리포트 조회 | BE · FE |

---

## 4. 데이터 항목

### 4-1. 제품 (Product)

| 필드 | 타입 | 비고 |
|---|---|---|
| product_id | UUID | PK |
| category_id | FK | 카테고리 연결 |
| name_ko / name_en / name_zh | Text | 다국어 |
| description_ko / description_en / description_zh | RichText | 다국어 |
| thumbnail | Image | 썸네일 |
| images | Image[] | 갤러리 |
| specs | JSON | 키-값 스펙 (예: 전압, 정밀도) |
| downloads | File[] | PDF, CAD 등 |
| sku | String | SKU 코드 |
| is_published | Boolean | 공개 여부 |
| created_at / updated_at | DateTime | |

### 4-2. 카테고리 (Category)

| 필드 | 타입 | 비고 |
|---|---|---|
| category_id | UUID | PK |
| parent_id | FK | nullable, 2depth |
| name_ko / name_en / name_zh | Text | |
| sort_order | Int | 정렬 |

### 4-3. 뉴스 / IR (Post)

| 필드 | 타입 | 비고 |
|---|---|---|
| post_id | UUID | PK |
| type | Enum | press / disclosure / financial |
| title_ko / title_en / title_zh | Text | |
| body_ko / body_en / body_zh | RichText | |
| attachments | File[] | PDF 등 |
| published_at | DateTime | |
| is_published | Boolean | |

### 4-4. 채용 공고 (JobPosting)

| 필드 | 타입 | 비고 |
|---|---|---|
| job_id | UUID | PK |
| title_ko / title_en / title_zh | Text | |
| department | String | |
| location | String | |
| employment_type | Enum | full-time / contract |
| description_ko / description_en / description_zh | RichText | |
| greenhouse_job_id | String | 외부 ATS 연동 키 |
| is_active | Boolean | |

### 4-5. 문의 (Inquiry)

| 필드 | 타입 | 비고 |
|---|---|---|
| inquiry_id | UUID | PK |
| type | Enum | general / visit |
| name | String | |
| company | String | nullable |
| email | Email | |
| phone | String | nullable |
| language | Enum | ko / en / zh |
| message | Text | |
| preferred_date | Date | 방문 예약 시 |
| preferred_time | String | 방문 예약 시 |
| gdpr_consent | Boolean | |
| created_at | DateTime | |

### 4-6. 사이트 설정 (SiteConfig)

| 필드 | 타입 | 비고 |
|---|---|---|
| meta_title_ko / en / zh | Text | 페이지별 메타 타이틀 |
| meta_description_ko / en / zh | Text | 페이지별 메타 설명 |
| og_image | Image | 소셜 공유 이미지 |

---

## 5. 마일스톤 안 (4주 단위 × 3)

### M1 — 기반 구축 (Week 1~4: 6/1 ~ 6/28)

- [ ] IA 확정 및 와이어프레임 승인
- [ ] 디자인 시안 5종 x 2(데스크탑/모바일) 제작·1차 피드백
- [ ] 기술 스택 확정, 프로젝트 셋업 (Repo · CI/CD · CDN)
- [ ] CMS 스키마 설계, 다국어 시스템 기본 구조 개발
- [ ] 기존 콘텐츠 마이그레이션 데이터 정제 착수 (320 SKU 분석)

**검증**: 와이어프레임 승인 완료, 개발 환경 구동, CMS에 샘플 제품 1건 다국어 등록 가능

### M2 — 핵심 기능 개발 (Week 5~8: 6/29 ~ 7/26)

- [ ] 디자인 시안 확정 및 퍼블리싱 완료
- [ ] 제품 카탈로그 (필터 · 비교 · 다운로드) 개발 완료
- [ ] 문의 · 방문 예약 폼 + 슬랙 알림 연동
- [ ] 채용 페이지 + Greenhouse API 연동
- [ ] 뉴스/IR CRUD 및 프론트 개발
- [ ] 통합검색 (한·영·중) 구현

**검증**: 주요 페이지 전체 동작, CMS에서 비개발자 테스트 CRUD 통과, Greenhouse 연동 확인

### M3 — 품질 · 런칭 (Week 9~12: 7/27 ~ 8/23)

- [ ] 320 SKU 전량 마이그레이션 완료
- [ ] GA4 이벤트 세팅 + 자체 대시보드 개발
- [ ] SEO 최적화 (sitemap · hreflang · 메타태그 · 영문 키워드)
- [ ] 성능 튜닝 (Lighthouse 90+ · LCP < 2.5s)
- [ ] 보안 점검 (OWASP Top10 · GDPR 배너)
- [ ] 접근성 점검 (WCAG 2.1 AA)
- [ ] QA 및 크로스브라우저·디바이스 테스트
- [ ] CMS 사용자 매뉴얼 작성 및 마케팅팀 교육

**검증**: Lighthouse 전 항목 90+, 보안 점검 리포트 클린, 마케팅팀 독립 운영 테스트 통과

> **참고**: M3 이후 나머지 기간(8/24~11/30)은 RFP 원문 6개월 사업 기간에 맞춰 UAT · 콘텐츠 최종 검수 · 스테이징/프로덕션 전환 · 안정화 · 인수인계 문서 작성에 할당합니다.

---

@Designer 위 IA·사용자 스토리를 기반으로 와이어프레임 및 디자인 컨셉을 작성해 주세요. 키엔스·지멘스 레퍼런스 톤을 반영하되, 모바일 우선 반응형으로 진행 부탁드립니다.

@Architect 기능 명세표·데이터 항목을 기반으로 기술 스택 선정 및 시스템 설계를 진행해 주세요. 다국어 URL 분리, Headless CMS 구조, CDN 전략을 포함해 주시기 바랍니다.
