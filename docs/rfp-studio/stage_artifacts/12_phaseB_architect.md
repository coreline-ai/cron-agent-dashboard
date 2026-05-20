# Stage 12 · @Architect · Phase B

시각: 2026-05-20T10:27:54.249688Z

---

# 한국정밀산업 글로벌 리뉴얼 — 상세 시스템 설계서

## 0. 설계 기준

- **프론트엔드**: Next.js App Router + Vercel, `/ko`, `/en`, `/zh` 라우팅
- **백엔드**: BFF API + CMS API + Search API 분리
- **DB**: Amazon RDS PostgreSQL
- **검색**: Amazon OpenSearch Service
- **미디어**: S3 + CloudFront
- **운영**: GitHub Actions, Sentry, GA4, reCAPTCHA, Slack Alert

---

## 1. 데이터베이스 스키마

### 1.1 ER 다이어그램

```txt
┌────────────┐
│  locales   │
└─────┬──────┘
      │ 1:N
      ▼
┌─────────────────────┐
│ localized_documents │◄──────────────┐
└─────────┬───────────┘               │ polymorphic
          │                           │ entity_type/entity_id
          │                           │
┌─────────▼──────────┐        ┌───────▼──────────┐
│ product_categories │ 1:N    │  content_items   │
└─────────┬──────────┘        │ news/ir/job/page │
          │                   └───────┬──────────┘
          │ 1:N                       │
          ▼                           │
┌────────────────────┐                │
│      products      │                │
└───────┬─────┬──────┘                │
        │     │                       │
        │1:N  │1:N                    │1:N
        ▼     ▼                       ▼
┌────────────┐  ┌────────────────────────────┐
│product_specs│ │       media_assets         │
└────────────┘  │ product/content attachment │
                └────────────────────────────┘

┌────────────┐ 1:N ┌───────────┐
│ cms_roles  │────►│ cms_users │
└────────────┘     └─────┬─────┘
                         │ created_by / assigned_to
                         ▼
                   ┌───────────┐
                   │ inquiries │
                   └───────────┘
```

### 1.2 주요 테이블 정의

| 테이블 | 주요 컬럼 | 인덱스 | 제약 / 비고 |
|---|---|---|---|
| `locales` | `code PK`, `name`, `is_active`, `sort_order`, `created_at` | `idx_locales_active` | 초기값 `ko`, `en`, `zh`; 향후 `ja`, `vi` 추가 |
| `cms_roles` | `id PK`, `name`, `permissions JSONB`, `created_at` | `uk_role_name`, `gin_permissions` | `admin`, `editor`, `translator`, `viewer` |
| `cms_users` | `id PK`, `role_id FK`, `email`, `password_hash`, `name`, `status`, `mfa_enabled`, `last_login_at` | `uk_user_email`, `idx_user_role` | 이메일 unique, 비밀번호 hash 저장, MFA 권장 |
| `product_categories` | `id PK`, `parent_id FK`, `slug`, `sort_order`, `is_active` | `uk_category_slug`, `idx_category_parent` | 다국어 명칭은 `localized_documents`에 저장 |
| `products` | `id PK`, `category_id FK`, `sku`, `model_no`, `status`, `comparable`, `launched_at`, `updated_at` | `uk_product_sku`, `idx_product_category_status` | `status`: `draft/review/published/archived` |
| `product_specs` | `id PK`, `product_id FK`, `spec_key`, `spec_value`, `unit`, `numeric_value`, `sort_order`, `filterable` | `idx_spec_product`, `idx_spec_filter` | 비교·필터용; 숫자 필터는 `numeric_value` 사용 |
| `media_assets` | `id PK`, `owner_type`, `owner_id`, `locale`, `asset_type`, `s3_key`, `url`, `mime_type`, `size_bytes`, `alt_text` | `uk_media_s3_key`, `idx_media_owner`, `idx_media_locale` | 제품 이미지, PDF, 뉴스/IR 첨부 통합 관리 |
| `content_items` | `id PK`, `type`, `category_slug`, `source_system`, `external_ref`, `status`, `publish_from`, `publish_to`, `metadata JSONB`, `created_by FK` | `idx_content_type_status_date`, `idx_content_external_ref` | `type`: `page/news/ir/job`; Greenhouse 공고는 `external_ref` |
| `localized_documents` | `id PK`, `entity_type`, `entity_id`, `locale FK`, `slug`, `title`, `summary`, `body_md`, `seo_title`, `seo_description`, `workflow_status`, `version`, `published_at` | `uk_locale_slug_type`, `idx_localized_entity`, `idx_localized_status`, `gin_search_vector` | 모든 다국어 콘텐츠의 단일 번역 테이블 |
| `inquiries` | `id PK`, `locale FK`, `type`, `company`, `contact_name`, `email`, `phone`, `country`, `message`, `preferred_date`, `consent_privacy`, `recaptcha_score`, `status`, `source_utm JSONB` | `idx_inquiry_status_date`, `idx_inquiry_locale_type` | 개인정보 암호화·마스킹, 보존기간 정책 적용 |

### 1.3 핵심 관계

| 관계 | 설명 |
|---|---|
| `product_categories.parent_id → product_categories.id` | 제품 카테고리 트리 구조 |
| `products.category_id → product_categories.id` | SKU는 1개 기본 카테고리에 속함 |
| `product_specs.product_id → products.id` | 제품별 스펙 필터·비교 |
| `localized_documents.entity_type/entity_id` | 제품, 카테고리, 뉴스, IR, 채용, 페이지의 다국어 문서 |
| `media_assets.owner_type/owner_id` | 제품 이미지, 다운로드 PDF, 뉴스/IR 첨부 통합 |
| `content_items.type = job` | 채용 공고, Greenhouse 외부 ATS URL 저장 |
| `cms_users.role_id → cms_roles.id` | CMS RBAC 권한 관리 |
| `inquiries.assigned_to → cms_users.id` | 문의 담당자 배정 선택 적용 |

---

## 2. API 엔드포인트 카탈로그

| # | 영역 | Method | Path | 요청 | 응답 | 인증 |
|---:|---|---|---|---|---|---|
| 1 | BFF | GET | `/api/bff/{locale}/navigation` | `locale` | GNB, footer, language switch | Public |
| 2 | BFF | GET | `/api/bff/{locale}/home` | `preview?` | hero, featured products, news | Public |
| 3 | BFF | GET | `/api/bff/{locale}/products` | `category`, `specs`, `page`, `sort` | product list, facets | Public |
| 4 | BFF | GET | `/api/bff/{locale}/products/{slug}` | `slug` | product detail, specs, media | Public |
| 5 | BFF | POST | `/api/bff/{locale}/products/compare` | `productIds[]` | comparison matrix | Public |
| 6 | BFF | GET | `/api/bff/{locale}/content` | `type`, `category`, `page` | news/IR/page list | Public |
| 7 | BFF | GET | `/api/bff/{locale}/content/{slug}` | `slug` | content detail, attachments | Public |
| 8 | BFF | GET | `/api/bff/{locale}/jobs` | `department`, `location` | job list, ATS links | Public |
| 9 | BFF | POST | `/api/bff/{locale}/inquiries` | form body, reCAPTCHA token | inquiry id, status | reCAPTCHA |
| 10 | CMS | POST | `/api/cms/auth/login` | email, password, OTP | JWT, refresh cookie | Public |
| 11 | CMS | GET | `/api/cms/products` | `status`, `q`, `category` | admin product list | `product.read` |
| 12 | CMS | POST | `/api/cms/products` | SKU, category, status | product id | `product.write` |
| 13 | CMS | PATCH | `/api/cms/products/{id}` | base product fields | updated product | `product.write` |
| 14 | CMS | PUT | `/api/cms/products/{id}/specs` | `specs[]` | bulk upsert result | `product.write` |
| 15 | CMS | POST | `/api/cms/media` | multipart file, metadata | media asset | `media.write` |
| 16 | CMS | GET | `/api/cms/content` | `type`, `status`, `locale` | content list | `content.read` |
| 17 | CMS | POST | `/api/cms/content` | type, metadata | content id | `content.write` |
| 18 | CMS | PATCH | `/api/cms/content/{id}` | metadata, status | updated content | `content.write` |
| 19 | CMS | PUT | `/api/cms/localizations/{entityType}/{entityId}/{locale}` | title, body, SEO, slug | draft localization | `translation.write` |
| 20 | CMS | POST | `/api/cms/localizations/{entityType}/{entityId}/{locale}/publish` | schedule, version | publish event | `publish.approve` |
| 21 | CMS | GET | `/api/cms/inquiries` | status, type, date range | inquiry list | `inquiry.read` |
| 22 | CMS | PATCH | `/api/cms/users/{id}/role` | role id, status | updated user | `admin.user` |
| 23 | Search | GET | `/api/search` | `q`, `locale`, `type`, `page` | hits, facets, highlights | Public |
| 24 | Search | GET | `/api/search/suggest` | `q`, `locale` | autocomplete terms | Public |
| 25 | Search | POST | `/api/search/reindex` | entity type, ids, locales | job id | Internal/Admin |

---

## 3. 다국어 콘텐츠 흐름

```txt
Editor
  │
  │ 1. 원문 등록/수정
  ▼
CMS UI
  │
  │ 2. content_items/products + localized_documents(ko, draft) 저장
  ▼
CMS API ───────────────► PostgreSQL
  │
  │ 3. en/zh 번역 작업 생성
  ▼
Translation Queue
  │
  │ 4. 번역가/검수자 작업
  ▼
Translator / Reviewer
  │
  │ 5. 번역본 저장, workflow_status = reviewed
  ▼
CMS API ───────────────► PostgreSQL
  │
  │ 6. 발행 승인
  ▼
Publisher
  │
  │ 7. locale별 published version 생성
  ▼
CMS API
  │
  ├────────► PostgreSQL
  │          - workflow_status = published
  │          - published_at 기록
  │          - slug unique 검증
  │
  ├────────► Search Indexer
  │          - locale별 문서 수집
  │          - product specs/facets 병합
  │          - OpenSearch upsert
  │
  └────────► CDN Invalidation Job
             - /{locale}/products/*
             - /{locale}/news-ir/*
             - /{locale}/sitemap.xml
             - hreflang sitemap
             - search suggestion cache

Search Indexer ───────► OpenSearch
CDN Job ──────────────► Vercel / CloudFront Purge
User ─────────────────► 최신 콘텐츠 조회
```

### 운영 원칙

| 항목 | 정책 |
|---|---|
| 발행 단위 | `locale`별 독립 발행 |
| SEO | locale별 slug, canonical, hreflang 생성 |
| 번역 누락 | 공개 페이지에서는 미발행 처리, CMS에서 누락 경고 |
| 검색 색인 | 발행 이벤트 기반 증분 색인 + 야간 전체 재색인 |
| 캐시 | 상세 페이지, 목록, sitemap, 검색 suggest 캐시 무효화 |

---

## 4. 배포 파이프라인

### 4.1 GitHub Actions 단계

```txt
Pull Request
  │
  ├─ 1. checkout / dependency cache
  ├─ 2. lint / format / typecheck
  ├─ 3. unit test
  ├─ 4. i18n key validation
  ├─ 5. build Next.js / CMS API
  ├─ 6. DB migration dry-run
  ├─ 7. security scan
  │     - CodeQL
  │     - dependency audit
  │     - secret scan
  │     - container scan
  ├─ 8. Playwright smoke / Lighthouse budget
  ├─ 9. preview deploy
  └─ 10. PR status report

main merge
  │
  ├─ deploy to dev
  ├─ integration test
  ├─ deploy to staging
  ├─ QA approval gate
  ├─ production migration
  ├─ production deploy
  ├─ smoke test
  ├─ Sentry release tagging
  └─ Slack notification
```

### 4.2 환경 분리 전략

| 환경 | 트리거 | 인프라 | 데이터 | 접근 제어 |
|---|---|---|---|---|
| `dev` | feature branch / PR | Vercel Preview, dev API | seed 데이터 | 개발팀 |
| `stg` | `main` merge | staging domain, staging API | 마스킹된 운영 복제본 | 개발팀 + 고객 QA |
| `prod` | release tag + 승인 | production domain, prod API | 운영 DB Multi-AZ | 최소 권한, MFA 필수 |

### 4.3 배포 원칙

| 항목 | 전략 |
|---|---|
| 프론트엔드 | Vercel immutable deployment, 즉시 rollback |
| CMS/Search API | AWS Lambda 또는 컨테이너 이미지 버전 배포 |
| DB Migration | expand → deploy → contract 3단계 |
| Secret | GitHub OIDC + AWS Secrets Manager + Vercel Env |
| 캐시 | 배포 후 sitemap, 주요 랜딩, 제품 목록 prewarm |
| 관측성 | Sentry release, CloudWatch metric, GA4 이벤트 검증 |

---

## 5. 장애 대응 플레이북

### 5.1 등급별 RTO/RPO

| 등급 | 시나리오 | RTO | RPO | 알림 |
|---|---|---:|---:|---|
| P1 | 전체 사이트 장애, DB 장애, 개인정보 유출 의심, 문의 접수 불가 | 30분 | 15분 | Sentry Critical, CloudWatch Alarm, Slack `#incident-p1`, SMS |
| P2 | CMS 발행 실패, 검색 장애, 특정 locale 5xx, LCP 급락 | 4시간 | 1시간 | Slack `#incident`, Sentry High |
| P3 | 단일 콘텐츠 오류, UI 깨짐, 비핵심 기능 오류 | 영업일 1일 | 24시간 | Jira/Linear ticket, Slack daily triage |

### 5.2 공통 대응 절차

```txt
1. 감지
   - health check, 5xx rate, latency, Sentry issue, 고객 신고

2. 분류
   - P1/P2/P3 판정
   - incident commander 지정

3. 완화
   - Vercel 이전 배포 rollback
   - API 이전 이미지 rollback
   - WAF rule 임시 적용
   - 검색 장애 시 DB fallback 검색 제한 제공

4. 복구
   - DB PITR 또는 snapshot restore
   - OpenSearch 재색인
   - CDN cache purge
   - smoke test

5. 커뮤니케이션
   - 내부 Slack 업데이트
   - 고객 담당자 보고
   - P1은 30분 단위 상황 공유

6. 사후 분석
   - 48시간 내 postmortem
   - 원인, 영향 범위, 재발 방지 액션 기록
```

### 5.3 주요 장애별 롤백

| 장애 | 1차 조치 | 2차 조치 |
|---|---|---|
| 프론트 배포 오류 | Vercel previous deployment rollback | 캐시 purge, smoke 재실행 |
| API 오류 | 이전 Lambda/container version rollback | feature flag off |
| DB migration 오류 | migration lock 확인, reversible down migration | PITR 새 인스턴스 복구 후 endpoint 전환 |
| 검색 장애 | OpenSearch read replica/재시작 | DB 기반 제한 검색 임시 제공 |
| 미디어 장애 | S3 origin 확인 | CloudFront invalidation, signed URL 재발급 |
| 문의 알림 누락 | DB 접수분 재전송 | Slack webhook rotate |

---

## 6. 월 운영비 추정

### 6.1 산정 가정

| 항목 | 기준 |
|---|---|
| 월 트래픽 | 50만 PV, 200만 API/edge request |
| 미디어 | S3 150GB, 월 CDN 전송 300GB |
| 관리자 | CMS 사용자 5명 |
| 검색 | 3개 locale, 320개 SKU, 뉴스/IR/채용 포함 |
| 리전 | AWS Seoul 기준 추정, 세금/VAT 제외 |
| 제외 | Greenhouse 유료 라이선스, Slack 유료 플랜, GA4 360 |

### 6.2 비용 표

| 구분 | 항목 | 월 추정 |
|---|---|---:|
| AWS | RDS PostgreSQL Multi-AZ, 100GB, backup | `$230` |
| AWS | RDS Proxy, Secrets Manager, KMS | `$25` |
| AWS | Lambda/API Gateway — CMS/Search jobs | `$30` |
| AWS | OpenSearch managed small cluster + EBS | `$120` |
| AWS | S3 media/log storage | `$6` |
| AWS | CloudFront media CDN + Route 53 | `$35` |
| AWS | AWS WAF managed rules | `$18` |
| AWS | CloudWatch logs, metrics, alarms, SNS | `$30` |
| AWS | Backup, snapshot, misc data transfer buffer | `$40` |
| **AWS 소계** |  | **`$534`** |
| Vercel | Pro plan + 기본 사용량 내 운영 | `$20` |
| **Vercel 소계** |  | **`$20`** |
| SaaS | reCAPTCHA Standard | `$8` |
| SaaS | GA4 Standard | `$0` |
| SaaS | Sentry Team | `$26` |
| SaaS | Slack notification webhook | `$0` |
| **SaaS 소계** |  | **`$34`** |
| **총합** | 기준 운영비 | **`$588 / month`** |
| **권장 예산** | 20% 버퍼 포함 | **`약 $700 / month`** |

### 6.3 비용 기준 출처

- Vercel Pro 및 포함 사용량: [Vercel Pricing](https://vercel.com/pricing)
- Sentry Team: [Sentry Pricing](https://sentry.io/pricing/)
- reCAPTCHA Standard: [Google Cloud reCAPTCHA](https://cloud.google.com/security/products/recaptcha?pricing=)
- AWS RDS PostgreSQL: [AWS RDS PostgreSQL Pricing](https://aws.amazon.com/rds/postgresql/pricing/)
- AWS OpenSearch: [AWS OpenSearch Pricing](https://aws.amazon.com/opensearch-service/pricing/)
- AWS S3: [AWS S3 Pricing](https://aws.amazon.com/s3/pricing/)
- AWS CloudFront: [AWS CloudFront Pricing](https://aws.amazon.com/cloudfront/pricing/)
- AWS WAF: [AWS WAF Pricing](https://aws.amazon.com/waf/pricing/)

@QA
