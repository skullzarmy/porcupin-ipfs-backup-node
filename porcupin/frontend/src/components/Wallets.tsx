import { useState } from "react";
import {
    AddWallet,
    SyncWallet,
    GetWallets,
    UpdateWalletSettings,
    DeleteWallet,
    DeleteWalletWithUnpin,
    UpdateWalletAlias,
} from "../lib/backend";
import type { db } from "../../wailsjs/go/models";
import { ConfirmModal } from "./ConfirmModal";

interface WalletsProps {
    wallets: db.Wallet[];
    loading: boolean;
    setLoading: (loading: boolean) => void;
    setError: (error: string) => void;
    onWalletsChange: () => void;
    onStatsChange: () => void;
}

type DeleteMode = "keep-pins" | "unpin";

export function Wallets({ wallets, loading, setLoading, setError, onWalletsChange, onStatsChange }: WalletsProps) {
    const [newAddress, setNewAddress] = useState("");
    const [newAlias, setNewAlias] = useState("");
    const [walletToDelete, setWalletToDelete] = useState<db.Wallet | null>(null);
    const [deleteMode, setDeleteMode] = useState<DeleteMode>("keep-pins");
    const [editingWallet, setEditingWallet] = useState<string | null>(null);
    const [editAlias, setEditAlias] = useState("");

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

    const handleDeleteWallet = async (wallet: db.Wallet) => {
        setWalletToDelete(wallet);
        setDeleteMode("keep-pins");
    };

    const confirmDeleteWallet = async () => {
        if (!walletToDelete) return;
        setLoading(true);
        setError("");
        try {
            if (deleteMode === "unpin") {
                await DeleteWalletWithUnpin(walletToDelete.address);
            } else {
                await DeleteWallet(walletToDelete.address, true);
            }
            onWalletsChange();
            onStatsChange();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : String(err));
        } finally {
            setLoading(false);
            setWalletToDelete(null);
        }
    };

    const handleEditAlias = (wallet: db.Wallet) => {
        setEditingWallet(wallet.address);
        setEditAlias(wallet.alias || "");
    };

    const handleSaveAlias = async (address: string) => {
        try {
            await UpdateWalletAlias(address, editAlias);
            setEditingWallet(null);
            setEditAlias("");
            onWalletsChange();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : String(err));
        }
    };

    const handleCancelEdit = () => {
        setEditingWallet(null);
        setEditAlias("");
    };

    return (
        <div className="wallets">
            <div className="page-header">
                <div className="page-header-row">
                    <div>
                        <h1>Wallets</h1>
                        <p className="page-subtitle">
                            {wallets.length} wallet{wallets.length !== 1 ? "s" : ""} tracked
                        </p>
                    </div>
                    <div className="header-actions">
                        <button type="button" onClick={() => handleSync()} disabled={loading} className="btn-primary">
                            {loading ? "Syncing..." : "Sync All"}
                        </button>
                    </div>
                </div>
            </div>

            <div className="add-wallet">
                <input
                    type="text"
                    placeholder="Enter Tezos Address (tz1...)"
                    value={newAddress}
                    onChange={(e) => setNewAddress(e.target.value)}
                    aria-label="Tezos wallet address"
                />
                <input
                    type="text"
                    placeholder="Alias (Optional)"
                    value={newAlias}
                    onChange={(e) => setNewAlias(e.target.value)}
                    aria-label="Wallet alias (optional)"
                />
                <button type="button" onClick={handleAddWallet} disabled={loading} className="btn-primary">
                    {loading ? "Adding..." : "Add Wallet"}
                </button>
            </div>

            <div className="wallets-list">
                {wallets.map((wallet) => (
                    <div key={wallet.address} className="wallet-card">
                        <div className="wallet-info">
                            {editingWallet === wallet.address ? (
                                <div className="wallet-edit-alias">
                                    <input
                                        type="text"
                                        value={editAlias}
                                        onChange={(e) => setEditAlias(e.target.value)}
                                        placeholder="Enter alias"
                                        onKeyDown={(e) => {
                                            if (e.key === "Enter") handleSaveAlias(wallet.address);
                                            if (e.key === "Escape") handleCancelEdit();
                                        }}
                                    />
                                    <button
                                        type="button"
                                        onClick={() => handleSaveAlias(wallet.address)}
                                        className="btn-small btn-primary"
                                    >
                                        Save
                                    </button>
                                    <button
                                        type="button"
                                        onClick={handleCancelEdit}
                                        className="btn-small btn-secondary"
                                    >
                                        Cancel
                                    </button>
                                </div>
                            ) : (
                                <>
                                    <div className="wallet-address" title={wallet.address}>
                                        {wallet.alias || wallet.address}
                                        <button
                                            type="button"
                                            className="btn-icon"
                                            onClick={() => handleEditAlias(wallet)}
                                            title="Edit alias"
                                        >
                                            ✏️
                                        </button>
                                    </div>
                                    <div className="wallet-meta">{wallet.address}</div>
                                </>
                            )}
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
                        <div className="wallet-actions">
                            <button
                                type="button"
                                onClick={() => handleSync(wallet.address)}
                                disabled={loading}
                                className="btn-secondary"
                            >
                                Sync
                            </button>
                            <button
                                type="button"
                                onClick={() => handleDeleteWallet(wallet)}
                                disabled={loading}
                                className="btn-danger"
                                title="Delete wallet"
                            >
                                Delete
                            </button>
                        </div>
                    </div>
                ))}
            </div>

            <ConfirmModal
                isOpen={walletToDelete !== null}
                title="Delete Wallet"
                confirmText="Delete"
                cancelText="Cancel"
                onConfirm={confirmDeleteWallet}
                onCancel={() => setWalletToDelete(null)}
                isDangerous
            >
                <p className="modal-message">Delete wallet "{walletToDelete?.alias || walletToDelete?.address}"?</p>
                <p className="modal-message">
                    This will remove the wallet from tracking and delete associated NFT data from the database.
                </p>
                <div className="delete-options">
                    <label className="delete-option">
                        <input
                            type="radio"
                            name="deleteMode"
                            checked={deleteMode === "keep-pins"}
                            onChange={() => setDeleteMode("keep-pins")}
                        />
                        <span>Keep pinned content (faster, preserves IPFS data)</span>
                    </label>
                    <label className="delete-option">
                        <input
                            type="radio"
                            name="deleteMode"
                            checked={deleteMode === "unpin"}
                            onChange={() => setDeleteMode("unpin")}
                        />
                        <span>Unpin content (frees disk space after GC)</span>
                    </label>
                </div>
            </ConfirmModal>
        </div>
    );
}
