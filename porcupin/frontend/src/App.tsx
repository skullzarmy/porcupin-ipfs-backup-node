import { useState, useEffect, useCallback } from "react";
import "./App.css";
import { GetWallets, GetAssetStats } from "./lib/backend";
import type { db } from "../wailsjs/go/models";
import { Sidebar } from "./components/Sidebar";
import { Dashboard } from "./components/Dashboard";
import { Wallets } from "./components/Wallets";
import { Assets } from "./components/Assets";
import { Settings } from "./components/Settings";
import { About } from "./components/About";
import { ConnectionProvider, useConnection } from "./lib/connection";

/** Asset statistics returned from the backend */
interface AssetStats {
    nft_count: number;
    pinned: number;
    failed: number;
    failed_unavailable: number;
    pending: number;
    disk_usage_bytes: number;
    total_size_bytes: number;
}

function AppContent() {
    const [activeTab, setActiveTab] = useState("dashboard");
    const [wallets, setWallets] = useState<db.Wallet[]>([]);
    const [stats, setStats] = useState<Partial<AssetStats>>({});
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");

    // Get connection state to trigger reloads when it changes
    const { state } = useConnection();
    const isConnected = state.status === "connected";

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
        } catch (err: unknown) {
            console.error("[App] GetAssetStats error:", err);
        }
    }, []);

    const loadWallets = useCallback(async () => {
        try {
            const res = await GetWallets();
            console.log("[App] GetWallets returned:", res?.length, "wallets");
            setWallets(res || []);
        } catch (err: unknown) {
            console.error(err);
        }
    }, []);

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

    return (
        <div className="app-layout">
            {/* Skip link for keyboard navigation - WCAG 2.4.1 */}
            <a href="#main-content" className="skip-link">
                Skip to main content
            </a>

            <Sidebar activeTab={activeTab} onTabChange={setActiveTab} />

            <main className="main-content" id="main-content" tabIndex={-1}>
                {/* Drag region for window - macOS/Windows title bar area */}
                <div className="drag-region" style={{ "--wails-draggable": "drag" } as React.CSSProperties}></div>

                {error && (
                    <div className="error-banner" role="alert">
                        <span>{error}</span>
                        <button type="button" onClick={() => setError("")} aria-label="Dismiss error">
                            ×
                        </button>
                    </div>
                )}

                {activeTab === "dashboard" && <Dashboard stats={stats} walletCount={wallets.length} />}

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

                {activeTab === "settings" && <Settings onStatsChange={updateStats} />}

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
