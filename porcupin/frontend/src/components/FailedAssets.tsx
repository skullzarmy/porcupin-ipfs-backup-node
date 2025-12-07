import { useEffect, useState, useCallback } from "react";
import { GetFailedAssets, RetryAsset, RetryAllFailed, ClearFailed, DeleteAsset } from "../../wailsjs/go/main/App";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { RefreshCw, Trash2, ExternalLink, PartyPopper } from "lucide-react";
import type { db } from "../../wailsjs/go/models";

interface FailedAssetsProps {
    onClose: () => void;
    onRetry: () => void;
}

export function FailedAssets({ onClose, onRetry }: FailedAssetsProps) {
    const [assets, setAssets] = useState<db.Asset[]>([]);
    const [loading, setLoading] = useState(true);
    const [retrying, setRetrying] = useState<Set<number>>(new Set());
    const [deleting, setDeleting] = useState<Set<number>>(new Set());
    const [confirmClearAll, setConfirmClearAll] = useState(false);

    const loadAssets = useCallback(async () => {
        try {
            const failed = await GetFailedAssets();
            setAssets(failed || []);
        } catch (err: unknown) {
            console.error("Failed to load failed assets:", err);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadAssets();
    }, [loadAssets]);

    const handleRetry = async (assetId: number) => {
        setRetrying((prev) => new Set(prev).add(assetId));
        try {
            await RetryAsset(assetId);
            // Remove from list
            setAssets((prev) => prev.filter((a) => a.id !== assetId));
            onRetry();
        } catch (err: unknown) {
            console.error("Failed to retry asset:", err);
        } finally {
            setRetrying((prev) => {
                const next = new Set(prev);
                next.delete(assetId);
                return next;
            });
        }
    };

    const handleRetryAll = async () => {
        try {
            const count = await RetryAllFailed();
            console.log(`Queued ${count} assets for retry`);
            setAssets([]);
            onRetry();
        } catch (err: unknown) {
            console.error("Failed to retry all:", err);
        }
    };

    const handleDelete = async (assetId: number) => {
        setDeleting((prev) => new Set(prev).add(assetId));
        try {
            await DeleteAsset(assetId);
            setAssets((prev) => prev.filter((a) => a.id !== assetId));
            onRetry();
        } catch (err: unknown) {
            console.error("Failed to delete asset:", err);
        } finally {
            setDeleting((prev) => {
                const next = new Set(prev);
                next.delete(assetId);
                return next;
            });
        }
    };

    const handleClearAll = async () => {
        if (!confirmClearAll) {
            setConfirmClearAll(true);
            return;
        }
        try {
            const count = await ClearFailed();
            console.log(`Cleared ${count} failed assets`);
            setAssets([]);
            setConfirmClearAll(false);
            onRetry();
        } catch (err: unknown) {
            console.error("Failed to clear:", err);
            setConfirmClearAll(false);
        }
    };

    const extractCID = (uri: string): string => {
        if (uri.startsWith("ipfs://")) {
            return uri.slice(7).split("/")[0].split("?")[0];
        }
        const match = uri.match(/\/ipfs\/([^/?]+)/);
        return match ? match[1] : uri;
    };

    const getStatusLabel = (status: string): string => {
        switch (status) {
            case "failed_unavailable":
                return "Unavailable";
            case "failed":
                return "Failed";
            default:
                return status;
        }
    };

    const getStatusClass = (status: string): string => {
        return status === "failed_unavailable" ? "unavailable" : "failed";
    };

    if (loading) {
        return (
            <div className="failed-assets-modal">
                <div className="failed-assets-content">
                    <div className="loading">Loading failed assets...</div>
                </div>
            </div>
        );
    }

    return (
        // biome-ignore lint/a11y/useKeyWithClickEvents: backdrop click-to-close is standard UX
        // biome-ignore lint/a11y/noStaticElementInteractions: modal backdrop pattern
        <div className="failed-assets-modal" onClick={onClose}>
            {/* biome-ignore lint/a11y/useKeyWithClickEvents: stopPropagation is necessary for modal */}
            <div className="failed-assets-content" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
                <div className="failed-assets-header">
                    <h2>Failed Assets ({assets.length})</h2>
                    <button type="button" className="close-btn" onClick={onClose}>
                        ×
                    </button>
                </div>

                {assets.length === 0 ? (
                    <div className="empty-state">
                        <p>
                            <PartyPopper size={24} /> No failed assets!
                        </p>
                        <p className="empty-subtitle">All your NFT assets have been successfully pinned.</p>
                    </div>
                ) : (
                    <>
                        <div className="failed-assets-actions">
                            <button type="button" className="btn-retry-all" onClick={handleRetryAll}>
                                <RefreshCw size={14} /> Retry All ({assets.length})
                            </button>
                            {confirmClearAll ? (
                                <div className="confirm-clear">
                                    <span>Delete all?</span>
                                    <button type="button" className="btn-confirm-yes" onClick={handleClearAll}>
                                        Yes, Delete
                                    </button>
                                    <button
                                        type="button"
                                        className="btn-confirm-no"
                                        onClick={() => setConfirmClearAll(false)}
                                    >
                                        Cancel
                                    </button>
                                </div>
                            ) : (
                                <button type="button" className="btn-clear-all" onClick={handleClearAll}>
                                    <Trash2 size={14} /> Clear All
                                </button>
                            )}
                        </div>

                        <div className="failed-assets-list">
                            {assets.map((asset) => (
                                <div key={asset.id} className={`failed-asset-item ${getStatusClass(asset.status)}`}>
                                    <div className="asset-info">
                                        <div className="asset-header">
                                            <span className={`status-tag ${getStatusClass(asset.status)}`}>
                                                {getStatusLabel(asset.status)}
                                            </span>
                                            <span className="asset-type">{asset.type}</span>
                                            {asset.retry_count > 0 && (
                                                <span className="retry-count">{asset.retry_count} retries</span>
                                            )}
                                        </div>

                                        {asset.nft && (
                                            <div className="nft-name">{asset.nft.name || "Untitled NFT"}</div>
                                        )}

                                        <div className="asset-cid" title={asset.uri}>
                                            {extractCID(asset.uri)}
                                        </div>

                                        {asset.error_msg && <div className="error-message">{asset.error_msg}</div>}
                                    </div>

                                    <div className="asset-actions">
                                        <button
                                            type="button"
                                            className="btn-retry"
                                            onClick={() => handleRetry(asset.id)}
                                            disabled={retrying.has(asset.id)}
                                        >
                                            {retrying.has(asset.id) ? (
                                                "..."
                                            ) : (
                                                <>
                                                    <RefreshCw size={14} /> Retry
                                                </>
                                            )}
                                        </button>
                                        <button
                                            type="button"
                                            className="btn-delete"
                                            onClick={() => handleDelete(asset.id)}
                                            disabled={deleting.has(asset.id)}
                                            title="Delete from database"
                                        >
                                            {deleting.has(asset.id) ? "..." : <Trash2 size={14} />}
                                        </button>
                                        <button
                                            type="button"
                                            className="btn-view"
                                            onClick={() =>
                                                BrowserOpenURL(`https://ipfs.io/ipfs/${extractCID(asset.uri)}`)
                                            }
                                        >
                                            <ExternalLink size={14} /> View
                                        </button>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}
