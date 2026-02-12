import React from 'react';

interface UpdateToastProps {
    version: string;
    onInstall: () => void;
    onDismiss: () => void;
}

const UpdateToast: React.FC<UpdateToastProps> = ({ version, onInstall, onDismiss }) => {
    return (
        <div style={{
            position: 'absolute',
            top: '20px',
            right: '20px',
            zIndex: 1000,
            background: 'var(--bg-card)',
            border: '1px solid var(--accent-primary)',
            borderRadius: 'var(--radius-md)',
            padding: '16px',
            boxShadow: 'var(--shadow-md)',
            display: 'flex',
            alignItems: 'center',
            gap: '16px',
            animation: 'slideIn 0.3s ease-out'
        }}>
            <div>
                <h4 style={{ margin: 0, fontSize: '14px', fontWeight: 600 }}>New Version Available</h4>
                <p style={{ margin: '4px 0 0 0', fontSize: '13px', color: 'var(--text-secondary)' }}>
                    v{version} is ready to install.
                </p>
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
                <button 
                    onClick={onDismiss}
                    style={{
                        background: 'transparent',
                        border: '1px solid var(--border-color)',
                        color: 'var(--text-secondary)',
                        padding: '6px 12px',
                        borderRadius: 'var(--radius-sm)',
                        cursor: 'pointer',
                        fontSize: '13px'
                    }}
                >
                    Later
                </button>
                <button 
                    onClick={onInstall}
                    style={{
                        background: 'var(--accent-primary)',
                        border: 'none',
                        color: 'white',
                        padding: '6px 12px',
                        borderRadius: 'var(--radius-sm)',
                        cursor: 'pointer',
                        fontWeight: 500,
                        fontSize: '13px'
                    }}
                >
                    Update Now
                </button>
            </div>
            <style>{`
                @keyframes slideIn {
                    from { transform: translateY(-20px); opacity: 0; }
                    to { transform: translateY(0); opacity: 1; }
                }
            `}</style>
        </div>
    );
};

export default UpdateToast;
