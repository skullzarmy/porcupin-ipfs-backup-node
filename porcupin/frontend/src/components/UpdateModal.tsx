import React from 'react';

interface UpdateModalProps {
    isOpen: boolean;
    error?: string;
    success?: boolean;
    progress?: {
        phase: string;
        message: string;
        percent: number;
    };
    onRestart?: () => void;
}

const UpdateModal: React.FC<UpdateModalProps> = ({ isOpen, error, success, progress, onRestart }) => {
    if (!isOpen) return null;

    return (
        <div style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0, 0, 0, 0.7)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 9999
        }}>
            <div style={{
                background: 'var(--bg-card)',
                width: '400px',
                padding: '24px',
                borderRadius: 'var(--radius-lg)',
                textAlign: 'center',
                boxShadow: 'var(--shadow-md)',
                border: '1px solid var(--border-color)'
            }}>
                {error ? (
                    <>
                        <div style={{ 
                            width: '48px', 
                            height: '48px', 
                            borderRadius: '50%', 
                            background: 'rgba(239, 68, 68, 0.1)', 
                            color: 'var(--accent-danger)',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            margin: '0 auto 16px'
                        }}>
                            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                <line x1="18" y1="6" x2="6" y2="18"></line>
                                <line x1="6" y1="6" x2="18" y2="18"></line>
                            </svg>
                        </div>
                        <h3 style={{ margin: '0 0 8px', fontSize: '18px' }}>Update Failed</h3>
                        <p style={{ margin: 0, color: 'var(--text-secondary)', fontSize: '14px' }}>
                            {error}
                        </p>
                        <button 
                            onClick={() => window.location.reload()}
                            style={{
                                marginTop: '20px',
                                background: 'var(--bg-secondary)',
                                border: '1px solid var(--border-color)',
                                color: 'var(--text-primary)',
                                padding: '8px 16px',
                                borderRadius: 'var(--radius-sm)',
                                cursor: 'pointer'
                            }}
                        >
                            Close
                        </button>
                    </>
                ) : success ? (
                    <>
                        <div style={{ 
                            width: '48px', 
                            height: '48px', 
                            borderRadius: '50%', 
                            background: 'rgba(16, 185, 129, 0.1)', 
                            color: 'var(--accent-success)',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            margin: '0 auto 16px'
                        }}>
                            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                <polyline points="20 6 9 17 4 12"></polyline>
                            </svg>
                        </div>
                        <h3 style={{ margin: '0 0 8px', fontSize: '18px' }}>Update Complete</h3>
                        <p style={{ margin: 0, color: 'var(--text-secondary)', fontSize: '14px' }}>
                            Porcupin has been updated successfully.<br/>Please restart the application.
                        </p>
                        <button 
                            onClick={onRestart || (() => window.location.reload())} // Wails Runtime Quit would be better, but reload works for now? No, reload just reloads assets. 
                            style={{
                                marginTop: '20px',
                                background: 'var(--accent-primary)',
                                border: 'none',
                                color: 'white',
                                padding: '8px 16px',
                                borderRadius: 'var(--radius-sm)',
                                cursor: 'pointer',
                                fontWeight: 500
                            }}
                        >
                            Restart Now
                        </button>
                    </>
                ) : (
                    <>
                        <div className="spin" style={{ 
                            width: '40px', 
                            height: '40px', 
                            border: '3px solid var(--bg-secondary)',
                            borderTopColor: 'var(--accent-primary)',
                            borderRadius: '50%', 
                            margin: '0 auto 20px'
                        }}></div>
                        <h3 style={{ margin: '0 0 8px', fontSize: '18px' }}>
                            {progress?.phase === 'downloading' ? 'Downloading Update...' : 'Installing Update...'}
                        </h3>
                        <p style={{ margin: 0, color: 'var(--text-secondary)', fontSize: '14px' }}>
                            {progress?.message || 'Please wait...'}
                        </p>
                    </>
                )}
            </div>
        </div>
    );
};

export default UpdateModal;
