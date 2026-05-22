-- Optional linkage between attachments and the comment they were submitted
-- under. Track F of dev-plan/implement_20260522_220446.md.
--
-- Before this migration every attachment was issue-scoped. The UI now lets
-- an operator submit an attachment as part of a comment; the comment_id
-- column captures that association so the IssueAttachmentsPanel can render
-- "댓글 N에 첨부" groupings. The column is nullable so existing rows
-- continue to display as issue-scoped without a backfill.
--
-- ON DELETE SET NULL keeps the attachment if the underlying comment is
-- removed — the file lives on disk and the issue is still its owner.
ALTER TABLE attachment
  ADD COLUMN comment_id TEXT NULL REFERENCES comment(id) ON DELETE SET NULL;

CREATE INDEX idx_attachment_comment ON attachment(comment_id);
