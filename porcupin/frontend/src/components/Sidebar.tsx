import { useState } from "react";
import {
    LayoutDashboard,
    Wallet,
    Image,
    Settings,
    HelpCircle,
    ChevronLeft,
    ChevronRight,
    Monitor,
    Server,
} from "lucide-react";
import { Logo } from "./Logo";
import { useConnectionStatus } from "../lib/connection";

interface SidebarProps {
    activeTab: string;
    onTabChange: (tab: string) => void;
}

const navItems = [
    { id: "dashboard", icon: LayoutDashboard, label: "Dashboard" },
    { id: "wallets", icon: Wallet, label: "Wallets" },
    { id: "assets", icon: Image, label: "Assets" },
    { id: "settings", icon: Settings, label: "Settings" },
    { id: "about", icon: HelpCircle, label: "About" },
];

export function Sidebar({ activeTab, onTabChange }: SidebarProps) {
    const [isExpanded, setIsExpanded] = useState(false);
    const connectionStatus = useConnectionStatus();

    return (
        <aside className={`sidebar ${isExpanded ? "expanded" : ""}`}>
            <div className="sidebar-header">
                <Logo size={28} className="sidebar-logo-img" />
                <span className="sidebar-title">Porcupin</span>
            </div>

            <nav className="sidebar-nav" aria-label="Main navigation">
                {navItems.map((item) => (
                    <button
                        key={item.id}
                        type="button"
                        className={`sidebar-item ${activeTab === item.id ? "active" : ""}`}
                        onClick={() => onTabChange(item.id)}
                        aria-label={item.label}
                        aria-current={activeTab === item.id ? "page" : undefined}
                    >
                        <span className="sidebar-icon" aria-hidden="true">
                            <item.icon size={20} />
                        </span>
                        <span className="sidebar-label">{item.label}</span>
                    </button>
                ))}
            </nav>

            <div className="sidebar-footer">
                <output
                    className={`connection-indicator ${connectionStatus.icon === "remote" ? "remote" : "local"}`}
                    aria-label={`Connection: ${connectionStatus.label}`}
                >
                    {connectionStatus.icon === "remote" ? (
                        <Server size={14} aria-hidden="true" />
                    ) : (
                        <Monitor size={14} aria-hidden="true" />
                    )}
                    <span className="connection-label">{connectionStatus.label}</span>
                </output>
                <button
                    type="button"
                    className="sidebar-toggle"
                    onClick={() => setIsExpanded(!isExpanded)}
                    aria-label={isExpanded ? "Collapse sidebar" : "Expand sidebar"}
                    aria-expanded={isExpanded}
                >
                    {isExpanded ? (
                        <ChevronLeft size={16} aria-hidden="true" />
                    ) : (
                        <ChevronRight size={16} aria-hidden="true" />
                    )}
                </button>
            </div>
        </aside>
    );
}
