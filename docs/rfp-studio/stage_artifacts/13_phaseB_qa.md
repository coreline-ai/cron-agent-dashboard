# Stage 13 · @QA · Phase B

시각: 2026-05-20T10:31:07.750561Z

---

# QA 통합 계획 — 한국정밀산업 글로벌 리뉴얼

## 전제

- 대상: 글로벌 코퍼레이트 사이트 `/ko`, `/en`, `/zh`
- 주요 화면: 메인, 회사소개, 제품, 채용, 문의
- 설계 기준: Next.js App Router, Vercel, BFF/CMS/Search API, PostgreSQL, OpenSearch, S3+CloudFront, GA4, Sentry, reCAPTCHA, Slack Alert
- QA 목표: 글로벌 바이어 신뢰도, 다국어 운영 안정성, 제품 탐색 전환, CMS 운영성, 보안·성능·접근성 출시 기준 확보

---

## 1) 테스트 전략

| 구분 | 범위 | 도구 / 방식 | 주요 산출물 |
|---|---|---|---|
| 단위 테스트 | UI 컴포넌트, 폼 검증, i18n 라우팅, 유틸, CMS 권한 로직 | Vitest/Jest, React Testing Library, schema validator | 컴포넌트 테스트 리포트, 커버리지 |
| 통합 테스트 | BFF-CMS-DB, Search API-OpenSearch, S3 PDF, reCAPTCHA, Slack, GA4, Greenhouse ATS | API contract test, MSW, Testcontainers, Postman/Newman | API 계약서, 연동 테스트 결과 |
| E2E 테스트 | 바이어 제품 탐색, 문의, CMS 발행, 채용 지원, 검색, 뉴스/IR 다운로드 | Playwright, Chrome/Safari/Firefox, 모바일 뷰포트 | E2E 자동화 리포트 |
| 성능 테스트 | LCP, CLS, INP, TTFB, API p95, 이미지/CDN 캐시 | Lighthouse CI, WebPageTest, Vercel Analytics, APM | 성능 SLI/SLO 대시보드 |
| 접근성 테스트 | WCAG 2.1 AA, 키보드 탐색, 명도, 스크린리더 | axe, Lighthouse, 수동 스크린리더 검수 | 접근성 결함 목록 |
| 보안 테스트 | OWASP Top 10, 인증/인가, 입력 검증, 개인정보 처리 | SAST, DAST/ZAP, dependency scan, 권한 테스트 | 보안 점검표, 취약점 조치 내역 |
| 수동 검수 / UAT | 마케팅팀 CMS, 글로벌 바이어, 지원자, 운영자 | 체크리스트 기반 세션, 이슈 워크숍 | UAT 승인서, 잔여 리스크 목록 |

---

## 2) 수용 기준 매트릭스

| 기능 | Pass 조건 | 검증 방법 |
|---|---|---|
| 다국어 라우팅 | `/ko`, `/en`, `/zh` 경로가 분리되고 언어 전환 시 동일 콘텐츠 맥락 유지 | E2E, sitemap/hreflang 검사 |
| 제품 카탈로그 | 320개 SKU 마이그레이션, 카테고리·스펙 필터·비교·다운로드 정상 동작 | 데이터 샘플링, E2E, API 테스트 |
| CMS | 비개발자가 제품/뉴스/IR/페이지를 등록·수정·미리보기·발행 가능 | UAT, 권한별 시나리오 |
| CMS 권한 | admin/editor/translator/viewer 권한이 CRUD 범위대로 제한 | 권한 테스트, API 인가 테스트 |
| 통합 검색 | 한·영·중 키워드로 제품/뉴스/IR/페이지 검색 가능 | 검색 정확도 샘플 테스트 |
| 채용 | 공고 목록·상세·Greenhouse 지원 이동 정상 | E2E, 외부 연동 테스트 |
| 뉴스/IR | 카테고리, PDF 첨부, 발행일, 다국어 메타 정보 정상 | CMS 발행 테스트 |
| 문의/방문 예약 | 3개 언어 폼, 유효성 검증, reCAPTCHA, Slack 알림 정상 | E2E, 알림 수신 확인 |
| 분석 | GA4 이벤트와 자체 대시보드의 월간 유입·전환 지표 일치 | GA DebugView, 대시보드 검산 |
| SEO | 다국어 sitemap, canonical, hreflang, 메타 태그 적용 | 크롤러, Search Console 사전 검사 |
| 성능 | 주요 페이지 LCP < 2.5s, Lighthouse 90+ | Lighthouse CI, 필드 데이터 |
| 보안/개인정보 | HTTPS, 보안 헤더, 개인정보 동의/처리방침, GDPR 대응 근거 확보 | 보안 점검, 문서 검수 |
| 접근성 | WCAG 2.1 AA 주요 항목 충족, 키보드 이용 가능 | axe, 수동 검수 |
| 반응형 | 모바일/태블릿/데스크탑에서 핵심 CTA와 정보 구조 유지 | 크로스 디바이스 QA |

---

## 3) E2E 테스트 시나리오 10개

| ID | 사용자 여정 | 단계 | 기대 결과 | 우선순위 |
|---|---|---|---|---|
| E2E-01 | 글로벌 바이어 첫 방문 | `/en` 접속 → 메인 CTA → 제품 카테고리 이동 | 영어 콘텐츠, CTA, 네비게이션 정상 | P0 |
| E2E-02 | 제품 탐색/필터 | 제품 목록 → 카테고리 선택 → 스펙 필터 → 상세 진입 | 필터 결과 정확, URL 상태 유지 | P0 |
| E2E-03 | 제품 비교 | 제품 2~3개 선택 → 비교 보기 → PDF 다운로드 | 비교 표와 다운로드 정상 | P0 |
| E2E-04 | 다국어 전환 | 제품 상세에서 KO/EN/ZH 전환 | 동일 SKU 상세로 이동, 혼합 언어 없음 | P0 |
| E2E-05 | 통합 검색 | 한·영·중 키워드 검색 → 제품/뉴스 결과 확인 | 관련 결과 노출, 빈 결과 UX 제공 | P0 |
| E2E-06 | 문의 제출 | 문의 폼 입력 → reCAPTCHA → 제출 | 완료 메시지, Slack 알림, 로그 저장 | P0 |
| E2E-07 | 채용 지원 | 채용 목록 → 상세 → Greenhouse 지원 클릭 | 외부 ATS 정상 이동, 추적 이벤트 발생 | P1 |
| E2E-08 | CMS 제품 발행 | editor 로그인 → 제품 수정 → preview → publish | 프론트 반영, 검색 인덱스 갱신 | P0 |
| E2E-09 | 뉴스/IR 발행 | CMS에서 IR PDF 등록 → 발행 → 다운로드 | 카테고리/첨부/메타 정상 | P1 |
| E2E-10 | 운영 회귀 | 배포 후 메인/제품/문의 smoke test | 핵심 경로 무중단, Sentry 오류 없음 | P0 |

---

## 4) UAT 체크리스트

| 페르소나 | 체크 항목 | 승인 기준 |
|---|---|---|
| 마케팅팀 CMS | 로그인/MFA, 제품 CRUD, 다국어 입력, 이미지/PDF 업로드, 미리보기, 예약 발행, 롤백 | 비개발자가 매뉴얼만으로 주요 콘텐츠 발행 가능 |
| 글로벌 바이어 | 언어 선택, 제품 필터, 비교, 다운로드, 문의, 모바일 사용성 | 5분 내 관심 제품 탐색 및 문의 가능 |
| 지원자 | 채용 목록/상세, 문화·복지 페이지, ATS 이동, 모바일 지원 흐름 | 지원 CTA가 명확하고 외부 ATS 전환 실패 없음 |
| 운영자/인프라 | 배포, 롤백, CDN purge, 알림, Sentry, 백업/복구, 권한 관리 | 장애 대응 Runbook으로 재현 가능 |

---

## 5) 다국어 QA 매트릭스 — 30셀

| 화면 | KO Desktop | KO Mobile | EN Desktop | EN Mobile | ZH Desktop | ZH Mobile |
|---|---|---|---|---|---|---|
| 메인 | 한국어 카피/CTA/CI 정합성 | 360px 줄바꿈, 햄버거 | 글로벌 톤, SEO title | CTA 터치 영역 | 간체 번역/폰트 | 중국어 줄바꿈 |
| 회사소개 | 연혁/지사 정보 정확 | 타임라인 가독성 | 수출/글로벌 메시지 | 카드 스택 정상 | 고유명사 표기 | 긴 문장 overflow 없음 |
| 제품 | 카테고리/스펙 용어 | 필터 조작성 | SKU/spec 영문 단위 | 비교표 스크롤 | 기술용어 간체 | 필터 UI 깨짐 없음 |
| 채용 | 복지/문화 문구 | 공고 CTA 노출 | ATS 이동 문구 | 외부 이동 안내 | 채용 번역 일관성 | CTA 터치 정상 |
| 문의 | 개인정보 동의 | 입력 필드 키보드 | GDPR 문구 | 국가/이메일 입력 | 중국어 폼 라벨 | 완료 메시지 정상 |

---

## 6) 성능 SLI/SLO

| 지표 | SLO 목표 | 측정 방법 | 알림 임계 |
|---|---:|---|---|
| LCP | p75 < 2.5s | Lighthouse CI, CrUX/PageSpeed, RUM | 경고 2.5s 초과, 차단 3.0s 초과 |
| CLS | p75 ≤ 0.1 | Lighthouse, RUM layout shift | 경고 0.1 초과, 차단 0.2 초과 |
| INP | p75 ≤ 200ms | Web Vitals RUM | 경고 200ms 초과, 차단 300ms 초과 |
| TTFB | p75 ≤ 800ms | CDN/APM, synthetic monitor | 경고 800ms 초과, 차단 1.2s 초과 |
| API p95 | BFF 500ms 이하, Search 700ms 이하 | APM, k6 smoke, 서버 로그 | p95 30분 지속 초과 시 알림 |
| Lighthouse | 주요 템플릿 90+ | CI PR gate | 90 미만 release block |

---

## 7) 보안 점검 리스트

| 영역 | 점검 항목 | 증빙 |
|---|---|---|
| A01 접근통제 | CMS 역할별 CRUD 제한, API 객체 권한 검증 | 권한 테스트 결과 |
| A02 암호화 실패 | HTTPS/HSTS, RDS/S3 암호화, secret 미노출 | 인프라 설정 캡처 |
| A03 Injection | SQL 파라미터화, 검색 쿼리 escape, 업로드 검증 | SAST/DAST 결과 |
| A04 안전하지 않은 설계 | 문의 spam, CMS 탈취, 파일 업로드 abuse case 검토 | Threat model |
| A05 보안 설정 오류 | CORS, CSP, 보안 헤더, 관리자 경로 보호 | 헤더 스캔 결과 |
| A06 취약 구성요소 | npm/package 취약점, 이미지 스캔 | dependency scan |
| A07 인증 실패 | MFA, 세션 만료, secure cookie, lockout | 인증 테스트 |
| A08 무결성 실패 | CI/CD 승인, 배포 권한, webhook 검증 | 배포 정책 |
| A09 로깅/모니터링 | 인증 실패, CMS 변경, 문의 제출 로그와 알림 | Sentry/로그 샘플 |
| A10 SSRF | 외부 URL fetch 제한, Slack/ATS webhook allowlist | 네트워크 정책 |
| 개인정보보호법 | 수집 목적, 항목, 보유기간, 처리위탁, 파기 기준 명시 | 개인정보처리방침 |
| GDPR | EU 이용자 대상 문의 시 동의, 권리 요청, 이전 근거, breach 대응 | GDPR 체크리스트 |

---

## 8) 리스크 매트릭스

| 리스크 | 영향도 | 가능성 | 완화 방안 |
|---|---|---|---|
| 320개 SKU 마이그레이션 오류 | 상 | 중 | 샘플링+전수 스크립트 검증, SKU checksum |
| 다국어 번역 품질 불일치 | 상 | 중 | 용어집, 번역 리뷰 워크플로우, locale diff |
| OpenSearch 인덱스 누락 | 상 | 중 | 발행 이벤트 재시도, nightly reindex, 검색 smoke |
| 성능 목표 미달 | 상 | 중 | 이미지 최적화, ISR/CDN, Lighthouse PR gate |
| CMS 권한 오동작 | 상 | 중 | RBAC 자동화 테스트, audit log |
| Greenhouse 연동 장애 | 중 | 중 | fallback 안내, 외부 상태 모니터링 |
| 개인정보/GDPR 누락 | 상 | 중 | 법무 리뷰, 동의 로그, 보유기간 정책 |
| 디자인 승인 지연 | 중 | 중 | 화면별 QA 기준 조기 합의 |
| 모바일 사용성 저하 | 상 | 중 | mobile-first QA, 실제 기기 테스트 |
| Slack/reCAPTCHA 장애 | 중 | 하 | 장애 fallback, 관리자 이메일 백업 알림 |

---

## 9) 회귀 테스트 시나리오 — 핵심 5개 골든 패스

| ID | 골든 패스 | 포함 검증 |
|---|---|---|
| REG-01 | 글로벌 바이어가 `/en`에서 제품 검색→필터→비교→다운로드 | 다국어, 제품, 검색, 다운로드 |
| REG-02 | CMS에서 제품 수정 후 발행→프론트/검색 반영 | CMS, DB, OpenSearch, 캐시 |
| REG-03 | 문의 폼 제출→reCAPTCHA→Slack 알림→GA4 전환 | 폼, 보안, 알림, 분석 |
| REG-04 | 채용 상세→Greenhouse 지원 이동 | 채용, ATS, 추적 이벤트 |
| REG-05 | 뉴스/IR PDF 등록→다국어 페이지 노출→SEO 메타 확인 | CMS, 첨부, SEO, 다국어 |

---

## 10) 품질 게이트 — 출시 차단 조건

| 등급 | 차단 조건 |
|---|---|
| Blocker | P0 E2E 실패, 제품 탐색/문의/CMS 발행 불가 |
| Blocker | Critical/High 보안 취약점 미조치 |
| Blocker | 개인정보 동의/처리방침/GDPR 핵심 고지 누락 |
| Blocker | 주요 페이지 Lighthouse 90 미만 또는 LCP 3.0s 초과 |
| Blocker | 다국어 라우팅, hreflang, sitemap 중대 오류 |
| Blocker | 320개 SKU 중 필수 제품 데이터 누락률 1% 초과 |
| Blocker | 모바일 핵심 CTA 사용 불가 |
| Blocker | 배포 롤백 절차 미검증 |
| Major | 접근성 AA 핵심 항목 미충족 |
| Major | Sentry 신규 high-volume 오류 지속 발생 |

---

## 11) 출시 D-1 체크리스트

1. Production 환경 변수와 secret 최종 확인  
2. DNS, SSL, HSTS 확인  
3. `/ko`, `/en`, `/zh` 주요 페이지 smoke test  
4. sitemap, robots.txt, canonical, hreflang 확인  
5. 제품 SKU 수량·필수 필드 최종 검증  
6. OpenSearch 전체 reindex 완료  
7. CMS 관리자 계정/MFA/권한 확인  
8. 문의 폼, reCAPTCHA, Slack 알림 확인  
9. Greenhouse ATS 링크 확인  
10. GA4 이벤트와 전환 DebugView 확인  
11. Lighthouse CI 주요 템플릿 90+ 확인  
12. Sentry release 연결 및 alert rule 확인  
13. CDN cache purge/rollback 리허설 완료  
14. 개인정보처리방침, 쿠키/동의 문구 게시 확인  
15. Go/No-Go 회의에서 잔여 P0/P1 이슈 0건 확인  

---

## 12) 검수 일정 안

| 기간 | 단계 | QA 투입 |
|---|---|---|
| 2026.06.01~06.14 | 킥오프/요구사항 확정 | QA 기준, 테스트 계획, 결함 등급 합의 |
| 2026.06.15~07.15 | IA/와이어프레임/디자인 | 접근성·반응형·다국어 UX 사전 리뷰 |
| 2026.07.01~08.15 | 기반 개발 | 단위/통합 테스트, API 계약 검증 |
| 2026.08.16~09.30 | 기능 개발 | CMS, 제품, 검색, 문의, 채용 기능 QA |
| 2026.10.01~10.31 | 시스템 통합 | E2E, 성능, 보안, 접근성 집중 검수 |
| 2026.11.01~11.15 | UAT | 페르소나별 검수, 운영 매뉴얼 확인 |
| 2026.11.16~11.29 | 출시 리허설 | D-1 체크, 롤백, 모니터링, Go/No-Go |
| 2026.11.30 | 정식 오픈 | smoke test, hypercare 전환 |

---

## 참고 기준

- OWASP Top 10:2021 공식 목록: https://owasp.org/Top10/2021/  
- Web Vitals 측정 기준/도구: https://web.dev/articles/vitals  
- WCAG 2.1 W3C 자료: https://www.w3.org/WAI/standards-guidelines/wcag/new-in-21/  
- 개인정보보호위원회 법령 정보: https://pipc.go.kr/np/default/page.do?mCode=D010010000  
- EU GDPR 안내: https://europa.eu/youreurope/business/dealing-with-customers/data-protection/data-protection-gdpr/index_en.htm  

@Lead 통합 보고
