import { useEffect, useState } from "react";
import { GetSyncProgress, PauseBackup, ResumeBackup, IsBackupPaused } from "../../wailsjs/go/main/App";
import { formatBytes } from "../utils";
import { FailedAssets } from "./FailedAssets";
import { Pause, Play, RefreshCw, Activity, Loader, Square, Palette, Pin, HardDrive, AlertTriangle } from "lucide-react";

interface ServiceStatus {
    state: string;
    message: string;
    is_paused: boolean;
    current_wallet: string;
    wallets_total: number;
    wallets_synced: number;
    total_nfts: number;
    processed_nfts: number;
    total_assets: number;
    pinned_assets: number;
    failed_assets: number;
    pending_retries: number;
    current_item: string;
    last_sync_at: string | null;
}

interface DashboardProps {
    stats: { [key: string]: number };
}

export function Dashboard({ stats }: DashboardProps) {
    const [status, setStatus] = useState<ServiceStatus | null>(null);
    const [isPaused, setIsPaused] = useState(false);
    const [showFailedModal, setShowFailedModal] = useState(false);

    useEffect(() => {
        const fetchStatus = async () => {
            try {
                const [serviceStatus, paused] = await Promise.all([GetSyncProgress(), IsBackupPaused()]);
                setStatus(serviceStatus as unknown as ServiceStatus);
                setIsPaused(paused);
            } catch (err) {
                console.error("Failed to fetch status:", err);
            }
        };

        fetchStatus();
        // Poll every 2 seconds normally, faster when actively syncing
        const interval = setInterval(fetchStatus, status?.state === "syncing" ? 1000 : 2000);
        return () => clearInterval(interval);
    }, [status?.state]);

    const handleTogglePause = async () => {
        try {
            if (isPaused) {
                await ResumeBackup();
            } else {
                await PauseBackup();
            }
            setIsPaused(!isPaused);
        } catch (err) {
            console.error("Failed to toggle pause:", err);
        }
    };

    const failedCount = (stats.failed || 0) + (stats.failed_unavailable || 0);

    const getStateIcon = () => {
        if (isPaused) return <Pause size={14} />;
        switch (status?.state) {
            case "syncing":
                return <RefreshCw size={14} className="spin" />;
            case "watching":
                return <Activity size={14} />;
            case "starting":
                return <Loader size={14} className="spin" />;
            default:
                return <Square size={14} />;
        }
    };

    const getStateLabel = () => {
        if (isPaused) return "Paused";
        switch (status?.state) {
            case "syncing":
                return "Syncing";
            case "watching":
                return "Monitoring";
            case "starting":
                return "Starting";
            default:
                return "Stopped";
        }
    };

    const getStatusClass = () => {
        if (isPaused) return "paused";
        switch (status?.state) {
            case "syncing":
                return "syncing";
            case "watching":
                return "watching";
            case "starting":
                return "starting";
            default:
                return "stopped";
        }
    };

    const isSyncing = status?.state === "syncing";
    // Only show detailed progress when there are pending assets to pin
    const hasPendingWork =
        status && status.total_assets > 0 && status.pinned_assets + status.failed_assets < status.total_assets;

    return (
        <div className="dashboard-page">
            <div className="page-header">
                <div className="page-header-row">
                    <div>
                        <h1>Dashboard</h1>
                        <p className="page-subtitle">Your NFTs are automatically backed up</p>
                    </div>
                    <div className="header-actions">
                        <span className={`status-badge ${getStatusClass()}`}>
                            {getStateIcon()} {getStateLabel()}
                        </span>
                        <button
                            type="button"
                            className={`btn-toggle ${isPaused ? "paused" : "active"}`}
                            onClick={handleTogglePause}
                        >
                            {isPaused ? (
                                <>
                                    <Play size={14} /> Resume
                                </>
                            ) : (
                                <>
                                    <Pause size={14} /> Pause
                                </>
                            )}
                        </button>
                    </div>
                </div>
            </div>

            {/* Sync Progress - only show detailed progress when actively pinning */}
            {isSyncing && status && hasPendingWork && (
                <div className="sync-progress-banner">
                    <div className="sync-details">
                        {status.current_wallet && (
                            <div className="sync-wallet-info">
                                <span className="wallet-label">Wallet:</span>
                                <span className="wallet-address">
                                    {status.current_wallet.slice(0, 8)}...{status.current_wallet.slice(-6)}
                                </span>
                                {status.wallets_total > 1 && (
                                    <span className="wallet-progress">
                                        ({status.wallets_synced}/{status.wallets_total})
                                    </span>
                                )}
                            </div>
                        )}

                        <div className="sync-stats">
                            {status.total_nfts > 0 && (
                                <div className="sync-stat">
                                    <span className="stat-value">{status.processed_nfts}</span>
                                    <span className="stat-label">/ {status.total_nfts} NFTs</span>
                                </div>
                            )}
                            {status.total_assets > 0 && (
                                <div className="sync-stat">
                                    <span className="stat-value">{status.pinned_assets}</span>
                                    <span className="stat-label">/ {status.total_assets} Pinned</span>
                                </div>
                            )}
                            {status.failed_assets > 0 && (
                                <div className="sync-stat failed">
                                    <span className="stat-value">{status.failed_assets}</span>
                                    <span className="stat-label">Failed</span>
                                </div>
                            )}
                        </div>

                        {status.total_assets > 0 && (
                            <div className="sync-progress-bar">
                                <div
                                    className="sync-progress-fill"
                                    style={{
                                        width: `${
                                            ((status.pinned_assets + status.failed_assets) / status.total_assets) * 100
                                        }%`,
                                    }}
                                ></div>
                            </div>
                        )}

                        {status.current_item && (
                            <div className="sync-current">
                                <span className="current-label">Current:</span>
                                <span className="current-item">{status.current_item}</span>
                            </div>
                        )}
                    </div>
                </div>
            )}

            {/* Last sync info when watching */}
            {status?.state === "watching" && status.last_sync_at && (
                <div className="last-sync-info">Last synced: {new Date(status.last_sync_at).toLocaleTimeString()}</div>
            )}

            {/* Stats Grid - 3 cards */}
            <div className="stats-grid-3">
                <div className="stat-card highlight">
                    <div className="stat-icon">
                        <Palette size={24} />
                    </div>
                    <div className="stat-content">
                        <div className="stat-value">{stats.nft_count || 0}</div>
                        <div className="stat-label">NFTs Backed Up</div>
                    </div>
                </div>

                <div className="stat-card primary">
                    <div className="stat-icon">
                        <Pin size={24} />
                    </div>
                    <div className="stat-content">
                        <div className="stat-value">{stats.pinned || 0}</div>
                        <div className="stat-label">Assets Pinned</div>
                    </div>
                </div>

                <div className="stat-card info">
                    <div className="stat-icon">
                        <HardDrive size={24} />
                    </div>
                    <div className="stat-content">
                        <div className="stat-value">{formatBytes(stats.disk_usage_bytes || 0)}</div>
                        <div className="stat-label">Disk Usage</div>
                    </div>
                </div>
            </div>

            {/* Show failed count as a clickable notice if there are any */}
            {failedCount > 0 && (
                <button type="button" className="failed-notice clickable" onClick={() => setShowFailedModal(true)}>
                    <AlertTriangle size={16} /> {failedCount} asset{failedCount !== 1 ? "s" : ""} failed to pin — Click
                    to view
                </button>
            )}

            {/* Failed Assets Modal */}
            {showFailedModal && (
                <FailedAssets
                    onClose={() => setShowFailedModal(false)}
                    onRetry={() => {
                        // Refresh will happen via stats polling
                    }}
                />
            )}
        </div>
    );
}
