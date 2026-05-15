import { type ReactNode } from 'react';
import { ErrorBoundary, type FallbackProps } from 'react-error-boundary';
import { MutationErrorAlert } from './MutationErrorAlert';

type AppErrorBoundaryProps = {
  children: ReactNode;
};

export function AppErrorBoundary({ children }: AppErrorBoundaryProps) {
  return (
    <ErrorBoundary FallbackComponent={AppErrorFallback} onError={(error) => console.error('render boundary caught error', error)}>
      {children}
    </ErrorBoundary>
  );
}

function AppErrorFallback({ error, resetErrorBoundary }: FallbackProps) {
  return (
    <main className="app-error-shell" role="main">
      <section className="panel app-error-card" aria-label="화면 렌더 오류">
        <div className="section-heading compact">
          <div>
            <p className="eyebrow">복구 모드</p>
            <h1>화면 렌더 실패</h1>
          </div>
          <span className="badge warning">error boundary</span>
        </div>
        <MutationErrorAlert error={error} title="화면 렌더 실패" />
        <p className="muted-copy">일시적인 UI 오류일 수 있습니다. 다시 시도하거나 홈으로 이동해 작업을 이어가세요.</p>
        <div className="inline-actions">
          <button className="button" type="button" onClick={resetErrorBoundary}>
            다시 시도
          </button>
          <a className="button secondary" href="/">
            홈으로 이동
          </a>
        </div>
      </section>
    </main>
  );
}
