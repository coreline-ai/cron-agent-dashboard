import { ChangeEvent, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../api/client';
import { type Attachment, useIssueAttachmentsQuery } from '../api/queries';

const MAX_BYTES = 10 * 1024 * 1024;

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function IssueAttachmentsPanel({ issueID }: { issueID: string }) {
  const queryClient = useQueryClient();
  const attachments = useIssueAttachmentsQuery(issueID);
  const [selected, setSelected] = useState<File | null>(null);
  const [error, setError] = useState('');

  const upload = useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append('file', file);
      return apiClient.postMultipart(`/issues/${issueID}/attachments`, form);
    },
    onSuccess: () => {
      setSelected(null);
      setError('');
      queryClient.invalidateQueries({ queryKey: ['attachments', issueID] });
    },
    onError: (err: unknown) => {
      setError(err instanceof Error ? err.message : '업로드 실패');
    }
  });

  const remove = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/attachments/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['attachments', issueID] });
    }
  });

  const onPick = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] ?? null;
    if (file && file.size > MAX_BYTES) {
      setError(`파일은 최대 ${formatSize(MAX_BYTES)}까지 업로드할 수 있습니다.`);
      setSelected(null);
      e.target.value = '';
      return;
    }
    setError('');
    setSelected(file);
  };

  return (
    <article className="panel">
      <div className="section-heading compact">
        <h2>첨부 파일</h2>
        <span className="badge">{attachments.data?.length ?? 0}</span>
      </div>
      <div className="attachment-panel__upload">
        <input type="file" onChange={onPick} />
        <button
          type="button"
          className="button secondary"
          disabled={!selected || upload.isPending}
          onClick={() => selected && upload.mutate(selected)}
        >
          {upload.isPending ? '업로드 중' : '업로드'}
        </button>
        {selected && (
          <span className="muted-copy">
            {selected.name} · {formatSize(selected.size)}
          </span>
        )}
      </div>
      {error && <p className="error-text">{error}</p>}
      {(attachments.data ?? []).length === 0 ? (
        <p className="muted-copy">첨부된 파일이 없습니다. 위 입력으로 파일을 추가하세요 (최대 10 MB).</p>
      ) : (
        <ul className="attachment-list">
          {(attachments.data ?? []).map((a) => (
            <AttachmentRow key={a.id} attachment={a} onDelete={() => remove.mutate(a.id)} />
          ))}
        </ul>
      )}
    </article>
  );
}

function AttachmentRow({ attachment, onDelete }: { attachment: Attachment; onDelete: () => void }) {
  return (
    <li className="attachment-row">
      <a className="attachment-row__name" href={attachment.download_url} target="_blank" rel="noreferrer">
        {attachment.filename}
      </a>
      <span className="muted-copy">
        {formatSize(attachment.size_bytes)} · {attachment.content_type} · {attachment.created_at.slice(0, 19).replace('T', ' ')}
      </span>
      <button
        className="button danger ghost"
        type="button"
        onClick={() => {
          if (window.confirm(`${attachment.filename} 파일을 삭제할까요?`)) {
            onDelete();
          }
        }}
      >
        삭제
      </button>
    </li>
  );
}
