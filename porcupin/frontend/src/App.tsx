import { useState, useEffect, useCallback } from "react";
import "./App.css";
import { GetWallets, GetAssetStats } from "../wailsjs/go/main/App";
import type { db } from "../wailsjs/go/models";
import { Sidebar } from "./components/Sidebar";
import { Dashboard } from "./components/Dashboard";
import { Wallets } from "./components/Wallets";
import { Assets } from "./components/Assets";
import { Settings } from "./components/Settings";
import { About } from "./components/About";

function App() {
    const [activeTab, setActiveTab] = useState("dashboard");
    const [wallets, setWallets] = useState<db.Wallet[]>([]);
    const [stats, setStats] = useState<{ [key: string]: number }>({});
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");

    // Apply saved theme on mount
    useEffect(() => {
        const savedTheme = localStorage.getItem("porcupin-theme") || "dark";
        document.documentElement.setAttribute("data-theme", savedTheme);
    }, []);

    const updateStats = useCallback(async () => {
        try {
            const newStats = await GetAssetStats();
            setStats(newStats || {});
        } catch (err) {
            console.error(err);
        }
    }, []);

    const loadWallets = useCallback(async () => {
        try {
            const res = await GetWallets();
            setWallets(res || []);
        } catch (err) {
            console.error(err);
        }
    }, []);

    useEffect(() => {
        loadWallets();
        updateStats();
        const interval = setInterval(updateStats, 5000);
        return () => clearInterval(interval);
    }, [loadWallets, updateStats]);

    return (
        <div className="app-layout">
            <Sidebar activeTab={activeTab} onTabChange={setActiveTab} />

            <main className="main-content">
                {/* Drag region for window - macOS/Windows title bar area */}
                <div className="drag-region" style={{ "--wails-draggable": "drag" } as React.CSSProperties}></div>

                {error && (
                    <div className="error-banner">
                        <span>{error}</span>
                        <button type="button" onClick={() => setError("")}>
                            ×
                        </button>
                    </div>
                )}

                {activeTab === "dashboard" && <Dashboard stats={stats} />}

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

export default App;
