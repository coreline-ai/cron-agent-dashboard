import { cleanup, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { IssueAttachmentsPanel } from './IssueAttachmentsPanel';
import { apiClient } from '../api/client';
import type { Attachment } from '../api/queries';

function renderPanel(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('IssueAttachmentsPanel', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('shows the empty-state copy and the upload hint', async () => {
    vi.spyOn(apiClient, 'get').mockResolvedValue({ attachments: [] } as any);
    renderPanel(<IssueAttachmentsPanel issueID="iss-1" />);
    expect(await screen.findByText(/첨부된 파일이 없습니다/)).toBeInTheDocument();
    expect(screen.getByText(/최대 10 MB/)).toBeInTheDocument();
  });

  it('renders an inline <img> preview when the attachment is an image content type', async () => {
    const sample: Attachment = {
      id: 'att-img',
      issue_id: 'iss-1',
      uploaded_by: 'user',
      filename: 'pic.png',
      content_type: 'image/png',
      size_bytes: 2048,
      sha256: 'def',
      download_url: '/api/attachments/att-img/download',
      created_at: '2026-05-22T10:00:00Z'
    };
    vi.spyOn(apiClient, 'get').mockResolvedValue({ attachments: [sample] } as any);
    renderPanel(<IssueAttachmentsPanel issueID="iss-1" />);
    const img = (await screen.findByAltText('pic.png')) as HTMLImageElement;
    expect(img.getAttribute('src')).toBe('/api/attachments/att-img/download');
  });

  it('does not render an inline preview for non-image attachments', async () => {
    const sample: Attachment = {
      id: 'att-pdf',
      issue_id: 'iss-1',
      uploaded_by: 'user',
      filename: 'doc.pdf',
      content_type: 'application/pdf',
      size_bytes: 4096,
      sha256: 'ghi',
      download_url: '/api/attachments/att-pdf/download',
      created_at: '2026-05-22T10:00:00Z'
    };
    vi.spyOn(apiClient, 'get').mockResolvedValue({ attachments: [sample] } as any);
    renderPanel(<IssueAttachmentsPanel issueID="iss-1" />);
    await screen.findByText('doc.pdf');
    expect(screen.queryByAltText('doc.pdf')).not.toBeInTheDocument();
  });

  it('renders the attached file row with filename, size, content type, and a delete button', async () => {
    const sample: Attachment = {
      id: 'att-1',
      issue_id: 'iss-1',
      uploaded_by: 'user',
      filename: 'rfp.pdf',
      content_type: 'application/pdf',
      size_bytes: 4096,
      sha256: 'abc',
      download_url: '/api/attachments/att-1/download',
      created_at: '2026-05-22T10:00:00Z'
    };
    vi.spyOn(apiClient, 'get').mockResolvedValue({ attachments: [sample] } as any);
    renderPanel(<IssueAttachmentsPanel issueID="iss-1" />);

    expect(await screen.findByText('rfp.pdf')).toBeInTheDocument();
    expect(screen.getByText(/application\/pdf/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '삭제' })).toBeInTheDocument();
    expect((screen.getByText('rfp.pdf') as HTMLAnchorElement).getAttribute('href')).toBe('/api/attachments/att-1/download');
  });
});
