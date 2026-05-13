import { useId, useMemo } from 'react';

type ModelOption = {
  id: string;
  label: string;
  provider: string;
};

type ModelSelectProps = {
  runtime: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  label?: string;
  helper?: string;
};

const modelOptions: Record<string, ModelOption[]> = {
  codex: [
    { id: 'gpt-5.5', label: 'GPT-5.5', provider: 'OpenAI' },
    { id: 'gpt-5.4', label: 'GPT-5.4', provider: 'OpenAI' },
    { id: 'gpt-5.4-mini', label: 'GPT-5.4 Mini', provider: 'OpenAI' },
    { id: 'gpt-5.3-codex', label: 'GPT-5.3 Codex', provider: 'OpenAI' },
    { id: 'gpt-5.3-codex-spark', label: 'GPT-5.3 Codex Spark', provider: 'OpenAI' },
    { id: 'gpt-5.2', label: 'GPT-5.2', provider: 'OpenAI' }
  ],
  claude: [
    { id: 'claude-sonnet-4.5', label: 'Claude Sonnet 4.5', provider: 'Anthropic' },
    { id: 'claude-opus-4.1', label: 'Claude Opus 4.1', provider: 'Anthropic' },
    { id: 'sonnet', label: 'Sonnet alias', provider: 'Anthropic' },
    { id: 'opus', label: 'Opus alias', provider: 'Anthropic' }
  ],
  gemini: [
    { id: 'gemini-3-pro', label: 'Gemini 3 Pro', provider: 'Google' },
    { id: 'gemini-2.5-pro', label: 'Gemini 2.5 Pro', provider: 'Google' },
    { id: 'gemini-2.5-flash', label: 'Gemini 2.5 Flash', provider: 'Google' }
  ]
};

const defaultHelper = '비워두면 런타임 기본 모델을 사용합니다. 목록에 없는 모델 ID도 직접 입력할 수 있습니다.';

function displayRuntime(runtime: string) {
  return runtime || 'runtime';
}

export function ModelSelect({ runtime, value, onChange, disabled, label = '모델', helper = defaultHelper }: ModelSelectProps) {
  const reactId = useId();
  const inputId = `agent-model-${reactId}`;
  const listId = `agent-model-options-${reactId}`;
  const options = useMemo(() => modelOptions[runtime] ?? [], [runtime]);
  const selected = options.find((option) => option.id === value);
  const custom = value && !selected;

  return (
    <div className="field-label model-select">
      <label className="model-select-header" htmlFor={inputId}>
        <span>{label}</span>
        <span className="model-runtime-pill">{displayRuntime(runtime)}</span>
      </label>
      <div className="model-input-row">
        <input
          id={inputId}
          list={listId}
          value={value}
          placeholder="기본 (런타임 기본값)"
          onChange={(event) => onChange(event.target.value)}
          disabled={disabled}
          autoComplete="off"
        />
        <datalist id={listId}>
          {options.map((option) => (
            <option key={option.id} value={option.id} label={`${option.label} · ${option.provider}`} />
          ))}
        </datalist>
        {value ? (
          <button className="button secondary model-clear-button" type="button" onClick={() => onChange('')} disabled={disabled}>
            기본값
          </button>
        ) : null}
      </div>
      <div className="model-chip-row" aria-label="추천 모델">
        <button className={!value ? 'model-chip active' : 'model-chip'} type="button" onClick={() => onChange('')} disabled={disabled}>
          기본
        </button>
        {options.slice(0, 5).map((option) => (
          <button
            key={option.id}
            className={value === option.id ? 'model-chip active' : 'model-chip'}
            type="button"
            title={option.id}
            onClick={() => onChange(option.id)}
            disabled={disabled}
          >
            {option.label}
          </button>
        ))}
      </div>
      <small>
        {selected ? `${selected.provider} · ${selected.id}` : custom ? `직접 입력한 모델 ID: ${value}` : helper}
      </small>
    </div>
  );
}
