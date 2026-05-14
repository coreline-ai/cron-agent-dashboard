import { Modal } from './Modal';

export type ConfirmDialogProps = {
  open: boolean;
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: 'default' | 'danger';
  pending?: boolean;
  onConfirm: () => void;
  onClose: () => void;
};

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = '확인',
  cancelLabel = '취소',
  tone = 'default',
  pending = false,
  onConfirm,
  onClose
}: ConfirmDialogProps) {
  const closeIfIdle = () => {
    if (!pending) {
      onClose();
    }
  };

  return (
    <Modal
      open={open}
      title={title}
      description={description}
      onClose={closeIfIdle}
      footer={
        <>
          <button className="button secondary" type="button" onClick={closeIfIdle} disabled={pending}>
            {cancelLabel}
          </button>
          <button className={`button${tone === 'danger' ? ' danger' : ''}`} type="button" onClick={onConfirm} disabled={pending}>
            {pending ? '처리 중...' : confirmLabel}
          </button>
        </>
      }
    >
      <div className={`confirm-dialog-content confirm-dialog-content--${tone}`} aria-live={pending ? 'polite' : undefined}>
        <p>{tone === 'danger' ? '되돌리기 어려운 작업일 수 있습니다. 내용을 확인한 뒤 진행하세요.' : '계속 진행하시겠습니까?'}</p>
        {pending ? <p className="confirm-dialog-pending">요청을 처리하고 있습니다.</p> : null}
      </div>
    </Modal>
  );
}
