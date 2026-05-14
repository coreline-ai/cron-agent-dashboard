import { KeyboardEvent, useId, useMemo, useRef, useState } from 'react';
import type { Agent } from '../api/queries';

type MentionAutocompleteProps = {
  value: string;
  agents: Agent[];
  placeholder?: string;
  required?: boolean;
  disabled?: boolean;
  onChange: (value: string) => void;
};

type MentionQuery = {
  start: number;
  end: number;
  term: string;
};

const maxSuggestions = 6;

export function MentionAutocomplete({ value, agents, placeholder, required, disabled, onChange }: MentionAutocompleteProps) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const helpID = useId();
  const [highlighted, setHighlighted] = useState(0);
  const query = getMentionQuery(value, textareaRef.current?.selectionStart ?? value.length);
  const suggestions = useMemo(() => {
    if (!query) {
      return [];
    }
    const term = query.term.toLocaleLowerCase();
    return agents
      .filter((agent) => !term || agent.name.toLocaleLowerCase().includes(term))
      .slice(0, maxSuggestions);
  }, [agents, query?.term]);
  const showSuggestions = Boolean(query && suggestions.length);

  const insertAgent = (agent: Agent) => {
    if (!query) {
      return;
    }
    const next = `${value.slice(0, query.start)}@${agent.name} ${value.slice(query.end)}`;
    const cursor = query.start + agent.name.length + 2;
    onChange(next);
    setHighlighted(0);
    const restoreFocus = () => {
      textareaRef.current?.focus();
      textareaRef.current?.setSelectionRange(cursor, cursor);
    };
    if (typeof requestAnimationFrame === 'function') {
      requestAnimationFrame(restoreFocus);
    } else {
      setTimeout(restoreFocus, 0);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (!showSuggestions) {
      return;
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setHighlighted((index) => (index + 1) % suggestions.length);
      return;
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      setHighlighted((index) => (index - 1 + suggestions.length) % suggestions.length);
      return;
    }
    if (event.key === 'Enter' || event.key === 'Tab') {
      event.preventDefault();
      insertAgent(suggestions[highlighted] ?? suggestions[0]);
    }
  };

  return (
    <div className="mention-autocomplete">
      <textarea
        ref={textareaRef}
        placeholder={placeholder}
        value={value}
        required={required}
        disabled={disabled}
        aria-describedby={helpID}
        onChange={(event) => {
          onChange(event.target.value);
          setHighlighted(0);
        }}
        onKeyDown={onKeyDown}
      />
      <p id={helpID} className="field-hint">
        @ 입력 후 에이전트 이름을 선택하면 정확한 멘션으로 자동 완성됩니다.
      </p>
      {showSuggestions ? (
        <div className="mention-suggestions" role="listbox" aria-label="에이전트 멘션 후보">
          {suggestions.map((agent, index) => (
            <button
              key={agent.id}
              type="button"
              role="option"
              aria-selected={index === highlighted}
              className={index === highlighted ? 'active' : ''}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => insertAgent(agent)}
            >
              <strong>@{agent.name}</strong>
              <small>{agent.runtime}{agent.model ? ` · ${agent.model}` : ''}</small>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function getMentionQuery(value: string, cursor: number): MentionQuery | null {
  const beforeCursor = value.slice(0, cursor);
  const match = /(^|\s)@([^@\s]*)$/.exec(beforeCursor);
  if (!match) {
    return null;
  }
  const prefix = match[1] ?? '';
  const term = match[2] ?? '';
  const start = beforeCursor.length - term.length - 1;
  return { start: start + (prefix && !prefix.endsWith('@') ? 0 : 0), end: cursor, term };
}
