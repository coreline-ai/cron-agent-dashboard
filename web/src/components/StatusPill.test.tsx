import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { StatusPill, getStatusPillLabel } from './StatusPill';

describe('StatusPill', () => {
  it('renders the localized status label and normalized metadata', () => {
    render(<StatusPill kind="run" status="RUNNING" pulse />);

    const pill = screen.getByText('실행 중');
    expect(pill).toHaveAttribute('data-kind', 'run');
    expect(pill).toHaveAttribute('data-status', 'running');
    expect(pill).toHaveClass('status-pill--run', 'status-pill--running', 'status-pill--pulse');
    expect(pill).toHaveAttribute('title', '실행 중');
  });

  it('falls back to a readable label for unknown status values', () => {
    expect(getStatusPillLabel('issue', 'needs_review')).toBe('needs review');
    expect(getStatusPillLabel('runtime', '   ')).toBe('알 수 없음');
  });
});
