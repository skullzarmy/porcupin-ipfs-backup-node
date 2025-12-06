import { useState } from "react";
import { LayoutDashboard, Wallet, Image, Settings, HelpCircle, ChevronLeft, ChevronRight } from "lucide-react";
import { Logo } from "./Logo";

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

    return (
        <aside className={`sidebar ${isExpanded ? "expanded" : ""}`}>
            <div className="sidebar-header">
                <Logo size={28} className="sidebar-logo-img" />
                <span className="sidebar-title">Porcupin</span>
            </div>

            <nav className="sidebar-nav">
                {navItems.map((item) => (
                    <button
                        key={item.id}
                        type="button"
                        className={`sidebar-item ${activeTab === item.id ? "active" : ""}`}
                        onClick={() => onTabChange(item.id)}
                        title={item.label}
                    >
                        <span className="sidebar-icon">
                            <item.icon size={20} />
                        </span>
                        <span className="sidebar-label">{item.label}</span>
                    </button>
                ))}
            </nav>

            <div className="sidebar-footer">
                <button
                    type="button"
                    className="sidebar-toggle"
                    onClick={() => setIsExpanded(!isExpanded)}
                    title={isExpanded ? "Collapse" : "Expand"}
                >
                    {isExpanded ? <ChevronLeft size={16} /> : <ChevronRight size={16} />}
                </button>
            </div>
        </aside>
    );
}
