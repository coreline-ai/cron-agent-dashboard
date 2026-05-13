import { useEffect, useMemo, useRef, useState } from 'react';
import type { WorkspaceSummary } from '../api/queries';

type WorkspaceSwitcherProps = {
  workspaces: WorkspaceSummary[];
  currentWorkspace?: WorkspaceSummary;
  isLoading?: boolean;
  onSelect: (slug: string) => void;
  onCreate: () => void;
};

export function WorkspaceSwitcher({ workspaces, currentWorkspace, isLoading, onSelect, onCreate }: WorkspaceSwitcherProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
      }
    };
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('pointerdown', onPointerDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.removeEventListener('pointerdown', onPointerDown);
    };
  }, [open]);

  const filteredWorkspaces = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) {
      return workspaces;
    }
    return workspaces.filter((workspace) =>
      [workspace.name, workspace.slug, workspace.identifier_prefix, workspace.description]
        .filter((value): value is string => Boolean(value))
        .some((value) => value.toLowerCase().includes(keyword))
    );
  }, [query, workspaces]);

  const selectWorkspace = (slug: string) => {
    onSelect(slug);
    setOpen(false);
    setQuery('');
  };

  const createWorkspace = () => {
    onCreate();
    setOpen(false);
    setQuery('');
  };

  return (
    <div className="workspace-switcher" ref={rootRef}>
      <p className="nav-group-label">현재 워크스페이스</p>
      <button
        className="workspace-switcher-trigger"
        type="button"
        onClick={() => setOpen((value) => !value)}
        aria-haspopup="dialog"
        aria-expanded={open}
        disabled={isLoading && !currentWorkspace}
      >
        <span className="workspace-prefix">{currentWorkspace?.identifier_prefix ?? '--'}</span>
        <span className="workspace-switcher-copy">
          <strong>{isLoading && !currentWorkspace ? '불러오는 중' : currentWorkspace?.name ?? '워크스페이스 없음'}</strong>
          <small>
            {currentWorkspace
              ? `open ${currentWorkspace.open_issue_count ?? 0} · agents ${currentWorkspace.agent_count ?? 0}`
              : '새 워크스페이스를 만들어 시작'}
          </small>
        </span>
        <span className="workspace-caret" aria-hidden="true">
          ▾
        </span>
      </button>

      <button className="workspace-create-link" type="button" onClick={createWorkspace}>
        + 새 워크스페이스
      </button>

      {open && (
        <div className="workspace-menu" role="dialog" aria-label="워크스페이스 선택">
          <div className="workspace-menu-header">
            <strong>워크스페이스 선택</strong>
            <span>{workspaces.length}개</span>
          </div>
          <input
            className="workspace-search"
            autoFocus
            placeholder="이름, slug, prefix 검색"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <div className="workspace-options" role="listbox" aria-label="워크스페이스 목록">
            {filteredWorkspaces.map((workspace) => {
              const selected = workspace.slug === currentWorkspace?.slug;
              return (
                <button
                  className={selected ? 'workspace-option active' : 'workspace-option'}
                  type="button"
                  key={workspace.id}
                  onClick={() => selectWorkspace(workspace.slug)}
                  role="option"
                  aria-selected={selected}
                >
                  <span className="workspace-prefix">{workspace.identifier_prefix}</span>
                  <span>
                    <strong>{workspace.name}</strong>
                    <small>
                      {workspace.slug} · open {workspace.open_issue_count ?? 0} · agents {workspace.agent_count ?? 0}
                    </small>
                  </span>
                  {selected && <span className="workspace-selected-dot" aria-label="현재 선택됨" />}
                </button>
              );
            })}
            {!filteredWorkspaces.length && <p className="workspace-empty">검색 결과가 없습니다.</p>}
          </div>
          <button className="workspace-menu-create" type="button" onClick={createWorkspace}>
            + 새 워크스페이스 만들기
          </button>
        </div>
      )}
    </div>
  );
}
