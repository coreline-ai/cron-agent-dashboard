-- Per-subscription opt-in to mask PII (email / phone fragments) in webhook
-- payload text fields before dispatch. Track B of
-- dev-plan/implement_20260522_212332.md.
--
-- Default is 0 to preserve the v0.1.x behaviour where payloads are sent
-- verbatim. Operators can flip it per-subscription to redact email /
-- phone before the dispatcher signs and POSTs the body.
ALTER TABLE webhook
  ADD COLUMN mask_pii INTEGER NOT NULL DEFAULT 0
  CHECK (mask_pii IN (0,1));
