import { useState, useEffect, useCallback, useMemo } from "react";
import { GetNFTsWithAssets, RetryAsset, UnpinAsset, DeleteAsset, ShowInFinder } from "../lib/backend";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import {
    Search,
    Grid3X3,
    List,
    LayoutList,
    RefreshCw,
    FolderOpen,
    ExternalLink,
    RotateCcw,
    Trash2,
    Pin,
    PinOff,
    ChevronLeft,
    ChevronRight,
    Image,
    FileText,
    Film,
    Music,
    File,
    AlertCircle,
    Clock,
    CheckCircle,
    XCircle,
} from "lucide-react";
import type { db } from "../../wailsjs/go/models";
import { formatBytes } from "../utils";

const IPFS_GATEWAY = "https://ipfs.fileship.xyz";

interface AssetsProps {
    onStatsChange: () => void;
}

type LayoutMode = "grid" | "list" | "compact";
type StatusFilter = "all" | "pinned" | "pending" | "failed";

// Extract CID from IPFS URI
function getCidFromUri(uri: string): string | null {
    if (!uri) return null;
    // Handle ipfs:// protocol
    if (uri.startsWith("ipfs://")) {
        return uri.replace("ipfs://", "");
    }
    // Handle gateway URLs
    const match = uri.match(/\/ipfs\/([a-zA-Z0-9]+)/);
    return match ? match[1] : null;
}

// Get preview URL for an asset
function getPreviewUrl(asset: db.Asset): string | null {
    const cid = getCidFromUri(asset.uri);
    if (!cid) return null;
    return `${IPFS_GATEWAY}/${cid}`;
}

// Get icon based on mime type
function getMimeIcon(mimeType: string | undefined) {
    if (!mimeType) return File;
    if (mimeType.startsWith("image/")) return Image;
    if (mimeType.startsWith("video/")) return Film;
    if (mimeType.startsWith("audio/")) return Music;
    if (mimeType.startsWith("text/") || mimeType.includes("json")) return FileText;
    return File;
}

// Get status icon
function getStatusIcon(status: string) {
    switch (status) {
        case "pinned":
            return CheckCircle;
        case "pending":
            return Clock;
        case "failed":
        case "failed_unavailable":
            return XCircle;
        default:
            return AlertCircle;
    }
}

export function Assets({ onStatsChange }: AssetsProps) {
    const [nfts, setNfts] = useState<db.NFT[]>([]);
    const [allAssets, setAllAssets] = useState<db.Asset[]>([]);
    const [page, setPage] = useState(1);
    const [hasMore, setHasMore] = useState(true);
    const [loading, setLoading] = useState(false);

    // UI state
    const [layout, setLayout] = useState<LayoutMode>(() => {
        return (localStorage.getItem("porcupin-asset-layout") as LayoutMode) || "grid";
    });
    const [searchQuery, setSearchQuery] = useState("");
    const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");

    // Debounced search query for API calls
    const [debouncedSearch, setDebouncedSearch] = useState("");

    const PAGE_SIZE = 50;

    // Debounce search input
    useEffect(() => {
        const timer = setTimeout(() => {
            setDebouncedSearch(searchQuery);
            setPage(1); // Reset to page 1 on search change
        }, 500);
        return () => clearTimeout(timer);
    }, [searchQuery]);

    // Reset page on status change
    useEffect(() => {
        setPage(1);
    }, [statusFilter]);

    const loadNfts = useCallback(async () => {
        setLoading(true);
        try {
            // Pass status and search to backend
            const res = await GetNFTsWithAssets(page, PAGE_SIZE, statusFilter, debouncedSearch);
            setNfts(res || []);
            setHasMore((res?.length || 0) >= PAGE_SIZE);

            // Flatten assets for the flat views
            const assets: db.Asset[] = [];
            for (const nft of res || []) {
                if (nft.assets) {
                    for (const asset of nft.assets) {
                        // Create a copy with the parent NFT reference
                        const assetWithNft = Object.assign(Object.create(Object.getPrototypeOf(asset)), asset);
                        assetWithNft.nft = nft;
                        assets.push(assetWithNft);
                    }
                }
            }
            setAllAssets(assets);
        } catch (err: unknown) {
            console.error(err);
        } finally {
            setLoading(false);
        }
    }, [page, statusFilter, debouncedSearch]);

    useEffect(() => {
        loadNfts();
    }, [loadNfts]);

    // Save layout preference
    useEffect(() => {
        localStorage.setItem("porcupin-asset-layout", layout);
    }, [layout]);

    // UI filtering is no longer needed since we filter on backend
    // But we might want to keep some simple client-side logic for "all" 
    // or just rely on backend results.
    // For now, we use the results directly from backend which are already filtered.
    
    const filteredAssets = allAssets;
    const filteredNfts = nfts;

    const handleRefresh = () => {
        loadNfts();
    };

    const handleRetry = async (id: number, e?: React.MouseEvent) => {
        e?.stopPropagation();
        try {
            await RetryAsset(id);
            handleRefresh();
            onStatsChange();
        } catch (err: unknown) {
            console.error(err);
        }
    };

    const handleUnpin = async (id: number, e?: React.MouseEvent) => {
        e?.stopPropagation();
        try {
            await UnpinAsset(id);
            handleRefresh();
            onStatsChange();
        } catch (err: unknown) {
            console.error(err);
        }
    };

    const handleDelete = async (id: number, e?: React.MouseEvent) => {
        e?.stopPropagation();
        if (!confirm("Delete this asset? This will unpin it from IPFS.")) return;
        try {
            await DeleteAsset(id);
            handleRefresh();
            onStatsChange();
        } catch (err: unknown) {
            console.error(err);
        }
    };

    const handleShowInFinder = async () => {
        try {
            await ShowInFinder();
        } catch (err: unknown) {
            console.error(err);
        }
    };

    const handleOpenPreview = (asset: db.Asset) => {
        const url = getPreviewUrl(asset);
        if (url) {
            BrowserOpenURL(url);
        }
    };

    const getStatusCounts = () => {
        const counts = { pinned: 0, pending: 0, failed: 0 };
        for (const asset of allAssets) {
            if (asset.status === "pinned") counts.pinned++;
            else if (asset.status === "pending") counts.pending++;
            else if (asset.status.includes("failed")) counts.failed++;
        }
        return counts;
    };

    const statusCounts = getStatusCounts();

    // Render asset card for grid view
    const renderGridCard = (nft: db.NFT) => {
        const thumbnailAsset =
            nft.assets?.find((a) => a.type === "thumbnail" || a.type === "display") || nft.assets?.[0];
        const previewUrl = thumbnailAsset ? getPreviewUrl(thumbnailAsset) : null;
        const allPinned = nft.assets?.every((a) => a.status === "pinned");
        const hasFailed = nft.assets?.some((a) => a.status.includes("failed"));
        const hasPending = nft.assets?.some((a) => a.status === "pending");

        return (
            <div key={nft.id} className="asset-grid-card">
                <button
                    type="button"
                    className="asset-preview"
                    onClick={() => thumbnailAsset && handleOpenPreview(thumbnailAsset)}
                >
                    {previewUrl ? (
                        <img src={previewUrl} alt={nft.name || "NFT"} loading="lazy" />
                    ) : (
                        <div className="preview-placeholder">
                            <Image size={32} />
                        </div>
                    )}
                    <div className="preview-overlay">
                        <ExternalLink size={20} />
                    </div>
                </button>
                <div className="asset-card-content">
                    <div className="asset-card-title">{nft.name || `Token #${nft.token_id}`}</div>
                    <div className="asset-card-meta">
                        <span className="asset-count">{nft.assets?.length || 0} assets</span>
                        <span
                            className={`status-indicator ${
                                allPinned ? "pinned" : hasFailed ? "failed" : hasPending ? "pending" : ""
                            }`}
                        >
                            {allPinned ? (
                                <CheckCircle size={14} />
                            ) : hasFailed ? (
                                <XCircle size={14} />
                            ) : (
                                <Clock size={14} />
                            )}
                        </span>
                    </div>
                    <div className="asset-card-actions">
                        {nft.assets?.map((asset) => (
                            <div key={asset.id} className="asset-mini-row">
                                <span className="asset-type-tag">{asset.type}</span>
                                {asset.status === "pinned" && (
                                    <button type="button" onClick={(e) => handleUnpin(asset.id, e)} title="Unpin">
                                        <PinOff size={14} />
                                    </button>
                                )}
                                {asset.status === "pending" && (
                                    <button type="button" onClick={(e) => handleRetry(asset.id, e)} title="Pin Now">
                                        <Pin size={14} />
                                    </button>
                                )}
                                {asset.status.includes("failed") && (
                                    <button type="button" onClick={(e) => handleRetry(asset.id, e)} title="Retry">
                                        <RotateCcw size={14} />
                                    </button>
                                )}
                            </div>
                        ))}
                    </div>
                </div>
            </div>
        );
    };

    // Render list row
    const renderListRow = (asset: db.Asset) => {
        const MimeIcon = getMimeIcon(asset.mime_type);
        const StatusIcon = getStatusIcon(asset.status);
        const previewUrl = getPreviewUrl(asset);

        return (
            <div key={asset.id} className="asset-list-row">
                <button type="button" className="asset-list-preview" onClick={() => handleOpenPreview(asset)}>
                    {previewUrl && asset.mime_type?.startsWith("image/") ? (
                        <img src={previewUrl} alt="" loading="lazy" />
                    ) : (
                        <MimeIcon size={24} />
                    )}
                </button>
                <div className="asset-list-info">
                    <div className="asset-list-title">{asset.nft?.name || `Token #${asset.nft?.token_id}`}</div>
                    <div className="asset-list-meta">
                        <span className="asset-type-badge">{asset.type}</span>
                        {asset.mime_type && <span className="mime-type">{asset.mime_type.split("/")[1]}</span>}
                        <span className="asset-size">{formatBytes(asset.size_bytes)}</span>
                    </div>
                </div>
                <div className={`asset-list-status ${asset.status}`}>
                    <StatusIcon size={16} />
                    <span>{asset.status.replace("_", " ")}</span>
                </div>
                <div className="asset-list-actions">
                    <button
                        type="button"
                        className="action-btn"
                        onClick={() => handleOpenPreview(asset)}
                        title="Open in browser"
                    >
                        <ExternalLink size={16} />
                    </button>
                    {asset.status === "pinned" && (
                        <button
                            type="button"
                            className="action-btn"
                            onClick={(e) => handleUnpin(asset.id, e)}
                            title="Unpin"
                        >
                            <PinOff size={16} />
                        </button>
                    )}
                    {(asset.status === "pending" || asset.status.includes("failed")) && (
                        <button
                            type="button"
                            className="action-btn"
                            onClick={(e) => handleRetry(asset.id, e)}
                            title={asset.status === "pending" ? "Pin Now" : "Retry"}
                        >
                            <RotateCcw size={16} />
                        </button>
                    )}
                    <button
                        type="button"
                        className="action-btn danger"
                        onClick={(e) => handleDelete(asset.id, e)}
                        title="Delete"
                    >
                        <Trash2 size={16} />
                    </button>
                </div>
            </div>
        );
    };

    // Render compact row
    const renderCompactRow = (asset: db.Asset) => {
        const StatusIcon = getStatusIcon(asset.status);

        return (
            <tr key={asset.id} className="compact-row">
                <td className="compact-name">
                    <span className="nft-name">{asset.nft?.name || `#${asset.nft?.token_id}`}</span>
                </td>
                <td className="compact-type">
                    <span className="type-badge">{asset.type}</span>
                </td>
                <td className="compact-status">
                    <span className={`status-pill ${asset.status}`}>
                        <StatusIcon size={12} />
                        {asset.status.replace("_", " ")}
                    </span>
                </td>
                <td className="compact-size">{formatBytes(asset.size_bytes)}</td>
                <td className="compact-actions">
                    <button type="button" onClick={() => handleOpenPreview(asset)} title="Open">
                        <ExternalLink size={14} />
                    </button>
                    {asset.status === "pinned" && (
                        <button type="button" onClick={(e) => handleUnpin(asset.id, e)} title="Unpin">
                            <PinOff size={14} />
                        </button>
                    )}
                    {(asset.status === "pending" || asset.status.includes("failed")) && (
                        <button type="button" onClick={(e) => handleRetry(asset.id, e)} title="Retry">
                            <RotateCcw size={14} />
                        </button>
                    )}
                    <button type="button" className="danger" onClick={(e) => handleDelete(asset.id, e)} title="Delete">
                        <Trash2 size={14} />
                    </button>
                </td>
            </tr>
        );
    };

    return (
        <div className="assets-page">
            {/* Header */}
            <div className="page-header">
                <div className="page-header-row">
                    <div>
                        <h1>Assets</h1>
                        <p className="page-subtitle">
                            {allAssets.length} assets across {nfts.length} NFTs
                        </p>
                    </div>
                    <div className="header-actions">
                        <button type="button" className="btn-secondary" onClick={handleShowInFinder}>
                            <FolderOpen size={16} />
                            <span>Open Folder</span>
                        </button>
                        <button type="button" className="btn-secondary" onClick={handleRefresh} disabled={loading}>
                            <RefreshCw size={16} className={loading ? "spin" : ""} />
                        </button>
                    </div>
                </div>
            </div>

            {/* Toolbar */}
            <div className="assets-toolbar">
                <div className="search-box">
                    <Search size={18} aria-hidden="true" />
                    <input
                        type="text"
                        placeholder="Search assets..."
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                        aria-label="Search assets"
                    />
                </div>

                <fieldset className="status-filters" aria-label="Filter by status">
                    <button
                        type="button"
                        className={statusFilter === "all" ? "active" : ""}
                        onClick={() => setStatusFilter("all")}
                        aria-pressed={statusFilter === "all"}
                    >
                        All
                    </button>
                    <button
                        type="button"
                        className={statusFilter === "pinned" ? "active" : ""}
                        onClick={() => setStatusFilter("pinned")}
                        aria-pressed={statusFilter === "pinned"}
                    >
                        <CheckCircle size={14} aria-hidden="true" />
                        Pinned
                        <span className="count">{statusCounts.pinned}</span>
                    </button>
                    <button
                        type="button"
                        className={statusFilter === "pending" ? "active" : ""}
                        onClick={() => setStatusFilter("pending")}
                        aria-pressed={statusFilter === "pending"}
                    >
                        <Clock size={14} aria-hidden="true" />
                        Pending
                        <span className="count">{statusCounts.pending}</span>
                    </button>
                    <button
                        type="button"
                        className={statusFilter === "failed" ? "active" : ""}
                        onClick={() => setStatusFilter("failed")}
                        aria-pressed={statusFilter === "failed"}
                    >
                        <XCircle size={14} aria-hidden="true" />
                        Failed
                        <span className="count">{statusCounts.failed}</span>
                    </button>
                </fieldset>

                <fieldset className="layout-toggle" aria-label="View layout">
                    <button
                        type="button"
                        className={layout === "grid" ? "active" : ""}
                        onClick={() => setLayout("grid")}
                        aria-label="Grid view"
                        aria-pressed={layout === "grid"}
                    >
                        <Grid3X3 size={18} aria-hidden="true" />
                    </button>
                    <button
                        type="button"
                        className={layout === "list" ? "active" : ""}
                        onClick={() => setLayout("list")}
                        aria-label="List view"
                        aria-pressed={layout === "list"}
                    >
                        <List size={18} aria-hidden="true" />
                    </button>
                    <button
                        type="button"
                        className={layout === "compact" ? "active" : ""}
                        onClick={() => setLayout("compact")}
                        aria-label="Compact view"
                        aria-pressed={layout === "compact"}
                    >
                        <LayoutList size={18} aria-hidden="true" />
                    </button>
                </fieldset>
            </div>

            {/* Content */}
            <div className="assets-content">
                {loading && allAssets.length === 0 ? (
                    <output className="assets-loading" aria-live="polite">
                        <RefreshCw size={24} className="spin" aria-hidden="true" />
                        <span>Loading assets...</span>
                    </output>
                ) : layout === "grid" ? (
                    <div className="assets-grid">
                        {filteredNfts.map(renderGridCard)}
                        {filteredNfts.length === 0 && (
                            <div className="empty-state">
                                <Image size={48} />
                                <p>No assets found</p>
                            </div>
                        )}
                    </div>
                ) : layout === "list" ? (
                    <div className="assets-list">
                        {filteredAssets.map(renderListRow)}
                        {filteredAssets.length === 0 && (
                            <div className="empty-state">
                                <Image size={48} />
                                <p>No assets found</p>
                            </div>
                        )}
                    </div>
                ) : (
                    <div className="assets-compact">
                        <table>
                            <thead>
                                <tr>
                                    <th>NFT</th>
                                    <th>Type</th>
                                    <th>Status</th>
                                    <th>Size</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>{filteredAssets.map(renderCompactRow)}</tbody>
                        </table>
                        {filteredAssets.length === 0 && (
                            <div className="empty-state">
                                <Image size={48} />
                                <p>No assets found</p>
                            </div>
                        )}
                    </div>
                )}
            </div>

            {/* Pagination */}
            <div className="assets-pagination">
                <button type="button" disabled={page === 1} onClick={() => setPage((p) => p - 1)}>
                    <ChevronLeft size={18} />
                    Previous
                </button>
                <span className="page-info">Page {page}</span>
                <button type="button" disabled={!hasMore} onClick={() => setPage((p) => p + 1)}>
                    Next
                    <ChevronRight size={18} />
                </button>
            </div>
        </div>
    );
}
