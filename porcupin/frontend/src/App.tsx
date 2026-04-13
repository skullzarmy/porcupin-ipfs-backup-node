import { useState, useEffect, useCallback } from "react";
import "./App.css";
import { GetWallets, GetAssetStats } from "./lib/backend";
import { InstallUpdate, RestartApp } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime";
import type { db } from "../wailsjs/go/models";
import { Sidebar } from "./components/Sidebar";
import { Dashboard } from "./components/Dashboard";
import { Wallets } from "./components/Wallets";
import { Assets } from "./components/Assets";
import { Settings } from "./components/Settings";
import { About } from "./components/About";
import { ConnectionProvider, useConnection } from "./lib/connection";
import UpdateToast from "./components/UpdateToast";
import UpdateModal from "./components/UpdateModal";

/** Asset statistics returned from the backend */
interface AssetStats {
    nft_count: number;
    pinned: number;
    failed: number;
    failed_unavailable: number;
    pending: number;
    skipped: number;
    disk_usage_bytes: number;
    total_size_bytes: number;
}

function AppContent() {
    const [activeTab, setActiveTab] = useState("dashboard");
    const [wallets, setWallets] = useState<db.Wallet[]>([]);
    const [stats, setStats] = useState<Partial<AssetStats>>({});
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const [isStale, setIsStale] = useState(false);
    const [scrollToSection, setScrollToSection] = useState("");

    // Update State
    const [showUpdateToast, setShowUpdateToast] = useState(false);
    const [updateVersion, setUpdateVersion] = useState("");
    const [showUpdateModal, setShowUpdateModal] = useState(false);
    const [updateError, setUpdateError] = useState("");
    const [updateSuccess, setUpdateSuccess] = useState(false);
    const [updateProgress, setUpdateProgress] = useState<
        { phase: string; message: string; percent: number } | undefined
    >(undefined);

    // Global clearing state — persists across tab switches
    const [clearing, setClearing] = useState(false);
    const [clearStatus, setClearStatus] = useState<{
        phase: string;
        message: string;
        total_pins: number;
        unpinned_count: number;
    } | null>(null);

    // Get connection state to trigger reloads when it changes
    const { state } = useConnection();
    const isConnected = state.status === "connected";
    const connectionMode = state.mode;

    // Apply saved theme on mount
    useEffect(() => {
        const savedTheme = localStorage.getItem("porcupin-theme") || "dark";
        document.documentElement.setAttribute("data-theme", savedTheme);
    }, []);

    const updateStats = useCallback(async () => {
        try {
            const newStats = await GetAssetStats();
            console.log("[App] GetAssetStats returned:", newStats);
            setStats(newStats || {});
            setIsStale(false);
        } catch (err: unknown) {
            console.error("[App] GetAssetStats error:", err);
            // Don't clear stats on error to avoid UI blips - just show stale data
            // setStats({});
            setIsStale(true);
        }
    }, []);

    const loadWallets = useCallback(async () => {
        try {
            const res = await GetWallets();
            console.log("[App] GetWallets returned:", res?.length, "wallets");
            setWallets(res || []);
        } catch (err: unknown) {
            console.error("[App] GetWallets error:", err);
            // Don't clear wallets on error to avoid UI blips - just show stale data
            // setWallets([]);
        }
    }, []);

    const handleNavigate = (tab: string, section?: string) => {
        setActiveTab(tab);
        if (section) setScrollToSection(section);
    };

    // Clear data when connection mode changes (switching between local/remote)
    useEffect(() => {
        console.log("[App] Connection mode changed to:", connectionMode, "status:", state.status);
        // Only clear data when we are fully disconnected, not during transient connecting states
        if (state.status === "disconnected") {
            setWallets([]);
            setStats({});
        }
    }, [connectionMode, state.status]);

    // Reload data when connection status changes to connected
    useEffect(() => {
        if (isConnected) {
            console.log("[App] Connection status changed to connected, reloading data...");
            loadWallets();
            updateStats();
        }
    }, [isConnected, loadWallets, updateStats]);

    // Initial load and polling
    useEffect(() => {
        loadWallets();
        updateStats();
        const interval = setInterval(updateStats, 5000);
        return () => clearInterval(interval);
    }, [loadWallets, updateStats]);

    // Listen for updates
    useEffect(() => {
        const cancel = EventsOn("update:available", (info: any) => {
            console.log("[App] Update available:", info);
            setUpdateVersion(info.version);
            setShowUpdateToast(true);
        });
        return () => cancel();
    }, []);

    // Listen for clear data events at app level so state persists across tab switches
    useEffect(() => {
        const unsubStart = EventsOn("clear:start", (status: any) => {
            setClearing(true);
            setClearStatus(status);
        });
        const unsubProgress = EventsOn("clear:progress", (status: any) => {
            setClearStatus(status);
        });
        const unsubError = EventsOn("clear:error", (status: any) => {
            setClearing(false);
            setClearStatus(null);
        });
        const unsubComplete = EventsOn("clear:complete", () => {
            setClearing(false);
            setClearStatus(null);
            updateStats();
        });
        return () => {
            unsubStart();
            unsubProgress();
            unsubError();
            unsubComplete();
        };
    }, [updateStats]);

    // Listen for progress events when update is active
    useEffect(() => {
        if (!showUpdateModal) return;

        console.log("[App] Setting up update progress listener");
        const cancelProgress = EventsOn("update:progress", (data: any) => {
            console.log("[App] Update progress:", data);
            setUpdateProgress(data);
        });

        return () => {
            console.log("[App] Cleaning up update progress listener");
            cancelProgress();
        };
    }, [showUpdateModal]);

    const handleInstallUpdate = async () => {
        setShowUpdateToast(false);
        setShowUpdateModal(true);
        setUpdateError("");
        setUpdateSuccess(false);
        setUpdateProgress({ phase: "starting", message: "Starting update...", percent: 0 });

        try {
            await InstallUpdate();
            setUpdateSuccess(true);
        } catch (err: any) {
            setUpdateError(err.toString());
        }
    };

    return (
        <div className="app-layout">
            {showUpdateToast && (
                <UpdateToast
                    version={updateVersion}
                    onInstall={handleInstallUpdate}
                    onDismiss={() => setShowUpdateToast(false)}
                />
            )}

            <UpdateModal
                isOpen={showUpdateModal}
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

            {/* Skip link for keyboard navigation - WCAG 2.4.1 */}
            <a href="#main-content" className="skip-link">
                Skip to main content
            </a>

            <Sidebar activeTab={activeTab} onTabChange={setActiveTab} />

            <main className="main-content" id="main-content" tabIndex={-1}>
                {/* Drag region for window - macOS/Windows title bar area */}
                <div className="drag-region" style={{ "--wails-draggable": "drag" } as React.CSSProperties}></div>

                {error && (
                    <div className="error-banner" role="alert" aria-live="assertive">
                        <span>{error}</span>
                        <button type="button" onClick={() => setError("")} aria-label="Dismiss error">
                            ×
                        </button>
                    </div>
                )}

                {isStale && !error && (
                    <div className="stale-banner" role="status" aria-live="polite">
                        <span>⚠️ Connection unstable - Data may be stale</span>
                    </div>
                )}

                {activeTab === "dashboard" && (
                    <Dashboard stats={stats} walletCount={wallets.length} onNavigate={handleNavigate} />
                )}

                {activeTab === "wallets" && (
                    <Wallets
                        wallets={wallets}
                        loading={loading}
                        setLoading={setLoading}
                        setError={setError}
                        onWalletsChange={loadWallets}
                        onStatsChange={updateStats}
                    />
                )}

                {activeTab === "assets" && <Assets onStatsChange={updateStats} />}

                {activeTab === "settings" && <Settings onStatsChange={updateStats} scrollToSection={scrollToSection} onScrolled={() => setScrollToSection("")} clearing={clearing} setClearing={setClearing} clearStatus={clearStatus} setClearStatus={setClearStatus} />}

                {activeTab === "about" && <About />}
            </main>
        </div>
    );
}

function App() {
    return (
        <ConnectionProvider>
            <AppContent />
        </ConnectionProvider>
    );
}

export default App;
