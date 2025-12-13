/**
 * API Client for Porcupin Remote Server
 *
 * This module provides an HTTP client that mirrors the Wails bindings,
 * allowing the GUI to connect to a remote headless server instead of
 * the local embedded backend.
 */

import type { config, core, db, ipfs, main, storage } from "../../wailsjs/go/models";

// =============================================================================
// Types
// =============================================================================

export interface APIConfig {
    host: string;
    port: number;
    token: string;
    useTLS: boolean;
}

export interface APIError {
    error: string;
    code?: string;
}

export interface HealthResponse {
    status: string;
    version: string;
    timestamp: string;
}

export interface VersionResponse {
    version: string;
}

export interface StatsResponse {
    total_nfts: number;
    total_assets: number;
    pinned_assets: number;
    pending_assets: number;
    failed_assets: number;
    storage_used_gb: number;
    wallets_count: number;
    service_state: string;
    last_sync_at?: string;
}

export interface PaginatedResponse<T> {
    data: T[];
    pagination: {
        page: number;
        limit: number;
        total: number;
        total_pages: number;
    };
}

export interface DiscoveredServer {
    name: string;
    host: string;
    port: number;
    version: string;
    useTLS: boolean;
    ips: string[];
}

// =============================================================================
// API Client Class
// =============================================================================

export class PorcupinAPIClient {
    private config: APIConfig;

    constructor(config: APIConfig) {
        this.config = config;
    }

    // =========================================================================
    // Internal Helpers
    // =========================================================================

    private get baseURL(): string {
        const protocol = this.config.useTLS ? "https" : "http";
        return `${protocol}://${this.config.host}:${this.config.port}`;
    }

    private get headers(): HeadersInit {
        return {
            Authorization: `Bearer ${this.config.token}`,
            "Content-Type": "application/json",
        };
    }

    private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
        const url = `${this.baseURL}${path}`;

        console.log(`[API] ${method} ${url}`);

        const init: RequestInit = {
            method,
            headers: this.headers,
            mode: "cors", // Explicitly enable CORS for WebView compatibility
        };

        if (body !== undefined) {
            init.body = JSON.stringify(body);
        }

        let response: Response;
        try {
            response = await fetch(url, init);
        } catch (err) {
            console.error(`[API] Network error for ${method} ${url}:`, err);
            throw new Error(`Network error: ${err instanceof Error ? err.message : "Failed to connect"}`);
        }

        console.log(`[API] Response: ${response.status} ${response.statusText}`);

        if (!response.ok) {
            let errorMsg = `API request failed: ${response.status} ${response.statusText}`;
            try {
                const errorData: APIError = await response.json();
                if (errorData.error) {
                    errorMsg = errorData.error;
                }
            } catch {
                // Ignore JSON parse errors
            }
            console.error(`[API] Error:`, errorMsg);
            throw new Error(errorMsg);
        }

        // Handle 204 No Content
        if (response.status === 204) {
            return undefined as T;
        }

        return response.json();
    }

    private async get<T>(path: string): Promise<T> {
        return this.request<T>("GET", path);
    }

    private async post<T>(path: string, body?: unknown): Promise<T> {
        return this.request<T>("POST", path, body);
    }

    private async put<T>(path: string, body?: unknown): Promise<T> {
        return this.request<T>("PUT", path, body);
    }

    private async delete<T>(path: string): Promise<T> {
        return this.request<T>("DELETE", path);
    }

    // =========================================================================
    // System Endpoints
    // =========================================================================

    /**
     * Check server health and connectivity
     */
    async getHealth(): Promise<HealthResponse> {
        return this.get<HealthResponse>("/api/v1/health");
    }

    /**
     * Get server version
     */
    async getVersion(): Promise<string> {
        const resp = await this.get<VersionResponse>("/api/v1/version");
        return resp.version;
    }

    /**
     * Discover Porcupin servers on the local network via mDNS
     */
    async discoverServers(timeout = 5): Promise<DiscoveredServer[]> {
        return this.get<DiscoveredServer[]>(`/api/v1/discover?timeout=${timeout}`);
    }

    /**
     * Get sync status and service state
     */
    async getStatus(): Promise<Record<string, unknown>> {
        return this.get<Record<string, unknown>>("/api/v1/status");
    }

    /**
     * Get sync progress (maps to GetSyncProgress Wails binding)
     */
    async getSyncProgress(): Promise<core.ServiceStatus> {
        return this.get<core.ServiceStatus>("/api/v1/status");
    }

    // =========================================================================
    // Statistics Endpoints
    // =========================================================================

    /**
     * Get current statistics (for Wails compatibility, returns mapped format)
     */
    async getAssetStats(): Promise<Record<string, number>> {
        const stats = await this.get<StatsResponse>("/api/v1/stats");
        // Map API response to Wails GetAssetStats format
        return {
            nft_count: stats.total_nfts,
            total: stats.total_assets,
            pinned: stats.pinned_assets,
            pending: stats.pending_assets,
            failed: stats.failed_assets,
        };
    }

    /**
     * Get raw stats response
     */
    async getStats(): Promise<StatsResponse> {
        return this.get<StatsResponse>("/api/v1/stats");
    }

    /**
     * Get recent activity
     */
    async getRecentActivity(limit: number): Promise<db.Asset[]> {
        return this.get<db.Asset[]>(`/api/v1/activity?limit=${limit}`);
    }

    // =========================================================================
    // Wallet Endpoints
    // =========================================================================

    /**
     * Get all wallets
     */
    async getWallets(): Promise<db.Wallet[]> {
        return this.get<db.Wallet[]>("/api/v1/wallets");
    }

    /**
     * Add a new wallet
     */
    async addWallet(address: string, alias: string): Promise<void> {
        await this.post("/api/v1/wallets", { address, alias });
    }

    /**
     * Delete a wallet
     */
    async deleteWallet(address: string, keepAssets: boolean): Promise<void> {
        await this.delete(`/api/v1/wallets/${address}?keep_assets=${keepAssets}`);
    }

    /**
     * Delete wallet and unpin all its assets
     */
    async deleteWalletWithUnpin(address: string): Promise<void> {
        await this.delete(`/api/v1/wallets/${address}?keep_assets=false`);
    }

    /**
     * Update wallet alias
     */
    async updateWalletAlias(address: string, alias: string): Promise<void> {
        await this.put(`/api/v1/wallets/${address}`, { alias });
    }

    /**
     * Update wallet settings (sync_owned, sync_created)
     */
    async updateWalletSettings(address: string, syncOwned: boolean, syncCreated: boolean): Promise<void> {
        await this.put(`/api/v1/wallets/${address}`, {
            sync_owned: syncOwned,
            sync_created: syncCreated,
        });
    }

    /**
     * Trigger sync for a specific wallet
     */
    async syncWallet(address: string): Promise<void> {
        await this.post(`/api/v1/wallets/${address}/sync`);
    }

    // =========================================================================
    // NFT Endpoints
    // =========================================================================

    /**
     * Get NFTs with pagination
     */
    async getNFTs(page: number, limit: number): Promise<PaginatedResponse<db.NFT>> {
        return this.get<PaginatedResponse<db.NFT>>(`/api/v1/nfts?page=${page}&limit=${limit}`);
    }

    /**
     * Get NFTs with their assets (for Wails compatibility)
     */
    async getNFTsWithAssets(page: number, limit: number): Promise<db.NFT[]> {
        const resp = await this.getNFTs(page, limit);
        return resp.data;
    }

    /**
     * Get single NFT
     */
    async getNFT(id: number): Promise<db.NFT> {
        return this.get<db.NFT>(`/api/v1/nfts/${id}`);
    }

    // =========================================================================
    // Asset Endpoints
    // =========================================================================

    /**
     * Get assets with pagination and optional status filter
     */
    async getAssets(page: number, limit: number, status?: string): Promise<db.Asset[]> {
        let path = `/api/v1/assets?page=${page}&limit=${limit}`;
        if (status) {
            path += `&status=${status}`;
        }
        const resp = await this.get<PaginatedResponse<db.Asset>>(path);
        return resp.data;
    }

    /**
     * Get failed assets
     */
    async getFailedAssets(): Promise<db.Asset[]> {
        // Get all failed assets (large limit for now)
        return this.getAssets(1, 1000, "failed");
    }

    /**
     * Retry a failed asset
     */
    async retryAsset(id: number): Promise<void> {
        await this.post(`/api/v1/assets/${id}/retry`);
    }

    /**
     * Retry all failed assets
     */
    async retryAllFailed(): Promise<number> {
        const resp = await this.post<{ retried: number }>("/api/v1/assets/retry-all");
        return resp.retried;
    }

    /**
     * Clear all failed assets
     */
    async clearFailed(): Promise<number> {
        const resp = await this.delete<{ cleared: number }>("/api/v1/assets/failed");
        return resp.cleared;
    }

    /**
     * Verify an asset
     */
    async verifyAsset(id: number): Promise<ipfs.VerifyResult> {
        return this.post<ipfs.VerifyResult>(`/api/v1/assets/${id}/verify`);
    }

    /**
     * Re-pin an asset
     */
    async repinAsset(id: number): Promise<void> {
        await this.post(`/api/v1/assets/${id}/repin`);
    }

    /**
     * Unpin an asset
     */
    async unpinAsset(id: number): Promise<void> {
        await this.post(`/api/v1/assets/${id}/unpin`);
    }

    /**
     * Delete an asset
     */
    async deleteAsset(id: number): Promise<void> {
        await this.delete(`/api/v1/assets/${id}`);
    }

    /**
     * Resync an asset
     */
    async resyncAsset(id: number): Promise<void> {
        await this.post(`/api/v1/assets/${id}/resync`);
    }

    /**
     * Re-pin zero-size assets
     */
    async repinZeroSizeAssets(): Promise<number> {
        const resp = await this.post<{ repinned: number }>("/api/v1/assets/repin-zero-size");
        return resp.repinned;
    }

    /**
     * Get asset gateway URL
     */
    async getAssetGatewayURL(id: number): Promise<Record<string, string>> {
        return this.get<Record<string, string>>(`/api/v1/assets/${id}/gateway-url`);
    }

    // =========================================================================
    // Service Control Endpoints
    // =========================================================================

    /**
     * Pause the backup service
     */
    async pauseBackup(): Promise<void> {
        await this.post("/api/v1/service/pause");
    }

    /**
     * Resume the backup service
     */
    async resumeBackup(): Promise<void> {
        await this.post("/api/v1/service/resume");
    }

    /**
     * Check if backup is paused
     */
    async isBackupPaused(): Promise<boolean> {
        const status = await this.getSyncProgress();
        return status.is_paused;
    }

    /**
     * Verify and fix pins
     */
    async verifyAndFixPins(): Promise<Record<string, number>> {
        return this.post<Record<string, number>>("/api/v1/service/verify-pins");
    }

    // =========================================================================
    // Config Endpoints
    // =========================================================================

    /**
     * Get configuration
     */
    async getConfig(): Promise<config.Config> {
        return this.get<config.Config>("/api/v1/config");
    }

    /**
     * Update settings
     */
    async updateSettings(settings: Record<string, unknown>): Promise<void> {
        await this.put("/api/v1/config", settings);
    }

    // =========================================================================
    // Storage Endpoints (Limited in Remote Mode)
    // =========================================================================

    /**
     * Get storage info
     */
    async getStorageInfo(): Promise<main.StorageInfo> {
        return this.get<main.StorageInfo>("/api/v1/storage/info");
    }

    /**
     * Get IPFS repo path (remote version)
     */
    async getIPFSRepoPath(): Promise<string> {
        const info = await this.getStorageInfo();
        return info.repo_path;
    }

    // =========================================================================
    // Unsupported in Remote Mode (Desktop-only features)
    // =========================================================================

    /**
     * These methods are not available in remote mode as they require
     * local desktop functionality. They will throw with a clear message.
     */

    browseForFolder(): Promise<string> {
        throw new Error("Browse for folder is not available in remote mode");
    }

    showInFinder(): Promise<void> {
        throw new Error("Show in Finder is not available in remote mode");
    }

    getStorageLocation(): Promise<storage.StorageLocation> {
        throw new Error("Storage location details not available in remote mode");
    }

    listStorageLocations(): Promise<storage.StorageLocation[]> {
        throw new Error("List storage locations not available in remote mode");
    }

    migrateStorage(_path: string): Promise<void> {
        throw new Error("Storage migration not available in remote mode");
    }

    getMigrationStatus(): Promise<storage.MigrationStatus> {
        throw new Error("Migration status not available in remote mode");
    }

    cancelMigration(): Promise<void> {
        throw new Error("Cancel migration not available in remote mode");
    }

    validateStoragePath(_path: string): Promise<void> {
        throw new Error("Validate storage path not available in remote mode");
    }

    getStorageType(_path: string): Promise<string> {
        throw new Error("Get storage type not available in remote mode");
    }

    previewAsset(_id: number, _maxSize: number): Promise<Record<string, unknown>> {
        throw new Error("Asset preview not available in remote mode");
    }

    resetDatabase(): Promise<void> {
        throw new Error("Reset database not available in remote mode");
    }
}

// =============================================================================
// Connection Testing & Discovery
// =============================================================================

/**
 * Test connection to a remote server
 * Returns the health response on success, throws on failure
 */
export async function testConnection(config: APIConfig): Promise<HealthResponse> {
    const client = new PorcupinAPIClient(config);
    return client.getHealth();
}

/**
 * Discover Porcupin servers on the local network via mDNS
 * This requires connecting to an existing server first (mDNS scan runs on server)
 * For GUI-only discovery without a server connection, this won't work
 */
export async function discoverServers(config: APIConfig, timeout = 5): Promise<DiscoveredServer[]> {
    const client = new PorcupinAPIClient(config);
    return client.discoverServers(timeout);
}

/**
 * Check if we're running in a Wails environment (desktop app)
 */
export function isWailsEnvironment(): boolean {
    // @ts-expect-error Wails runtime is injected globally
    return typeof window !== "undefined" && typeof window.go !== "undefined";
}

/**
 * Wait for Wails runtime to be available
 * Returns immediately if already available, otherwise polls until ready
 */
export async function waitForWails(timeoutMs = 5000): Promise<boolean> {
    if (isWailsEnvironment()) {
        return true;
    }

    const startTime = Date.now();
    return new Promise((resolve) => {
        const check = () => {
            if (isWailsEnvironment()) {
                resolve(true);
            } else if (Date.now() - startTime > timeoutMs) {
                resolve(false);
            } else {
                setTimeout(check, 50);
            }
        };
        check();
    });
}
