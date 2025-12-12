/**
 * Backend Abstraction Layer for Porcupin
 *
 * This module provides a unified interface that routes calls to either:
 * - Local Wails bindings (when running as desktop app)
 * - Remote API client via Go proxy (when connected to headless server)
 *
 * Components should import from this module instead of directly from
 * wailsjs/go/main/App to enable seamless local/remote switching.
 *
 * Usage:
 *   import { getBackend } from '../lib/backend';
 *   const backend = getBackend();
 *   const wallets = await backend.GetWallets();
 */

import * as WailsApp from "../../wailsjs/go/main/App";
import type { config, core, db, ipfs, main, storage } from "../../wailsjs/go/models";
import type { ProxyAPIClient } from "./proxy-api-client";

// =============================================================================
// Types
// =============================================================================

/**
 * Backend interface matching Wails App bindings
 * All methods return Promises for consistency between local and remote
 */
export interface Backend {
    // Wallet operations
    AddWallet(address: string, alias: string): Promise<void>;
    DeleteWallet(address: string, keepAssets: boolean): Promise<void>;
    DeleteWalletWithUnpin(address: string): Promise<void>;
    GetWallets(): Promise<db.Wallet[]>;
    SyncWallet(address: string): Promise<void>;
    UpdateWalletAlias(address: string, alias: string): Promise<void>;
    UpdateWalletSettings(address: string, syncOwned: boolean, syncCreated: boolean): Promise<void>;

    // Asset operations
    ClearFailed(): Promise<number>;
    DeleteAsset(id: number): Promise<void>;
    GetAssetGatewayURL(id: number): Promise<Record<string, string>>;
    GetAssetStats(): Promise<Record<string, number>>;
    GetAssets(page: number, limit: number, status: string): Promise<db.Asset[]>;
    GetFailedAssets(): Promise<db.Asset[]>;
    RepinAsset(id: number): Promise<void>;
    RepinZeroSizeAssets(): Promise<number>;
    ResyncAsset(id: number): Promise<void>;
    RetryAllFailed(): Promise<number>;
    RetryAsset(id: number): Promise<void>;
    UnpinAsset(id: number): Promise<void>;
    VerifyAsset(id: number): Promise<ipfs.VerifyResult>;

    // NFT operations
    GetNFTsWithAssets(page: number, limit: number): Promise<db.NFT[]>;

    // Service control
    GetRecentActivity(limit: number): Promise<db.Asset[]>;
    GetStatus(): Promise<Record<string, unknown>>;
    GetSyncProgress(): Promise<core.ServiceStatus>;
    IsBackupPaused(): Promise<boolean>;
    PauseBackup(): Promise<void>;
    ResumeBackup(): Promise<void>;
    VerifyAndFixPins(): Promise<Record<string, number>>;

    // Config
    GetConfig(): Promise<config.Config>;
    GetVersion(): Promise<string>;
    UpdateSettings(settings: Record<string, unknown>): Promise<void>;

    // Storage (some only available locally)
    GetIPFSRepoPath(): Promise<string>;
    GetStorageInfo(): Promise<main.StorageInfo>;

    // Desktop-only features (will throw in remote mode)
    BrowseForFolder(): Promise<string>;
    CancelMigration(): Promise<void>;
    GetMigrationStatus(): Promise<storage.MigrationStatus>;
    GetStorageLocation(): Promise<storage.StorageLocation>;
    GetStorageType(path: string): Promise<string>;
    ListStorageLocations(): Promise<storage.StorageLocation[]>;
    MigrateStorage(path: string): Promise<void>;
    PreviewAsset(id: number, maxSize: number): Promise<Record<string, unknown>>;
    ResetDatabase(): Promise<void>;
    ShowInFinder(): Promise<void>;
    ValidateStoragePath(path: string): Promise<void>;
}

// =============================================================================
// Connection State (module-level singleton)
// =============================================================================

let apiClient: ProxyAPIClient | null = null;
let isRemoteMode = false;

/**
 * Set the API client for remote mode
 * Called by ConnectionProvider when connecting to remote server
 */
export function setAPIClient(client: ProxyAPIClient | null): void {
    console.log("[Backend] setAPIClient called with:", client ? "ProxyAPIClient instance" : "null");
    apiClient = client;
    isRemoteMode = client !== null;
    console.log("[Backend] isRemoteMode now:", isRemoteMode);
}

/**
 * Check if currently in remote mode
 */
export function isRemote(): boolean {
    return isRemoteMode;
}

/**
 * Check if a feature is available in current mode
 */
export function isFeatureAvailable(_feature: "finder" | "browse" | "migrate" | "preview" | "reset"): boolean {
    if (!isRemoteMode) return true;
    // These features are not available in remote mode
    return false;
}

// =============================================================================
// Local Backend (Wails)
// =============================================================================

const localBackend: Backend = {
    // Wallet operations
    AddWallet: WailsApp.AddWallet,
    DeleteWallet: WailsApp.DeleteWallet,
    DeleteWalletWithUnpin: WailsApp.DeleteWalletWithUnpin,
    GetWallets: WailsApp.GetWallets,
    SyncWallet: WailsApp.SyncWallet,
    UpdateWalletAlias: WailsApp.UpdateWalletAlias,
    UpdateWalletSettings: WailsApp.UpdateWalletSettings,

    // Asset operations
    ClearFailed: WailsApp.ClearFailed,
    DeleteAsset: WailsApp.DeleteAsset,
    GetAssetGatewayURL: WailsApp.GetAssetGatewayURL,
    GetAssetStats: WailsApp.GetAssetStats,
    GetAssets: WailsApp.GetAssets,
    GetFailedAssets: WailsApp.GetFailedAssets,
    RepinAsset: WailsApp.RepinAsset,
    RepinZeroSizeAssets: WailsApp.RepinZeroSizeAssets,
    ResyncAsset: WailsApp.ResyncAsset,
    RetryAllFailed: WailsApp.RetryAllFailed,
    RetryAsset: WailsApp.RetryAsset,
    UnpinAsset: WailsApp.UnpinAsset,
    VerifyAsset: WailsApp.VerifyAsset,

    // NFT operations
    GetNFTsWithAssets: WailsApp.GetNFTsWithAssets,

    // Service control
    GetRecentActivity: WailsApp.GetRecentActivity,
    GetStatus: WailsApp.GetStatus,
    GetSyncProgress: WailsApp.GetSyncProgress,
    IsBackupPaused: WailsApp.IsBackupPaused,
    PauseBackup: WailsApp.PauseBackup,
    ResumeBackup: WailsApp.ResumeBackup,
    VerifyAndFixPins: WailsApp.VerifyAndFixPins,

    // Config
    GetConfig: WailsApp.GetConfig,
    GetVersion: WailsApp.GetVersion,
    UpdateSettings: WailsApp.UpdateSettings,

    // Storage
    GetIPFSRepoPath: WailsApp.GetIPFSRepoPath,
    GetStorageInfo: WailsApp.GetStorageInfo,

    // Desktop-only features
    BrowseForFolder: WailsApp.BrowseForFolder,
    CancelMigration: WailsApp.CancelMigration,
    GetMigrationStatus: WailsApp.GetMigrationStatus,
    GetStorageLocation: WailsApp.GetStorageLocation,
    GetStorageType: WailsApp.GetStorageType,
    ListStorageLocations: WailsApp.ListStorageLocations,
    MigrateStorage: WailsApp.MigrateStorage,
    PreviewAsset: WailsApp.PreviewAsset,
    ResetDatabase: WailsApp.ResetDatabase,
    ShowInFinder: WailsApp.ShowInFinder,
    ValidateStoragePath: WailsApp.ValidateStoragePath,
};

// =============================================================================
// Remote Backend (Proxy API Client wrapper)
// =============================================================================

function createRemoteBackend(client: ProxyAPIClient): Backend {
    return {
        // Wallet operations
        AddWallet: (address, alias) => client.addWallet(address, alias),
        DeleteWallet: (address, keepAssets) => client.deleteWallet(address, keepAssets),
        DeleteWalletWithUnpin: (address) => client.deleteWalletWithUnpin(address),
        GetWallets: () => client.getWallets(),
        SyncWallet: (address) => client.syncWallet(address),
        UpdateWalletAlias: (address, alias) => client.updateWalletAlias(address, alias),
        UpdateWalletSettings: (address, syncOwned, syncCreated) =>
            client.updateWalletSettings(address, syncOwned, syncCreated),

        // Asset operations
        ClearFailed: () => client.clearFailed(),
        DeleteAsset: (id) => client.deleteAsset(id),
        GetAssetGatewayURL: (id) => client.getAssetGatewayURL(id),
        GetAssetStats: () => client.getAssetStats(),
        GetAssets: (page, limit, status) => client.getAssets(page, limit, status),
        GetFailedAssets: () => client.getFailedAssets(),
        RepinAsset: (id) => client.repinAsset(id),
        RepinZeroSizeAssets: () => client.repinZeroSizeAssets(),
        ResyncAsset: (id) => client.resyncAsset(id),
        RetryAllFailed: () => client.retryAllFailed(),
        RetryAsset: (id) => client.retryAsset(id),
        UnpinAsset: (id) => client.unpinAsset(id),
        VerifyAsset: (id) => client.verifyAsset(id),

        // NFT operations
        GetNFTsWithAssets: (page, limit) => client.getNFTsWithAssets(page, limit),

        // Service control
        GetRecentActivity: (limit) => client.getRecentActivity(limit),
        GetStatus: () => client.getStatus(),
        GetSyncProgress: () => client.getSyncProgress(),
        IsBackupPaused: () => client.isBackupPaused(),
        PauseBackup: () => client.pauseBackup(),
        ResumeBackup: () => client.resumeBackup(),
        VerifyAndFixPins: () => client.verifyAndFixPins(),

        // Config
        GetConfig: () => client.getConfig(),
        GetVersion: () => client.getVersion(),
        UpdateSettings: (settings) => client.updateSettings(settings),

        // Storage
        GetIPFSRepoPath: () => client.getIPFSRepoPath(),
        GetStorageInfo: () => client.getStorageInfo(),

        // Desktop-only features - throw in remote mode
        BrowseForFolder: () => client.browseForFolder(),
        CancelMigration: () => client.cancelMigration(),
        GetMigrationStatus: () => client.getMigrationStatus(),
        GetStorageLocation: () => client.getStorageLocation(),
        GetStorageType: (path) => client.getStorageType(path),
        ListStorageLocations: () => client.listStorageLocations(),
        MigrateStorage: (path) => client.migrateStorage(path),
        PreviewAsset: (id, maxSize) => client.previewAsset(id, maxSize),
        ResetDatabase: () => client.resetDatabase(),
        ShowInFinder: () => client.showInFinder(),
        ValidateStoragePath: (path) => client.validateStoragePath(path),
    };
}

// =============================================================================
// Public API
// =============================================================================

/**
 * Get the current backend instance
 * Returns remote backend if connected to remote server, otherwise local Wails backend
 */
export function getBackend(): Backend {
    console.log("[Backend] getBackend called, isRemoteMode:", isRemoteMode, "apiClient:", apiClient ? "set" : "null");
    if (isRemoteMode && apiClient) {
        console.log("[Backend] Returning REMOTE backend");
        return createRemoteBackend(apiClient);
    }
    console.log("[Backend] Returning LOCAL backend");
    return localBackend;
}

/**
 * Convenience re-exports for direct usage (routes based on current mode)
 * These allow drop-in replacement of Wails imports:
 *
 * Before: import { GetWallets } from '../wailsjs/go/main/App';
 * After:  import { GetWallets } from '../lib/backend';
 */
export const AddWallet = (...args: Parameters<Backend["AddWallet"]>) => getBackend().AddWallet(...args);
export const DeleteWallet = (...args: Parameters<Backend["DeleteWallet"]>) => getBackend().DeleteWallet(...args);
export const DeleteWalletWithUnpin = (...args: Parameters<Backend["DeleteWalletWithUnpin"]>) =>
    getBackend().DeleteWalletWithUnpin(...args);
export const GetWallets = () => getBackend().GetWallets();
export const SyncWallet = (...args: Parameters<Backend["SyncWallet"]>) => getBackend().SyncWallet(...args);
export const UpdateWalletAlias = (...args: Parameters<Backend["UpdateWalletAlias"]>) =>
    getBackend().UpdateWalletAlias(...args);
export const UpdateWalletSettings = (...args: Parameters<Backend["UpdateWalletSettings"]>) =>
    getBackend().UpdateWalletSettings(...args);

export const ClearFailed = () => getBackend().ClearFailed();
export const DeleteAsset = (...args: Parameters<Backend["DeleteAsset"]>) => getBackend().DeleteAsset(...args);
export const GetAssetGatewayURL = (...args: Parameters<Backend["GetAssetGatewayURL"]>) =>
    getBackend().GetAssetGatewayURL(...args);
export const GetAssetStats = () => getBackend().GetAssetStats();
export const GetAssets = (...args: Parameters<Backend["GetAssets"]>) => getBackend().GetAssets(...args);
export const GetFailedAssets = () => getBackend().GetFailedAssets();
export const RepinAsset = (...args: Parameters<Backend["RepinAsset"]>) => getBackend().RepinAsset(...args);
export const RepinZeroSizeAssets = () => getBackend().RepinZeroSizeAssets();
export const ResyncAsset = (...args: Parameters<Backend["ResyncAsset"]>) => getBackend().ResyncAsset(...args);
export const RetryAllFailed = () => getBackend().RetryAllFailed();
export const RetryAsset = (...args: Parameters<Backend["RetryAsset"]>) => getBackend().RetryAsset(...args);
export const UnpinAsset = (...args: Parameters<Backend["UnpinAsset"]>) => getBackend().UnpinAsset(...args);
export const VerifyAsset = (...args: Parameters<Backend["VerifyAsset"]>) => getBackend().VerifyAsset(...args);

export const GetNFTsWithAssets = (...args: Parameters<Backend["GetNFTsWithAssets"]>) =>
    getBackend().GetNFTsWithAssets(...args);

export const GetRecentActivity = (...args: Parameters<Backend["GetRecentActivity"]>) =>
    getBackend().GetRecentActivity(...args);
export const GetStatus = () => getBackend().GetStatus();
export const GetSyncProgress = () => getBackend().GetSyncProgress();
export const IsBackupPaused = () => getBackend().IsBackupPaused();
export const PauseBackup = () => getBackend().PauseBackup();
export const ResumeBackup = () => getBackend().ResumeBackup();
export const VerifyAndFixPins = () => getBackend().VerifyAndFixPins();

export const GetConfig = () => getBackend().GetConfig();
export const GetVersion = () => getBackend().GetVersion();
export const UpdateSettings = (...args: Parameters<Backend["UpdateSettings"]>) => getBackend().UpdateSettings(...args);

export const GetIPFSRepoPath = () => getBackend().GetIPFSRepoPath();
export const GetStorageInfo = () => getBackend().GetStorageInfo();

export const BrowseForFolder = () => getBackend().BrowseForFolder();
export const CancelMigration = () => getBackend().CancelMigration();
export const GetMigrationStatus = () => getBackend().GetMigrationStatus();
export const GetStorageLocation = () => getBackend().GetStorageLocation();
export const GetStorageType = (...args: Parameters<Backend["GetStorageType"]>) => getBackend().GetStorageType(...args);
export const ListStorageLocations = () => getBackend().ListStorageLocations();
export const MigrateStorage = (...args: Parameters<Backend["MigrateStorage"]>) => getBackend().MigrateStorage(...args);
export const PreviewAsset = (...args: Parameters<Backend["PreviewAsset"]>) => getBackend().PreviewAsset(...args);
export const ResetDatabase = () => getBackend().ResetDatabase();
export const ShowInFinder = () => getBackend().ShowInFinder();
export const ValidateStoragePath = (...args: Parameters<Backend["ValidateStoragePath"]>) =>
    getBackend().ValidateStoragePath(...args);

// Discovery and remote connection - only works in local/desktop mode
export const DiscoverServers = WailsApp.DiscoverServers;
export const TestRemoteConnection = WailsApp.TestRemoteConnection;
