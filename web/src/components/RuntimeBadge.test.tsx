import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { RuntimeBadge, classifyRuntime } from './RuntimeBadge';

describe('RuntimeBadge', () => {
  const detected = [
    { name: 'codex', version: '0.129.0', path: '/usr/local/bin/codex', supported: true },
    { name: 'claude', version: '2.1', path: '/usr/local/bin/claude', supported: true },
    { name: 'gemini', version: '', path: '/usr/local/bin/gemini', supported: false, warning: '--version 확인 실패' }
  ];

  it('marks codex as recommended when detected on PATH', () => {
    const res = classifyRuntime('codex', detected);
    expect(res.variant).toBe('ok');
    expect(res.description).toContain('/usr/local/bin/codex');
  });

  it('flags claude with the operational hang warning even when detected', () => {
    const res = classifyRuntime('claude', detected);
    expect(res.variant).toBe('warn-known-issue');
    expect(res.description).toContain('hang');
  });

  it('marks runtimes that are not on PATH as not-detected and surfaces the server warning', () => {
    const res = classifyRuntime('gemini', detected);
    expect(res.variant).toBe('warn-not-detected');
    expect(res.description).toContain('--version 확인 실패');
  });

  it('handles a runtime missing from the settings response by warning operators', () => {
    const res = classifyRuntime('unknown', detected);
    expect(res.variant).toBe('warn-not-detected');
    expect(res.description).toContain('unknown CLI가 PATH');
  });

  it('renders the badge with stable data attributes for styling and tests', () => {
    render(<RuntimeBadge runtime="codex" runtimes={detected} />);
    const badge = screen.getByText('✓ 권장');
    expect(badge).toHaveAttribute('data-runtime', 'codex');
    expect(badge).toHaveAttribute('data-variant', 'ok');
  });
});
