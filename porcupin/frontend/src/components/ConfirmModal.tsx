import type { ReactNode } from "react";

interface ConfirmModalProps {
    isOpen: boolean;
    title: string;
    message?: string;
    children?: ReactNode;
    confirmText?: string;
    cancelText?: string;
    onConfirm: () => void;
    onCancel: () => void;
    isDangerous?: boolean;
    disableConfirm?: boolean;
    disableCancel?: boolean;
}

export function ConfirmModal({
    isOpen,
    title,
    message,
    children,
    confirmText = "Confirm",
    cancelText = "Cancel",
    onConfirm,
    onCancel,
    isDangerous = false,
    disableConfirm = false,
    disableCancel = false,
}: ConfirmModalProps) {
    if (!isOpen) return null;

    return (
        <div
            className="modal-overlay"
            onClick={disableCancel ? undefined : onCancel}
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-title"
            tabIndex={-1}
        >
            <div
                className="modal-content"
                onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => {
                    if (e.key === "Escape" && !disableCancel) {
                        onCancel();
                    }
                    e.stopPropagation();
                }}
                role="document"
            >
                <h3 className="modal-title">{title}</h3>
                {message && <p className="modal-message">{message}</p>}
                {children}
                <div className="modal-actions">
                    <button type="button" className="btn-secondary" onClick={onCancel} disabled={disableCancel}>
                        {cancelText}
                    </button>
                    <button type="button" className={isDangerous ? "btn-danger" : "btn-primary"} onClick={onConfirm} disabled={disableConfirm}>
                        {confirmText}
                    </button>
                </div>
            </div>
        </div>
    );
}
