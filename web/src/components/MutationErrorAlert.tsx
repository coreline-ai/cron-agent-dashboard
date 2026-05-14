import { ApiError } from '../api/client';

export type MutationErrorAlertProps = {
  error: unknown;
  title?: string;
};

const DEFAULT_ERROR_MESSAGE = '요청 처리 중 오류가 발생했습니다.';

export function MutationErrorAlert({ error, title = '요청 실패' }: MutationErrorAlertProps) {
  if (error == null) {
    return null;
  }

  return (
    <div className="mutation-error-alert" role="alert">
      <strong>{title}</strong>
      <p>{getMutationErrorMessage(error)}</p>
    </div>
  );
}

export function getMutationErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return formatApiError(error);
  }

  if (typeof error === 'string') {
    return error.trim() || DEFAULT_ERROR_MESSAGE;
  }

  if (error instanceof Error) {
    return error.message.trim() || DEFAULT_ERROR_MESSAGE;
  }

  if (isRecord(error)) {
    const nestedError = error.error;
    const nestedMessage = isRecord(nestedError) ? readString(nestedError.message) : readString(nestedError);
    const directMessage = readString(error.message);
    const code = readString(error.code) ?? (isRecord(nestedError) ? readString(nestedError.code) : undefined);
    const message = nestedMessage ?? directMessage;

    if (message) {
      return withCode(message, code);
    }
    if (code) {
      return `${DEFAULT_ERROR_MESSAGE} (${code})`;
    }
  }

  return DEFAULT_ERROR_MESSAGE;
}

function formatApiError(error: ApiError) {
  const body = error.body;
  const bodyMessage = body?.error?.message ?? body?.message;
  const message = bodyMessage ?? error.message;
  const code = body?.error?.code ?? body?.code;
  const suffix = code ?? (bodyMessage ? `HTTP ${error.status}` : undefined);
  return withCode(message || DEFAULT_ERROR_MESSAGE, suffix);
}

function withCode(message: string, code?: string) {
  const trimmedMessage = message.trim() || DEFAULT_ERROR_MESSAGE;
  if (!code) {
    return trimmedMessage;
  }
  if (trimmedMessage.includes(code)) {
    return trimmedMessage;
  }
  return `${trimmedMessage} (${code})`;
}

function readString(value: unknown) {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
