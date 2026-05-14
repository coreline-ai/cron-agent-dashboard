import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

export type ToastTone = 'success' | 'error' | 'info' | 'warning';

export type Toast = {
  id: string;
  tone: ToastTone;
  title: string;
  description?: string;
  action?: { label: string; href: string };
  durationMs?: number;
};

export type ToastInput = Omit<Toast, 'id'> & {
  id?: string;
};

export type ToastShortcutOptions = Pick<ToastInput, 'description' | 'action' | 'durationMs' | 'id'>;

export type ToastContextValue = {
  toasts: Toast[];
  showToast: (toast: ToastInput) => string;
  dismissToast: (id: string) => void;
  success: (title: string, options?: ToastShortcutOptions) => string;
  error: (title: string, options?: ToastShortcutOptions) => string;
  info: (title: string, options?: ToastShortcutOptions) => string;
  warning: (title: string, options?: ToastShortcutOptions) => string;
};

export type ToastProviderProps = {
  children: ReactNode;
};

const DEFAULT_DURATION_MS: Record<ToastTone, number> = {
  success: 4_000,
  info: 5_000,
  warning: 6_000,
  error: 8_000
};

const TOAST_ICON: Record<ToastTone, string> = {
  success: '✓',
  error: '!',
  info: 'i',
  warning: '!'
};

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: ToastProviderProps) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const dismissToast = useCallback((id: string) => {
    setToasts((currentToasts) => currentToasts.filter((toast) => toast.id !== id));
  }, []);

  const showToast = useCallback((input: ToastInput) => {
    const id = input.id ?? createToastId();
    const toast: Toast = {
      id,
      tone: input.tone,
      title: input.title,
      description: input.description,
      action: input.action,
      durationMs: input.durationMs
    };

    setToasts((currentToasts) => [toast, ...currentToasts.filter((currentToast) => currentToast.id !== id)].slice(0, 5));
    return id;
  }, []);

  const success = useCallback((title: string, options?: ToastShortcutOptions) => showToast({ tone: 'success', title, ...options }), [showToast]);
  const error = useCallback((title: string, options?: ToastShortcutOptions) => showToast({ tone: 'error', title, ...options }), [showToast]);
  const info = useCallback((title: string, options?: ToastShortcutOptions) => showToast({ tone: 'info', title, ...options }), [showToast]);
  const warning = useCallback((title: string, options?: ToastShortcutOptions) => showToast({ tone: 'warning', title, ...options }), [showToast]);

  const value = useMemo(
    () => ({ toasts, showToast, dismissToast, success, error, info, warning }),
    [toasts, showToast, dismissToast, success, error, info, warning]
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-viewport" role="region" aria-label="알림">
        {toasts.map((toast) => (
          <ToastCard key={toast.id} toast={toast} onDismiss={dismissToast} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within ToastProvider');
  }
  return context;
}

function ToastCard({ toast, onDismiss }: { toast: Toast; onDismiss: (id: string) => void }) {
  useEffect(() => {
    const durationMs = toast.durationMs ?? DEFAULT_DURATION_MS[toast.tone];
    if (durationMs <= 0) {
      return;
    }

    const timeoutId = window.setTimeout(() => onDismiss(toast.id), durationMs);
    return () => window.clearTimeout(timeoutId);
  }, [toast, onDismiss]);

  return (
    <section className={`toast-card toast-card--${toast.tone}`} role={toast.tone === 'error' ? 'alert' : 'status'}>
      <span className="toast-icon" aria-hidden="true">
        {TOAST_ICON[toast.tone]}
      </span>
      <div className="toast-content">
        <strong>{toast.title}</strong>
        {toast.description ? <p>{toast.description}</p> : null}
        {toast.action ? (
          <a className="toast-action" href={toast.action.href} onClick={() => onDismiss(toast.id)}>
            {toast.action.label}
          </a>
        ) : null}
      </div>
      <button className="toast-close" type="button" onClick={() => onDismiss(toast.id)} aria-label="알림 닫기">
        ×
      </button>
    </section>
  );
}

function createToastId() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `toast-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}
