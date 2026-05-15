import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { AppErrorBoundary } from './AppErrorBoundary';

function BrokenComponent() {
  throw new Error('boom');
  return null;
}

describe('AppErrorBoundary', () => {
  it('renders a recoverable fallback when a child throws during render', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);

    render(
      <AppErrorBoundary>
        <BrokenComponent />
      </AppErrorBoundary>
    );

    expect(screen.getByRole('main')).toHaveTextContent('화면 렌더 실패');
    expect(screen.getByRole('button', { name: '다시 시도' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '홈으로 이동' })).toHaveAttribute('href', '/');

    spy.mockRestore();
  });
});
