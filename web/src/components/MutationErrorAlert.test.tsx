import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { ApiError } from '../api/client';
import { MutationErrorAlert, getMutationErrorMessage } from './MutationErrorAlert';

describe('MutationErrorAlert', () => {
  it('renders ApiError messages with server error codes', () => {
    const error = new ApiError(409, {
      error: {
        message: '이미 실행 중입니다.',
        code: 'RUN_CONFLICT'
      }
    });

    render(<MutationErrorAlert error={error} />);

    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('요청 실패');
    expect(alert).toHaveTextContent('이미 실행 중입니다. (RUN_CONFLICT)');
  });

  it('does not render an alert for empty errors and falls back for blank strings', () => {
    const { container } = render(<MutationErrorAlert error={null} />);

    expect(container).toBeEmptyDOMElement();
    expect(getMutationErrorMessage('   ')).toBe('요청 처리 중 오류가 발생했습니다.');
  });
});
