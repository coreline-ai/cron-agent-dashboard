import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { Agent } from '../api/queries';
import { MentionAutocomplete } from './MentionAutocomplete';

afterEach(() => cleanup());

const agents: Agent[] = [
  { id: 'a1', name: 'NewsLead', runtime: 'codex', model: 'gpt-5.5', instructions: 'lead', is_main: true },
  { id: 'a2', name: '리뷰어', runtime: 'claude', model: 'gpt-5.5', instructions: 'review', is_main: false }
];

describe('MentionAutocomplete', () => {
  it('renders filtered agent suggestions after an @ mention and inserts the selected agent', () => {
    const onChange = vi.fn();

    render(<MentionAutocomplete value="검토 @리" agents={agents} onChange={onChange} />);

    expect(screen.getByRole('option', { name: /@리뷰어/ })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('option', { name: /@리뷰어/ }));

    expect(onChange).toHaveBeenCalledWith('검토 @리뷰어 ');
  });

  it('supports keyboard selection with Enter', () => {
    const onChange = vi.fn();

    render(<MentionAutocomplete value="@News" agents={agents} onChange={onChange} />);
    fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' });

    expect(onChange).toHaveBeenCalledWith('@NewsLead ');
  });
});
