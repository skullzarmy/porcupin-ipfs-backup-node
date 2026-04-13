import { useState, useEffect, useCallback } from "react";
import {
    GetConfig,
    GetStorageInfo,
    UpdateSettings,
    ResetDatabase,
    GetIPFSRepoPath,
    GetIPFSHealth,
    GetStorageLocation,
    ListStorageLocations,
    BrowseForFolder,
    MigrateStorage,
    ValidateStoragePath,
    GetMigrationStatus,
    CancelMigration,
    DiscoverServers,
    TestRemoteConnection,
    isRemote,
} from "../lib/backend";
// Updates and Logs are always client-scoped — use Wails bindings directly
import {
    CheckForUpdates,
    ExportDiagnosticReportToFile,
    ExportLogsToFile,
    GetLogs,
    GetVersion,
    InstallUpdate,
    RestartApp,
} from "../../wailsjs/go/main/App";
import { useConnection } from "../lib/connection";
import { ConfirmModal } from "./ConfirmModal";
import UpdateModal from "./UpdateModal";
import { EventsOn, LogInfo, LogError } from "../../wailsjs/runtime/runtime";
import {
    AlertTriangle,
    Download,
    FileText,
    FolderOpen,
    HardDrive,
    Loader,
    Monitor,
    Moon,
    Plug,
    RefreshCcw,
    Search,
    Server,
    Sun,
    Unplug,
    Usb,
    Wifi,
    Check,
    X,
} from "lucide-react";
import type { api, logging, main, storage } from "../../wailsjs/go/models";
import { formatBytes, compareSemver } from "../utils";

interface ClearStatus {
    phase: string;
    message: string;
    total_pins: number;
    unpinned_count: number;
}

interface SettingsProps {
    onStatsChange: () => void;
    scrollToSection?: string;
    onScrolled?: () => void;
    clearing: boolean;
    setClearing: (clearing: boolean) => void;
    clearStatus: ClearStatus | null;
    setClearStatus: (status: ClearStatus | null) => void;
}

export function Settings({ onStatsChange, scrollToSection, onScrolled, clearing, setClearing, clearStatus, setClearStatus }: SettingsProps) {
    const [storageInfo, setStorageInfo] = useState<main.StorageInfo | null>(null);
    const [repoPath, setRepoPath] = useState("");
    const [saving, setSaving] = useState(false);
    const [message, setMessage] = useState("");
    const [appVersion, setAppVersion] = useState("");

    // Scroll to section handling
    useEffect(() => {
        if (scrollToSection) {
            // Small timeout to allow render
            const timer = setTimeout(() => {
                const element = document.getElementById(scrollToSection);
                if (element) {
                    element.scrollIntoView({ behavior: "smooth" });
                }
                onScrolled?.();
            }, 100);
            return () => clearTimeout(timer);
        }
    }, [scrollToSection, onScrolled]);

    // Form state
    const [maxStorageGB, setMaxStorageGB] = useState(0);
    const [storageWarningPct, setStorageWarningPct] = useState(80);
    const [maxConcurrency, setMaxConcurrency] = useState(5);
    const [minFreeDiskSpaceGB, setMinFreeDiskSpaceGB] = useState(5);
    const [syncOwned, setSyncOwned] = useState(true);
    const [syncCreated, setSyncCreated] = useState(true);
    const [ipfsSwarmPort, setIpfsSwarmPort] = useState(4001);
    const [ipfsPortChanged, setIpfsPortChanged] = useState(false);

    // Storage location state
    const [currentLocation, setCurrentLocation] = useState<storage.StorageLocation | null>(null);
    const [availableLocations, setAvailableLocations] = useState<storage.StorageLocation[]>([]);
    const [selectedPath, setSelectedPath] = useState("");
    const [customPath, setCustomPath] = useState("");
    const [pathError, setPathError] = useState("");
    const [migrating, setMigrating] = useState(false);
    const [migrationStatus, setMigrationStatus] = useState<storage.MigrationStatus | null>(null);

    // Update State
    const [showUpdateConfirm, setShowUpdateConfirm] = useState(false);
    const [updateInfo, setUpdateInfo] = useState<any>(null);
    const [updateCheckMsg, setUpdateCheckMsg] = useState("");
    const [showUpdateProgress, setShowUpdateProgress] = useState(false);
    const [updateError, setUpdateError] = useState("");
    const [updateSuccess, setUpdateSuccess] = useState(false);
    const [updateProgress, setUpdateProgress] = useState<
        { phase: string; message: string; percent: number } | undefined
    >(undefined);

    // Theme state
    // activeSection removed as it was unused
    const [theme, setTheme] = useState<"light" | "dark" | "system">(() => {
        const saved = localStorage.getItem("porcupin-theme");
        return (saved as "light" | "dark" | "system") || "dark";
    });

    // Confirmation dialog state
    const [showResetConfirm, setShowResetConfirm] = useState(false);

    // Remote server state
    const {
        state: connectionState,
        connect,
        disconnect,
        testRemoteConnection,
        isRemote: isRemoteConnected,
        getSavedConfigs,
        saveConfig,
        removeConfig,
        refreshServerVersion,
    } = useConnection();
    const [remoteHost, setRemoteHost] = useState("");
    const [remotePort, setRemotePort] = useState("8085");
    const [remoteToken, setRemoteToken] = useState("");
    const [remoteUseTLS, setRemoteUseTLS] = useState(false);
    const [remoteLabel, setRemoteLabel] = useState("");
    const [remoteTesting, setRemoteTesting] = useState(false);
    const [remoteConnecting, setRemoteConnecting] = useState(false);
    const [remoteError, setRemoteError] = useState("");
    const [remoteTestResult, setRemoteTestResult] = useState<string | null>(null);
    const [savedProfiles, setSavedProfiles] = useState<ReturnType<typeof getSavedConfigs>>([]);

    // Discovery state
    const [discoveredServers, setDiscoveredServers] = useState<api.DiscoveredServer[]>([]);
    const [scanning, setScanning] = useState(false);

    // IPFS connectivity check state
    const [connectivityChecking, setConnectivityChecking] = useState(false);
    const [connectivityResult, setConnectivityResult] = useState<{ online: boolean; peers: number; message: string } | null>(null);

    // Logs & Diagnostics state
    const [logs, setLogs] = useState<logging.Entry[]>([]);
    const [logLevel, setLogLevel] = useState("");
    const [logExporting, setLogExporting] = useState(false);
    const [diagExporting, setDiagExporting] = useState(false);
    const [exportError, setExportError] = useState("");
    const [exportSuccess, setExportSuccess] = useState("");

    const loadSettings = useCallback(async () => {
        try {
            // Always get client version from local Wails binding
            const clientVersion = await GetVersion();
            setAppVersion(clientVersion);

            // Use allSettled so one failure doesn't block the others
            const [cfgRes, storageRes, pathRes] = await Promise.allSettled([
                GetConfig(),
                GetStorageInfo(),
                GetIPFSRepoPath(),
            ]);

            if (storageRes.status === "fulfilled") setStorageInfo(storageRes.value);
            if (pathRes.status === "fulfilled") setRepoPath(pathRes.value);

            // Desktop-only: storage location management (not available in remote mode)
            if (!isRemote()) {
                try {
                    const [locationRes, locationsRes] = await Promise.all([
                        GetStorageLocation(),
                        ListStorageLocations(),
                    ]);
                    setCurrentLocation(locationRes);
                    setAvailableLocations(locationsRes || []);
                } catch (err) {
                    // Storage location features not available - that's ok
                    console.log("Storage location features not available:", err);
                }
            }

            // Populate form - note: Config uses PascalCase from Go struct
            const cfg = cfgRes.status === "fulfilled" ? cfgRes.value : null;
            if (cfg?.Backup) {
                setMaxStorageGB(cfg.Backup.max_storage_gb || 0);
                setStorageWarningPct(cfg.Backup.storage_warning_pct || 80);
                setMaxConcurrency(cfg.Backup.max_concurrency || 5);
                setMinFreeDiskSpaceGB(cfg.Backup.min_free_disk_space_gb || 5);
                setSyncOwned(cfg.Backup.sync_owned !== false);
                setSyncCreated(cfg.Backup.sync_created !== false);
            }
            if (cfg?.IPFS) {
                setIpfsSwarmPort(cfg.IPFS.swarm_port || 4001);
                setIpfsPortChanged(false);
            }
        } catch (err: unknown) {
            console.error(err);
        }
    }, []);

    // Apply theme when it changes
    useEffect(() => {
        document.documentElement.setAttribute("data-theme", theme);
        localStorage.setItem("porcupin-theme", theme);
    }, [theme]);

    const handleThemeChange = (newTheme: "light" | "dark" | "system") => {
        setTheme(newTheme);
    };

    // Load saved connection profiles
    useEffect(() => {
        setSavedProfiles(getSavedConfigs());
    }, [getSavedConfigs]);

    useEffect(() => {
        loadSettings();

        // Check if migration is already in progress (e.g., after navigating away and back)
        const checkMigrationStatus = async () => {
            try {
                const status = await GetMigrationStatus();
                if (status?.in_progress) {
                    console.log("Migration already in progress:", status);
                    setMigrating(true);
                    setMigrationStatus(status);
                }
            } catch (err: unknown) {
                console.error("Error checking migration status:", err);
            }
        };
        checkMigrationStatus();

        // Listen for migration events
        const unsubStart = EventsOn("storage:migration:start", (data) => {
            console.log("Migration started:", data);
            setMigrating(true);
        });
        const unsubProgress = EventsOn("storage:migration:progress", (status: storage.MigrationStatus) => {
            console.log("Migration progress:", status);
            setMigrationStatus(status);
        });
        const unsubError = EventsOn("storage:migration:error", (error: string) => {
            console.log("Migration error event:", error);
            setMigrating(false);
            setMessage("Migration failed: " + error);
        });
        const unsubComplete = EventsOn("storage:migration:complete", (data) => {
            console.log("Migration complete:", data);
            setMigrating(false);
            setMigrationStatus(null);
            setMessage("Migration complete!");
            loadSettings();
        });
        const unsubCancelled = EventsOn("storage:migration:cancelled", () => {
            console.log("Migration cancelled");
            setMigrating(false);
            setMigrationStatus(null);
            setMessage("Migration cancelled");
            loadSettings();
        });

        // Clear data events — state is managed in App.tsx, but we handle Settings-specific UI here
        const unsubClearError = EventsOn("clear:error", (status: any) => {
            setMessage("Clear failed: " + status.error);
        });
        const unsubClearComplete = EventsOn("clear:complete", (status: any) => {
            setShowResetConfirm(false);
            setMessage(`Cleared ${status.unpinned_count} pins. Re-sync your wallets to rebuild.`);
            loadSettings();
        });

        return () => {
            unsubStart();
            unsubProgress();
            unsubError();
            unsubComplete();
            unsubCancelled();
            unsubClearError();
            unsubClearComplete();
        };
    }, [loadSettings, onStatsChange]);

    useEffect(() => {
        const fetchLogs = async () => {
            try {
                const entries = await GetLogs(200, logLevel);
                setLogs(entries || []);
            } catch {
                // Non-fatal — log viewer is best-effort
            }
        };
        fetchLogs();
        const interval = setInterval(fetchLogs, 5000);
        return () => clearInterval(interval);
    }, [logLevel]);

    const handleCheckConnectivity = async () => {
        setConnectivityChecking(true);
        setConnectivityResult(null);
        try {
            const result = await GetIPFSHealth();
            setConnectivityResult({ online: result.is_online, peers: result.peer_count, message: result.message });
        } catch {
            setConnectivityResult({ online: false, peers: 0, message: "Connection check failed" });
        } finally {
            setConnectivityChecking(false);
        }
    };

    const handleSave = async () => {
        setSaving(true);
        setMessage("");
        try {
            await UpdateSettings({
                max_storage_gb: maxStorageGB,
                storage_warning_pct: storageWarningPct,
                max_concurrency: maxConcurrency,
                min_free_disk_space_gb: minFreeDiskSpaceGB,
                sync_owned: syncOwned,
                sync_created: syncCreated,
                ipfs_swarm_port: ipfsSwarmPort,
            });
            if (ipfsPortChanged) {
                setMessage("Settings saved! Restart the app for IPFS port change to take effect.");
            } else {
                setMessage("Settings saved!");
            }
            loadSettings();
        } catch (err: unknown) {
            setMessage("Error saving: " + String(err));
        } finally {
            setSaving(false);
        }
    };

    const handleBrowseFolder = async () => {
        try {
            const path = await BrowseForFolder();
            console.log("Selected folder:", path);
            if (path) {
                setCustomPath(path);
                setPathError("");
                // Validate the path
                try {
                    await ValidateStoragePath(path);
                    console.log("Path validation passed");
                } catch (err) {
                    console.error("Path validation failed:", err);
                    setPathError(String(err));
                }
            }
        } catch (err) {
            console.error("Browse folder error:", err);
        }
    };

    const handleSelectLocation = (path: string) => {
        setSelectedPath(path);
        setCustomPath("");
        setPathError("");
    };

    const handleMigrate = async () => {
        console.log("=== handleMigrate called ===");
        console.log("customPath:", customPath);
        console.log("selectedPath:", selectedPath);

        const targetPath = customPath || selectedPath;
        console.log("targetPath:", targetPath);

        if (!targetPath) {
            setPathError("Please select or enter a destination path");
            return;
        }

        // TODO: Add proper modal confirmation dialog (browser confirm() doesn't work in Wails)

        try {
            setPathError("");
            setMessage("Starting migration...");
            console.log("Starting migration to:", targetPath);
            await MigrateStorage(targetPath);
            console.log("MigrateStorage returned successfully");
            // Events will handle the rest
        } catch (err) {
            console.error("Migration error:", err);
            setMessage("Migration failed: " + String(err));
        }
    };

    const getStorageIcon = (type: string) => {
        switch (type) {
            case "external":
                return <Usb size={16} />;
            case "network":
                return <Wifi size={16} />;
            default:
                return <HardDrive size={16} />;
        }
    };

    const handleReset = async () => {
        if (clearing) return;
        try {
            setClearing(true);
            await ResetDatabase();
            // Events will handle the UI updates
        } catch (err) {
            setClearing(false);
            setClearStatus(null);
            setMessage("Error: " + String(err));
        }
    };

    return (
        <div className="settings">
            <div className="page-header">
                <div className="page-header-row">
                    <div>
                        <h1>Settings</h1>
                        <p className="page-subtitle">Configure backup and storage options</p>
                    </div>
                </div>
            </div>

            {/* Storage Info */}
            <div className="settings-section">
                <h3>Storage</h3>
                <div className="storage-info">
                    <div className="storage-stat">
                        <span className="label">Disk Usage:</span>
                        <span className="value">
                            {storageInfo?.disk_usage_bytes
                                ? formatBytes(storageInfo.disk_usage_bytes)
                                : "Calculating..."}
                        </span>
                    </div>
                    <div className="storage-stat">
                        <span className="label">Free Disk:</span>
                        <span className="value">{storageInfo?.free_disk_space_gb?.toFixed(1)} GB</span>
                    </div>
                    <div className="storage-stat">
                        <span className="label">IPFS Repo:</span>
                        <span className="value path">{repoPath}</span>
                    </div>
                    <div className="storage-stat">
                        <span className="label">Storage Type:</span>
                        <span className="value storage-type">
                            {getStorageIcon(currentLocation?.type || "local")}
                            {currentLocation?.type || "local"}
                        </span>
                    </div>
                    {storageInfo?.is_warning && (
                        <div className="storage-warning">
                            <AlertTriangle size={16} /> Storage usage at {storageInfo.usage_pct.toFixed(0)}% of limit
                        </div>
                    )}
                </div>
            </div>

            {/* Appearance */}
            <div className="settings-section">
                <h3>Appearance</h3>
                <div className="theme-switcher">
                    <span className="theme-label">Theme:</span>
                    <div className="theme-options">
                        <button
                            type="button"
                            className={`theme-option ${theme === "light" ? "active" : ""}`}
                            onClick={() => handleThemeChange("light")}
                        >
                            <Sun size={16} />
                            Light
                        </button>
                        <button
                            type="button"
                            className={`theme-option ${theme === "dark" ? "active" : ""}`}
                            onClick={() => handleThemeChange("dark")}
                        >
                            <Moon size={16} />
                            Dark
                        </button>
                        <button
                            type="button"
                            className={`theme-option ${theme === "system" ? "active" : ""}`}
                            onClick={() => handleThemeChange("system")}
                        >
                            <Monitor size={16} />
                            System
                        </button>
                    </div>
                </div>
            </div>

            {/* Software Update */}
            <div className="settings-section">
                <h3>Software Update</h3>
                <div className="storage-info">
                    <div className="storage-stat">
                        <span className="label">Current Version:</span>
                        <span className="value">{appVersion || "Loading..."}</span>
                        {isRemote() && connectionState.serverVersion && (
                            <span
                                className="value"
                                style={{ marginLeft: "12px", color: "var(--text-secondary)", fontSize: "13px" }}
                            >
                                (Server: v{connectionState.serverVersion})
                            </span>
                        )}
                    </div>
                    <div className="migration-actions" style={{ marginTop: "16px" }}>
                        <button
                            className="btn-primary"
                            onClick={async () => {
                                setUpdateCheckMsg("");
                                try {
                                    if (isRemote() && connectionState.remoteConfig) {
                                        await refreshServerVersion();
                                    }
                                    const info = await CheckForUpdates();
                                    if (info.available) {
                                        setUpdateInfo(info);
                                        setShowUpdateConfirm(true);
                                    } else {
                                        setUpdateCheckMsg("You're running the latest version.");
                                    }
                                } catch (err) {
                                    setUpdateCheckMsg("Error checking for updates: " + (err instanceof Error ? err.message : String(err)));
                                }
                            }}
                        >
                            Check for Updates
                        </button>
                        {updateCheckMsg && (
                            <span style={{ marginLeft: "12px", fontSize: "13px", color: "var(--text-secondary)" }}>
                                {updateCheckMsg}
                            </span>
                        )}
                    </div>
                </div>
            </div>

            <ConfirmModal
                isOpen={showUpdateConfirm}
                title="Update Available"
                message={`Version ${updateInfo?.version} is available. Would you like to update now?`}
                confirmText="Update Now"
                onConfirm={async () => {
                    setShowUpdateConfirm(false);
                    setShowUpdateProgress(true);
                    setUpdateError("");
                    setUpdateSuccess(false);
                    setUpdateProgress({ phase: "starting", message: "Starting update...", percent: 0 });

                    try {
                        await InstallUpdate();
                        setUpdateSuccess(true);
                    } catch (err: any) {
                        setUpdateError(err.toString());
                    }
                }}
                onCancel={() => setShowUpdateConfirm(false)}
            />

            {/* Listener for update progress */}
            {showUpdateProgress && <UpdateProgressListener onProgress={(data) => setUpdateProgress(data)} />}

            <UpdateModal
                isOpen={showUpdateProgress}
                error={updateError}
                success={updateSuccess}
                progress={updateProgress}
                onRestart={async () => {
                    try {
                        await RestartApp();
                    } catch (e: any) {
                        console.error("Restart failed", e);
                        setUpdateError("Restart failed: " + e.toString());
                    }
                }}
            />

            {/* Storage Location */}
            <div className="settings-section">
                <h3>
                    <HardDrive size={18} style={{ marginRight: 8, verticalAlign: "middle" }} />
                    Storage Location
                </h3>

                {migrating ? (
                    <div className="migration-progress">
                        <div className="migration-header">
                            <RefreshCcw size={16} className="spinning" />
                            <span>
                                {migrationStatus?.phase === "preparing" && "Preparing migration..."}
                                {migrationStatus?.phase === "copying" && "Copying files..."}
                                {migrationStatus?.phase === "cleanup" && "Cleaning up..."}
                                {migrationStatus?.phase === "complete" && "Migration complete!"}
                                {migrationStatus?.phase === "cancelled" && "Migration cancelled"}
                                {!migrationStatus?.phase && "Migrating storage..."}
                            </span>
                        </div>
                        {migrationStatus && (
                            <div className="migration-info">
                                <p>
                                    From: <code>{migrationStatus.source_path}</code>
                                </p>
                                <p>
                                    To: <code>{migrationStatus.dest_path}</code>
                                </p>
                                <p>
                                    Method: {migrationStatus.method === "rename" ? "Move (instant)" : "Rsync (copying)"}
                                </p>
                                {migrationStatus.current_file && (
                                    <p className="current-file">{migrationStatus.current_file}</p>
                                )}
                            </div>
                        )}
                        {migrationStatus?.method === "rsync" && migrationStatus.total_bytes > 0 && (
                            <>
                                <div className="progress-bar">
                                    <div className="progress-fill" style={{ width: `${migrationStatus.progress}%` }} />
                                </div>
                                <div className="migration-stats">
                                    <span>{migrationStatus.progress.toFixed(1)}%</span>
                                    <span>
                                        {formatBytes(migrationStatus.bytes_copied)} /{" "}
                                        {formatBytes(migrationStatus.total_bytes)}
                                    </span>
                                </div>
                            </>
                        )}
                        <div className="migration-actions">
                            <button
                                type="button"
                                onClick={async () => {
                                    try {
                                        await CancelMigration();
                                        setMigrating(false);
                                        setMigrationStatus(null);
                                        setMessage("Migration cancelled. Partial data may remain at destination.");
                                        loadSettings();
                                    } catch (err) {
                                        setMessage("Failed to cancel: " + String(err));
                                    }
                                }}
                                className="btn-danger"
                            >
                                <X size={16} /> Cancel Migration
                            </button>
                        </div>
                        <p className="migration-warning">
                            ⚠️ Cancelling will stop the transfer. Partial data at destination will need manual cleanup.
                        </p>
                    </div>
                ) : (
                    <>
                        <p className="section-description">
                            Move your IPFS data to a different drive. Supports local drives, USB drives, SD cards, and
                            network storage.
                        </p>

                        {/* Available locations */}
                        {availableLocations.length > 0 && (
                            <div className="location-list">
                                <span className="list-label">Quick Select:</span>
                                {availableLocations.map((loc) => (
                                    <button
                                        key={loc.path}
                                        type="button"
                                        className={`location-option ${selectedPath === loc.path ? "selected" : ""}`}
                                        onClick={() => handleSelectLocation(loc.path)}
                                        disabled={!loc.is_writable}
                                    >
                                        <span className="location-icon">{getStorageIcon(loc.type)}</span>
                                        <span className="location-details">
                                            <span className="location-label">{loc.label || loc.path}</span>
                                            <span className="location-meta">
                                                {formatBytes(loc.free_bytes)} free • {loc.type}
                                            </span>
                                        </span>
                                        {selectedPath === loc.path && <Check size={16} className="check-icon" />}
                                    </button>
                                ))}
                            </div>
                        )}

                        {/* Custom path */}
                        <div className="custom-path-section">
                            <label htmlFor="customPath">Or enter a custom path:</label>
                            <div className="custom-path-input">
                                <input
                                    id="customPath"
                                    type="text"
                                    value={customPath}
                                    onChange={(e) => {
                                        setCustomPath(e.target.value);
                                        setSelectedPath("");
                                    }}
                                    placeholder="/path/to/storage or smb://server/share"
                                />
                                <button type="button" onClick={handleBrowseFolder} className="btn-secondary">
                                    <FolderOpen size={16} />
                                    Browse
                                </button>
                            </div>
                            {pathError && (
                                <div className="path-error">
                                    <X size={14} /> {pathError}
                                </div>
                            )}
                        </div>

                        {/* Migrate button */}
                        <div className="migrate-actions">
                            <button
                                type="button"
                                onClick={handleMigrate}
                                disabled={(!selectedPath && !customPath) || !!pathError}
                                className="btn-primary"
                            >
                                Move Storage
                            </button>
                            <span className="hint">
                                {selectedPath || customPath
                                    ? `Will move to: ${customPath || selectedPath}`
                                    : "Select a destination above"}
                            </span>
                        </div>
                    </>
                )}
            </div>

            {/* Storage Limits */}
            <div className="settings-section">
                <h3>Storage Limits</h3>
                <div className="form-group">
                    <label htmlFor="maxStorageGB">Max Storage (GB)</label>
                    <input
                        id="maxStorageGB"
                        type="number"
                        value={maxStorageGB}
                        onChange={(e) => setMaxStorageGB(Number(e.target.value))}
                        min={0}
                    />
                    <span className="hint">0 = unlimited</span>
                </div>
                <div className="form-group">
                    <label htmlFor="storageWarningPct">Warning Threshold (%)</label>
                    <input
                        id="storageWarningPct"
                        type="number"
                        value={storageWarningPct}
                        onChange={(e) => setStorageWarningPct(Number(e.target.value))}
                        min={0}
                        max={100}
                    />
                </div>
                <div className="form-group">
                    <label htmlFor="minFreeDiskSpaceGB">Min Free Disk Space (GB)</label>
                    <input
                        id="minFreeDiskSpaceGB"
                        type="number"
                        value={minFreeDiskSpaceGB}
                        onChange={(e) => setMinFreeDiskSpaceGB(Number(e.target.value))}
                        min={1}
                    />
                </div>
            </div>

            {/* Performance */}
            <div className="settings-section">
                <h3>Performance</h3>
                <div className="form-group">
                    <label htmlFor="maxConcurrency">Max Concurrent Pins</label>
                    <input
                        id="maxConcurrency"
                        type="number"
                        value={maxConcurrency}
                        onChange={(e) => setMaxConcurrency(Number(e.target.value))}
                        min={1}
                        max={20}
                    />
                </div>
            </div>

            {/* IPFS Network */}
            <div className="settings-section">
                <h3>IPFS Network</h3>
                {isRemote() && (
                    <div className="info-notice">
                        <AlertTriangle size={14} />
                        <span>IPFS settings are managed on the remote server and cannot be changed here.</span>
                    </div>
                )}
                <div className="form-group">
                    <label htmlFor="ipfsSwarmPort">Swarm Port</label>
                    <input
                        id="ipfsSwarmPort"
                        type="number"
                        value={ipfsSwarmPort}
                        onChange={(e) => {
                            const newPort = Number(e.target.value);
                            setIpfsSwarmPort(newPort);
                            setIpfsPortChanged(true);
                        }}
                        min={1024}
                        max={65535}
                        disabled={isRemote()}
                    />
                    <span className="hint">Port for IPFS peer-to-peer connections (default: 4001)</span>
                    {ipfsPortChanged && !isRemote() && (
                        <div className="warning-notice">
                            <AlertTriangle size={14} />
                            <span>Changing the port requires an app restart to take effect.</span>
                        </div>
                    )}
                </div>
                <div className="form-group">
                    <label>IPFS Connectivity</label>
                    <button
                        type="button"
                        className="btn-secondary"
                        onClick={handleCheckConnectivity}
                        disabled={connectivityChecking}
                    >
                        {connectivityChecking ? <Loader size={14} className="spin" /> : <Wifi size={14} />}
                        {connectivityChecking ? "Checking…" : "Check Connectivity"}
                    </button>
                    {connectivityResult && (
                        <span
                            className="hint"
                            style={{ color: connectivityResult.online ? "var(--accent-success)" : "var(--accent-danger)" }}
                        >
                            {connectivityResult.online
                                ? `Online — ${connectivityResult.peers} peer${connectivityResult.peers === 1 ? "" : "s"} connected`
                                : `Offline — ${connectivityResult.message}`}
                        </span>
                    )}
                </div>
            </div>

            {/* Sync Defaults */}
            <div className="settings-section">
                <h3>Sync Defaults</h3>
                <p className="section-description">
                    Default settings for new wallets. These can be overridden per wallet.
                </p>
                <div className="toggle-options">
                    <div className="toggle-option">
                        <label htmlFor="syncOwned">
                            <input
                                id="syncOwned"
                                type="checkbox"
                                checked={syncOwned}
                                onChange={(e) => setSyncOwned(e.target.checked)}
                            />
                            <div className="toggle-content">
                                <span className="toggle-label">Sync Owned NFTs</span>
                                <span className="toggle-hint">Backup NFTs this wallet currently owns</span>
                            </div>
                        </label>
                    </div>
                    <div className="toggle-option">
                        <label htmlFor="syncCreated">
                            <input
                                id="syncCreated"
                                type="checkbox"
                                checked={syncCreated}
                                onChange={(e) => setSyncCreated(e.target.checked)}
                            />
                            <div className="toggle-content">
                                <span className="toggle-label">Sync Created NFTs</span>
                                <span className="toggle-hint">Backup NFTs this wallet minted (even if sold)</span>
                            </div>
                        </label>
                    </div>
                </div>
            </div>

            {/* Remote Server */}
            <div className="settings-section" id="remote-server-settings">
                <h3>
                    <Server size={18} />
                    Remote Server
                </h3>
                <p className="section-description">
                    Connect to a remote Porcupin server instead of using the local embedded backend.
                </p>

                {isRemoteConnected ? (
                    <div className="remote-connected">
                        <div className="connection-status connected">
                            <Plug size={16} />
                            <span>
                                Connected to {connectionState.remoteConfig?.host}:{connectionState.remoteConfig?.port}
                            </span>
                            {connectionState.serverVersion && (
                                <span className="server-version">v{connectionState.serverVersion}</span>
                            )}
                            {connectionState.serverVersion &&
                                appVersion &&
                                compareSemver(connectionState.serverVersion, appVersion) < 0 && (
                                    <div className="warning-notice" style={{ marginTop: "8px" }}>
                                        <AlertTriangle size={14} />
                                        <span>
                                            Server is running v{connectionState.serverVersion} — client is v{appVersion}
                                            . Update the server via <code>porcupin --update</code>.
                                        </span>
                                    </div>
                                )}
                        </div>
                        <button type="button" onClick={() => disconnect()} className="btn-secondary">
                            <Unplug size={14} />
                            Disconnect
                        </button>
                    </div>
                ) : (
                    <div className="remote-form">
                        {/* Network Discovery */}
                        <div className="discovery-section">
                            <div className="discovery-header">
                                <button
                                    type="button"
                                    onClick={async () => {
                                        setScanning(true);
                                        setDiscoveredServers([]);
                                        try {
                                            const servers = await DiscoverServers();
                                            setDiscoveredServers(servers || []);
                                        } catch (err) {
                                            console.error("Discovery failed:", err);
                                        } finally {
                                            setScanning(false);
                                        }
                                    }}
                                    disabled={scanning}
                                    className="btn-secondary"
                                >
                                    <Search size={14} />
                                    {scanning ? "Scanning..." : "Scan Network"}
                                </button>
                            </div>
                            {discoveredServers.length > 0 && (
                                <div className="discovered-servers">
                                    {discoveredServers.map((server) => (
                                        <button
                                            key={`${server.host}:${server.port}`}
                                            type="button"
                                            className="discovered-server"
                                            onClick={() => {
                                                setRemoteHost(server.host);
                                                setRemotePort(String(server.port));
                                                setRemoteUseTLS(server.useTLS);
                                                setRemoteError("");
                                                setRemoteTestResult(null);
                                            }}
                                        >
                                            <Server size={14} />
                                            <span className="server-name">{server.name}</span>
                                            <span className="server-host">
                                                {server.host}:{server.port}
                                            </span>
                                            {server.version && (
                                                <span className="server-version">v{server.version}</span>
                                            )}
                                        </button>
                                    ))}
                                </div>
                            )}
                            {!scanning && discoveredServers.length === 0 && (
                                <p className="hint">Click "Scan Network" to find Porcupin servers on your LAN</p>
                            )}
                        </div>

                        {/* Saved Profiles */}
                        {savedProfiles.length > 0 && (
                            <div className="saved-profiles">
                                <h4>Saved Servers</h4>
                                <div className="profile-list">
                                    {savedProfiles.map((profile) => (
                                        <div key={`${profile.host}:${profile.port}`} className="saved-profile">
                                            <button
                                                type="button"
                                                className="profile-select"
                                                onClick={() => {
                                                    setRemoteHost(profile.host);
                                                    setRemotePort(String(profile.port));
                                                    setRemoteToken(profile.token);
                                                    setRemoteUseTLS(profile.useTLS);
                                                    setRemoteLabel(profile.label || "");
                                                    setRemoteError("");
                                                    setRemoteTestResult(null);
                                                }}
                                            >
                                                <Server size={14} />
                                                <span className="profile-name">
                                                    {profile.label || `${profile.host}:${profile.port}`}
                                                </span>
                                                {profile.label && (
                                                    <span className="profile-host">
                                                        {profile.host}:{profile.port}
                                                    </span>
                                                )}
                                            </button>
                                            <button
                                                type="button"
                                                className="profile-remove"
                                                onClick={() => {
                                                    removeConfig(profile.host, profile.port);
                                                    setSavedProfiles(getSavedConfigs());
                                                }}
                                                title="Remove saved server"
                                            >
                                                <X size={14} />
                                            </button>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}

                        <div className="form-row">
                            <div className="form-group">
                                <label htmlFor="remoteHost">Host</label>
                                <input
                                    id="remoteHost"
                                    type="text"
                                    value={remoteHost}
                                    onChange={(e) => {
                                        setRemoteHost(e.target.value);
                                        setRemoteError("");
                                        setRemoteTestResult(null);
                                    }}
                                    placeholder="192.168.1.100 or hostname"
                                />
                            </div>
                            <div className="form-group form-group-small">
                                <label htmlFor="remotePort">Port</label>
                                <input
                                    id="remotePort"
                                    type="text"
                                    value={remotePort}
                                    onChange={(e) => {
                                        setRemotePort(e.target.value);
                                        setRemoteError("");
                                        setRemoteTestResult(null);
                                    }}
                                    placeholder="8085"
                                />
                            </div>
                        </div>
                        <div className="form-group">
                            <label htmlFor="remoteToken">API Token</label>
                            <input
                                id="remoteToken"
                                type="password"
                                value={remoteToken}
                                onChange={(e) => {
                                    setRemoteToken(e.target.value);
                                    setRemoteError("");
                                    setRemoteTestResult(null);
                                }}
                                placeholder="prcpn_..."
                            />
                        </div>
                        <div className="form-group toggle-group">
                            <label htmlFor="remoteUseTLS">
                                <input
                                    id="remoteUseTLS"
                                    type="checkbox"
                                    checked={remoteUseTLS}
                                    onChange={(e) => setRemoteUseTLS(e.target.checked)}
                                />
                                <span>Use TLS (HTTPS)</span>
                            </label>
                        </div>
                        <div className="form-group">
                            <label htmlFor="remoteLabel">Label (optional)</label>
                            <input
                                id="remoteLabel"
                                type="text"
                                value={remoteLabel}
                                onChange={(e) => setRemoteLabel(e.target.value)}
                                placeholder="My Home Server"
                            />
                            <span className="hint">A friendly name for this server</span>
                        </div>

                        {remoteError && (
                            <div className="remote-error">
                                <AlertTriangle size={14} />
                                {remoteError}
                            </div>
                        )}

                        {remoteTestResult && (
                            <div className="remote-success">
                                <Check size={14} />
                                {remoteTestResult}
                            </div>
                        )}

                        <div className="remote-actions">
                            <button
                                type="button"
                                onClick={async () => {
                                    LogInfo("[Settings] Test Connection button clicked");
                                    if (!remoteHost || !remoteToken) {
                                        setRemoteError("Host and token are required");
                                        return;
                                    }
                                    LogInfo(`[Settings] Testing connection to ${remoteHost}:${remotePort}`);
                                    setRemoteTesting(true);
                                    setRemoteError("");
                                    setRemoteTestResult(null);
                                    try {
                                        // Use Go binding directly to bypass WebView fetch restrictions
                                        const health = await TestRemoteConnection({
                                            host: remoteHost,
                                            port: parseInt(remotePort) || 8085,
                                            token: remoteToken,
                                            useTLS: remoteUseTLS,
                                        });
                                        LogInfo(`[Settings] Test Connection success: ${health.version}`);
                                        setRemoteTestResult(`Connection OK - Server v${health.version}`);
                                    } catch (err) {
                                        const errMsg = err instanceof Error ? err.message : "Connection failed";
                                        LogError(`[Settings] Test Connection error: ${errMsg}`);
                                        setRemoteError(errMsg);
                                    } finally {
                                        setRemoteTesting(false);
                                    }
                                }}
                                disabled={remoteTesting || remoteConnecting || !remoteHost || !remoteToken}
                                className="btn-secondary"
                            >
                                {remoteTesting ? "Testing..." : "Test Connection"}
                            </button>
                            <button
                                type="button"
                                onClick={async () => {
                                    if (!remoteHost || !remoteToken) {
                                        setRemoteError("Host and token are required");
                                        return;
                                    }
                                    setRemoteConnecting(true);
                                    setRemoteError("");
                                    try {
                                        const config = {
                                            host: remoteHost,
                                            port: parseInt(remotePort) || 8085,
                                            token: remoteToken,
                                            useTLS: remoteUseTLS,
                                            label: remoteLabel || undefined,
                                        };
                                        await connect(config);
                                        // Save profile on successful connection
                                        saveConfig(config);
                                        setSavedProfiles(getSavedConfigs());
                                    } catch (err) {
                                        setRemoteError(err instanceof Error ? err.message : "Connection failed");
                                    } finally {
                                        setRemoteConnecting(false);
                                    }
                                }}
                                disabled={remoteTesting || remoteConnecting || !remoteHost || !remoteToken}
                                className="btn-primary"
                            >
                                <Plug size={14} />
                                {remoteConnecting ? "Connecting..." : "Connect"}
                            </button>
                        </div>
                    </div>
                )}
            </div>

            {/* Actions */}
            <div className="settings-actions">
                <button type="button" onClick={handleSave} disabled={saving} className="btn-primary">
                    {saving ? "Saving..." : "Save Settings"}
                </button>
                {message && <span className="message">{message}</span>}
            </div>

            {/* Logs & Diagnostics */}
            <div className="settings-section" id="logs-diagnostics">
                <h3>
                    <FileText size={18} />
                    Logs &amp; Diagnostics
                    {isRemote() && <span className="hint" style={{ fontWeight: 400, fontSize: "12px", marginLeft: 6 }}>(local client)</span>}
                </h3>
                <div className="log-filter">
                    <button
                        type="button"
                        className={logLevel === "" ? "btn-primary" : "btn-secondary"}
                        onClick={() => setLogLevel("")}
                    >
                        All
                    </button>
                    <button
                        type="button"
                        className={logLevel === "WARN" ? "btn-primary" : "btn-secondary"}
                        onClick={() => setLogLevel("WARN")}
                    >
                        Warnings+
                    </button>
                    <button
                        type="button"
                        className={logLevel === "ERROR" ? "btn-primary" : "btn-secondary"}
                        onClick={() => setLogLevel("ERROR")}
                    >
                        Errors Only
                    </button>
                </div>
                <div className="log-list">
                    {logs.length === 0 ? (
                        <div className="log-empty">No log entries.</div>
                    ) : (
                        logs.map((entry, i) => (
                            <div key={i} className={`log-row log-level-${entry.level.toLowerCase()}`}>
                                <span className="log-time">
                                    {typeof entry.time === "string" ? entry.time.slice(0, 19).replace("T", " ") : ""}
                                </span>
                                <span className="log-badge">{entry.level}</span>
                                <span className="log-message">{entry.message}</span>
                            </div>
                        ))
                    )}
                </div>
                <div className="log-actions">
                    <button
                        type="button"
                        className="btn-secondary"
                        disabled={logExporting}
                        onClick={async () => {
                            setLogExporting(true);
                            setExportError("");
                            setExportSuccess("");
                            try {
                                const path = await ExportLogsToFile();
                                if (path) setExportSuccess(`Saved to: ${path}`);
                            } catch (err: unknown) {
                                setExportError("Export failed: " + String(err));
                            } finally {
                                setLogExporting(false);
                            }
                        }}
                    >
                        <Download size={14} />
                        {logExporting ? "Exporting..." : "Export Logs"}
                    </button>
                    <button
                        type="button"
                        className="btn-secondary"
                        disabled={diagExporting}
                        onClick={async () => {
                            setDiagExporting(true);
                            setExportError("");
                            setExportSuccess("");
                            try {
                                const path = await ExportDiagnosticReportToFile();
                                if (path) setExportSuccess(`Saved to: ${path}`);
                            } catch (err: unknown) {
                                setExportError("Export failed: " + String(err));
                            } finally {
                                setDiagExporting(false);
                            }
                        }}
                    >
                        <Download size={14} />
                        {diagExporting ? "Exporting..." : "Export Diagnostic Report"}
                    </button>
                </div>
                {exportSuccess && (
                    <p style={{ color: "var(--accent-success)", fontSize: "12px", margin: "4px 0 0" }}>
                        {exportSuccess}
                    </p>
                )}
                {exportError && (
                    <p style={{ color: "var(--accent-danger)", fontSize: "12px", margin: "4px 0 0" }}>
                        {exportError}
                    </p>
                )}
            </div>

            {/* Danger Zone */}
            <div className="settings-section danger-zone">
                <h3>Danger Zone</h3>
                {clearing ? (
                    <div className="clear-progress">
                        <div className="clear-header">
                            <RefreshCcw size={16} className="spinning" />
                            <span>
                                {clearStatus?.phase === "unpinning" && "Unpinning IPFS content..."}
                                {clearStatus?.phase === "garbage_collect" && "Running garbage collection..."}
                                {clearStatus?.phase === "clearing_db" && "Clearing database..."}
                                {!clearStatus?.phase && "Clearing data..."}
                            </span>
                        </div>
                        {clearStatus && (
                            <div className="clear-info">
                                <p>{clearStatus.message}</p>
                                {clearStatus.total_pins > 0 && (
                                    <>
                                        <div className="progress-bar">
                                            <div
                                                className="progress-fill"
                                                style={{
                                                    width: `${
                                                        (clearStatus.unpinned_count / clearStatus.total_pins) * 100
                                                    }%`,
                                                }}
                                            />
                                        </div>
                                        <div className="clear-stats">
                                            <span>
                                                {clearStatus.unpinned_count} / {clearStatus.total_pins} pins removed
                                            </span>
                                        </div>
                                    </>
                                )}
                            </div>
                        )}
                    </div>
                ) : showResetConfirm ? (
                    <div className="confirm-dialog">
                        <p>
                            This will unpin all IPFS content, run garbage collection, and clear the database. Wallets
                            will be kept.
                        </p>
                        <div className="confirm-actions">
                            <button type="button" onClick={handleReset} className="btn-danger">
                                Yes, Clear All Data
                            </button>
                            <button type="button" onClick={() => setShowResetConfirm(false)} className="btn-secondary">
                                Cancel
                            </button>
                        </div>
                    </div>
                ) : (
                    <>
                        <button type="button" onClick={() => setShowResetConfirm(true)} className="btn-danger">
                            Clear All Data
                        </button>
                        <p className="hint">
                            Unpins all content, frees disk space, and clears database. Keeps wallets.
                        </p>
                    </>
                )}
            </div>
        </div>
    );
}
// Helper component to manage event listener lifecycle
function UpdateProgressListener({ onProgress }: { onProgress: (data: any) => void }) {
    useEffect(() => {
        const cancel = EventsOn("update:progress", (data: any) => {
            console.log("[Settings] Update progress:", data);
            onProgress(data);
        });
        return () => cancel();
    }, [onProgress]);
    return null;
}
