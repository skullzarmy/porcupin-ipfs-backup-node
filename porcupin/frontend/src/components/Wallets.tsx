import { useState } from "react";
import { AddWallet, SyncWallet, GetWallets, UpdateWalletSettings } from "../../wailsjs/go/main/App";
import type { db } from "../../wailsjs/go/models";

interface WalletsProps {
    wallets: db.Wallet[];
    loading: boolean;
    setLoading: (loading: boolean) => void;
    setError: (error: string) => void;
    onWalletsChange: () => void;
    onStatsChange: () => void;
}

export function Wallets({ wallets, loading, setLoading, setError, onWalletsChange, onStatsChange }: WalletsProps) {
    const [newAddress, setNewAddress] = useState("");
    const [newAlias, setNewAlias] = useState("");

    const handleAddWallet = async () => {
        if (!newAddress) return;
        setLoading(true);
        setError("");
        try {
            await AddWallet(newAddress, newAlias);
            setNewAddress("");
            setNewAlias("");
            onWalletsChange();
            onStatsChange();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : String(err));
        } finally {
            setLoading(false);
        }
    };

    const handleSync = async (address?: string) => {
        setLoading(true);
        setError("");
        try {
            if (address) {
                await SyncWallet(address);
            } else {
                const currentWallets = await GetWallets();
                for (const w of currentWallets) {
                    await SyncWallet(w.address);
                }
            }
            onStatsChange();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : String(err));
        } finally {
            setLoading(false);
        }
    };

    const handleToggleSyncOwned = async (wallet: db.Wallet) => {
        try {
            await UpdateWalletSettings(wallet.address, !wallet.sync_owned, wallet.sync_created);
            onWalletsChange();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : String(err));
        }
    };

    const handleToggleSyncCreated = async (wallet: db.Wallet) => {
        try {
            await UpdateWalletSettings(wallet.address, wallet.sync_owned, !wallet.sync_created);
            onWalletsChange();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : String(err));
        }
    };

    return (
        <div className="wallets">
            <h2>Wallet Management</h2>
            <div className="add-wallet">
                <input
                    type="text"
                    placeholder="Enter Tezos Address (tz1...)"
                    value={newAddress}
                    onChange={(e) => setNewAddress(e.target.value)}
                />
                <input
                    type="text"
                    placeholder="Alias (Optional)"
                    value={newAlias}
                    onChange={(e) => setNewAlias(e.target.value)}
                />
                <button type="button" onClick={handleAddWallet} disabled={loading} className="btn-primary">
                    {loading ? "Adding..." : "Add Wallet"}
                </button>
            </div>

            <div className="actions">
                <button type="button" onClick={() => handleSync()} disabled={loading} className="btn-primary">
                    {loading ? "Syncing All..." : "Sync All Wallets"}
                </button>
            </div>

            <div className="wallets-list">
                {wallets.map((wallet) => (
                    <div key={wallet.address} className="wallet-card">
                        <div className="wallet-info">
                            <div className="wallet-address" title={wallet.address}>
                                {wallet.alias || wallet.address}
                            </div>
                            <div className="wallet-meta">{wallet.address}</div>
                        </div>
                        <div className="wallet-toggles">
                            <label className="toggle-label" title="Sync NFTs this wallet owns">
                                <input
                                    type="checkbox"
                                    checked={wallet.sync_owned !== false}
                                    onChange={() => handleToggleSyncOwned(wallet)}
                                />
                                <span>Owned</span>
                            </label>
                            <label className="toggle-label" title="Sync NFTs this wallet created">
                                <input
                                    type="checkbox"
                                    checked={wallet.sync_created !== false}
                                    onChange={() => handleToggleSyncCreated(wallet)}
                                />
                                <span>Created</span>
                            </label>
                        </div>
                        <button
                            type="button"
                            onClick={() => handleSync(wallet.address)}
                            disabled={loading}
                            className="btn-secondary"
                        >
                            Sync
                        </button>
                    </div>
                ))}
            </div>
        </div>
    );
}
