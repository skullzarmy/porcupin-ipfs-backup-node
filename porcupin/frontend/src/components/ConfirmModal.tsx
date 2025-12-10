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
}: ConfirmModalProps) {
    if (!isOpen) return null;

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === "Escape") onCancel();
    };

    return (
        <div
            className="modal-overlay"
            onClick={onCancel}
            onKeyDown={handleKeyDown}
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-title"
            tabIndex={-1}
        >
            <div
                className="modal-content"
                onClick={(e) => e.stopPropagation()}
                onKeyDown={(e) => e.stopPropagation()}
                role="document"
            >
                <h3 className="modal-title">{title}</h3>
                {message && <p className="modal-message">{message}</p>}
                {children}
                <div className="modal-actions">
                    <button type="button" className="btn-secondary" onClick={onCancel}>
                        {cancelText}
                    </button>
                    <button type="button" className={isDangerous ? "btn-danger" : "btn-primary"} onClick={onConfirm}>
                        {confirmText}
                    </button>
                </div>
            </div>
        </div>
    );
}
