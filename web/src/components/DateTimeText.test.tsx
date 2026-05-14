import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { DateTimeText } from './DateTimeText';

const absoluteFormatter = new Intl.DateTimeFormat('ko-KR', {
  dateStyle: 'medium',
  timeStyle: 'short'
});

describe('DateTimeText', () => {
  it('renders an absolute datetime with a machine-readable datetime attribute', () => {
    const value = '2026-05-14T12:30:00.000Z';
    const expectedText = absoluteFormatter.format(new Date(value));

    render(<DateTimeText value={value} mode="absolute" />);

    expect(screen.getByText(expectedText)).toHaveAttribute('datetime', new Date(value).toISOString());
  });

  it('renders the configured empty placeholder when value is blank', () => {
    render(<DateTimeText value="  " empty="날짜 없음" />);

    expect(screen.getByText('날짜 없음')).toHaveClass('date-time-text--empty');
  });
});
