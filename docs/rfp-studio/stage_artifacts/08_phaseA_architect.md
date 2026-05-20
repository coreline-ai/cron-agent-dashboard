# Stage 08 · @Architect · Phase A

시각: 2026-05-19T13:12:07.844481Z

---

# Architect 산출물 — 한국정밀산업 글로벌 리뉴얼

> 기준 확인: 2026-05-19. OWASP는 최신 공개판인 **OWASP Top 10:2025** 기준으로 설계합니다.

---

## 1) 기술 스택

| 영역 | 후보 | 선정 | 선정 사유 |
|---|---|---|---|
| 프론트엔드 | Next.js / Astro / Nuxt | **Next.js 16 + TypeScript** | App Router 기반 다국어 URL, SSG/ISR, SEO·성능 최적화에 유리 |
| UI | Tailwind / CSS Modules / MUI | **Tailwind + 디자인 토큰** | 와이어프레임을 빠르게 컴포넌트화, 반응형·접근성 일관성 확보 |
| 백엔드/CMS | Strapi / Directus / Custom NestJS | **Strapi 5 + Node BFF** | 비개발자 CMS, i18n, RBAC, Draft/Publish, 커스텀 API 확장 가능 |
| 검색 | OpenSearch / Algolia / Meilisearch | **Amazon OpenSearch** | 한·영·중 검색 품질, CJK/ICU 분석기, 운영·보안 통합 |
| 데이터베이스 | PostgreSQL / MySQL / MongoDB | **Aurora PostgreSQL 또는 RDS PostgreSQL** | 정형 제품·IR·문의 데이터와 JSONB 스펙 필드 동시 처리 |
| 파일 저장 | S3 / Cloudinary / CMS 로컬 | **S3 + CloudFront** | PDF/CAD/이미지 대용량 파일 안정 저장, CDN 캐싱 |
| 인프라 | Vercel+AWS / Full AWS / SaaS CMS | **Vercel Front + AWS Backend** | 글로벌 엣지 성능과 한국 리전 데이터 운영을 분리 최적화 |

보조 연동: Greenhouse Job Board API, GA4/Data API, Slack Webhook, reCAPTCHA Enterprise 또는 v3.

---

## 2) 시스템 아키텍처

```text
[Global Users / Buyers / Candidates / Search Bots]
                         |
                   [DNS / TLS / WAF]
                         |
              [Vercel Edge CDN + Next.js]
        ┌────────────────┼────────────────┐
        |                |                |
   SSG/ISR Pages     BFF API Routes    GA4 Events
        |                |                |
        |                v                v
        |        [Integration Layer]   [Analytics Store]
        |          |      |      |
        |          |      |      └── Slack 알림
        |          |      └──────── Greenhouse ATS
        |          └────────────── reCAPTCHA 검증
        |
        v
[Strapi Headless CMS / Admin]
        |
 ┌──────┼──────────────┬──────────────┐
 |      |              |              |
 v      v              v              v
[RDS/Aurora PG]   [S3 Assets]   [OpenSearch]   [Webhook Worker]
                                  ^             |
                                  └── Index Sync┘
```

### 컴포넌트 설명

| 컴포넌트 | 역할 |
|---|---|
| Next.js Web | `/ko`, `/en`, `/zh` 다국어 라우팅, 제품·채용·IR 화면, SEO 메타·sitemap·hreflang 생성 |
| BFF API | 폼 제출, ATS/Slack/GA4 서버 연동, CMS API 보호용 중간 계층 |
| Strapi CMS | 제품, 뉴스/IR, 페이지 콘텐츠, 다국어 번역, 관리자 권한 관리 |
| PostgreSQL | 제품·콘텐츠·문의·분석 요약 데이터 저장 |
| S3/CloudFront | PDF, CAD, 이미지, 보도자료 첨부파일 저장·배포 |
| OpenSearch | 한·영·중 통합검색, 제품 스펙/뉴스/페이지 색인 |
| Webhook Worker | CMS 발행 시 ISR 재검증, 검색 인덱스 갱신, 캐시 무효화 |
| Observability | Vercel Analytics, CloudWatch, Sentry, GA4 기반 장애·성능 추적 |

---

## 3) 데이터 모델

| 엔티티 | 주요 필드 | 관계 |
|---|---|---|
| Locale | code, name, default_flag | Page/Product/Article 번역 기준 |
| Page | id, type, slug, locale, title, body, seo_meta | Locale N:1 |
| ProductCategory | id, parent_id, slug, locale_name | Product 1:N, 자기참조 트리 |
| Product | id, sku, category_id, status, compare_enabled | Category N:1, Spec/Asset 1:N |
| ProductSpec | id, product_id, key, value, unit, filterable | Product N:1 |
| Asset | id, type, url, filename, locale, alt_text | Product/Article/Page와 N:M |
| NewsIRPost | id, type, locale, title, body, published_at | Asset N:M, Locale N:1 |
| JobPostingCache | greenhouse_id, locale, title, location, apply_url | Greenhouse API 동기화 캐시 |
| InquiryRequest | id, type, locale, payload, consent_at, status | Slack 알림, 개인정보 보관정책 적용 |
| AnalyticsMonthlyMetric | month, locale, source, visitors, conversions | GA4 집계 데이터 저장 |

---

## 4) 성능·보안 요구사항

### SLA / 성능

| 항목 | 목표 |
|---|---|
| Public Site 가용성 | 월 99.9% |
| CMS/API 가용성 | 월 99.5% |
| LCP | 글로벌 CDN 기준 2.5초 미만 |
| Lighthouse | Performance/SEO/Accessibility 90+ |
| API 응답 | p95 500ms 이하 |
| 검색 응답 | p95 300ms 이하 |
| 콘텐츠 반영 | CMS 발행 후 5분 이내 |
| 백업 | DB PITR, RPO 15분 / RTO 4시간 |

### OWASP Top 10:2025 대응

| 위험 | 대응 |
|---|---|
| Broken Access Control | CMS RBAC, API 서버 권한검사, 관리자 MFA |
| Security Misconfiguration | IaC 표준화, 보안 헤더, CSP, 환경별 Secret 분리 |
| Software Supply Chain Failures | lockfile, SCA, SBOM, Dependabot, 이미지 스캔 |
| Cryptographic Failures | TLS, KMS, DB/S3 암호화, 개인정보 로그 금지 |
| Injection | ORM/파라미터 바인딩, 입력 검증, 파일 업로드 제한 |
| Insecure Design | 위협 모델링, Abuse Case, 폼 rate limit |
| Authentication Failures | SSO/MFA, 세션 만료, 관리자 IP 제한 옵션 |
| Integrity Failures | 서명된 webhook, 승인 기반 배포, 감사 로그 |
| Logging/Alerting Failures | CloudWatch/Sentry 알림, 보안 이벤트 대시보드 |
| Exceptional Conditions | 사용자 친화 오류, 스택트레이스 비노출, 회로차단 |

추가: GDPR/개인정보보호법 대응을 위해 동의 이력, 보관 기간, 삭제 요청 프로세스, 쿠키 배너를 포함합니다.

---

## 5) 인력·일정 추정

| 역할 | 6월 발견/IA | 6~7월 설계/디자인 | 7~9월 구축 | 8~10월 콘텐츠/SEO | 11월 QA/런칭 | 유지보수 | 합계 |
|---|---:|---:|---:|---:|---:|---:|---:|
| PM/Architect | 0.8 | 0.6 | 0.7 | 0.5 | 0.5 | 0.3 | 3.4 |
| UX/UI Designer | 0.4 | 1.5 | 0.8 | 0.3 | 0.2 | 0.0 | 3.2 |
| Frontend | 0.2 | 0.5 | 3.0 | 1.0 | 0.8 | 0.1 | 5.6 |
| Backend/CMS | 0.2 | 0.5 | 2.8 | 1.1 | 0.7 | 0.2 | 5.5 |
| Data/SEO/i18n | 0.1 | 0.3 | 0.5 | 1.7 | 0.5 | 0.1 | 3.2 |
| QA | 0.0 | 0.1 | 0.4 | 0.5 | 1.2 | 0.1 | 2.3 |
| DevOps/Security | 0.1 | 0.2 | 0.5 | 0.3 | 0.5 | 0.1 | 1.7 |
| **합계** | **1.8** | **3.7** | **8.7** | **5.4** | **4.4** | **0.9** | **24.9 PM** |

---

## 6) CI/CD·배포 전략

| 환경 | 용도 | 배포 방식 |
|---|---|---|
| Local | 개발자 로컬 | Docker Compose, mock env |
| Dev | 통합 개발 | develop 브랜치 자동 배포 |
| Preview | PR 검수 | Vercel Preview + 임시 CMS 데이터 |
| Staging | UAT/콘텐츠 검수 | 운영과 동일 구성, 승인 배포 |
| Production | 실서비스 | main 태그 기반 승인 배포 |

### 자동화 흐름

1. PR 생성 → lint/typecheck/unit test/build 실행  
2. Storybook/Playwright 접근성·반응형 스모크 테스트  
3. SCA/Secret Scan/Docker 이미지 취약점 검사  
4. Frontend는 Vercel Preview → 승인 후 Production 승격  
5. Backend는 ECR → ECS Blue/Green 또는 Rolling 배포  
6. DB migration은 backward-compatible 원칙, Staging 검증 후 승인 적용  
7. CMS publish webhook → ISR revalidate → OpenSearch index sync  
8. 장애 시 Vercel instant rollback, ECS 이전 task definition 복구

### 참고 기준

- [Next.js Internationalization](https://nextjs.org/docs/app/guides/internationalization)
- [Strapi Internationalization](https://docs.strapi.io/cms/features/internationalization)
- [OpenSearch ICU Analyzer](https://docs.opensearch.org/latest/analyzers/language-analyzers/icu/)
- [Greenhouse Job Board API](https://developers.greenhouse.io/job-board)
- [AWS S3 + CloudFront Guidance](https://docs.aws.amazon.com/prescriptive-guidance/latest/patterns/serve-static-content-in-an-amazon-s3-bucket-through-a-vpc-by-using-cloudfront.html)
- [OWASP Top 10:2025](https://owasp.org/Top10/2025/)

@QA 위임
