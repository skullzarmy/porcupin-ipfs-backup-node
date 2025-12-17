/**
 * Proxy API Client for Porcupin Remote Server
 *
 * This module provides an HTTP client that uses Go bindings to proxy requests
 * to a remote server. This bypasses Wails WebView restrictions that prevent
 * direct fetch() calls to external servers.
 *
 * Unlike api-client.ts which uses fetch(), this uses the RemoteProxy Go binding.
 */

import { RemoteProxy } from "../../wailsjs/go/main/App";
import type { config, core, db, ipfs, main, storage } from "../../wailsjs/go/models";

// =============================================================================
// Types
// =============================================================================

export interface ProxyAPIConfig {
    host: string;
    port: number;
    token: string;
    useTLS: boolean;
}

export interface APIError {
    error: string;
    code?: string;
}

// =============================================================================
// Proxy API Client Class
// =============================================================================

/**
 * API client that uses Go proxy binding to make HTTP requests to remote servers.
 * This allows the Wails WebView to communicate with external servers.
 */
export class ProxyAPIClient {
    private config: ProxyAPIConfig;

    constructor(config: ProxyAPIConfig) {
        this.config = config;
    }

    // =========================================================================
    // Internal Helpers
    // =========================================================================

    private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
        console.log(`[ProxyAPI] ${method} ${path}`);

        const proxyRequest = {
            host: this.config.host,
            port: this.config.port,
            token: this.config.token,
            useTLS: this.config.useTLS,
            method,
            path,
            body: body !== undefined ? JSON.stringify(body) : "",
        };

        const response = await RemoteProxy(proxyRequest);

        console.log(`[ProxyAPI] Response: ${response.statusCode}`);

        if (response.statusCode >= 400) {
            let errorMsg = `API request failed: ${response.statusCode}`;
            try {
                const errorData: APIError = JSON.parse(response.body);
                if (errorData.error) {
                    errorMsg = errorData.error;
                }
            } catch {
                // Use raw body as error message
                if (response.body) {
                    errorMsg = response.body;
                }
            }
            console.error(`[ProxyAPI] Error:`, errorMsg);
            throw new Error(errorMsg);
        }

        // Handle 204 No Content
        if (response.statusCode === 204 || !response.body) {
            return undefined as T;
        }

        return JSON.parse(response.body);
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

    async getHealth(): Promise<{ status: string; version: string; timestamp: string }> {
        return this.get("/api/v1/health");
    }

    async getVersion(): Promise<string> {
        const resp = await this.get<{ data: { version: string } }>("/api/v1/version");
        return resp.data.version;
    }

    async getStatus(): Promise<Record<string, unknown>> {
        const resp = await this.get<{ data: Record<string, unknown> }>("/api/v1/status");
        return resp.data;
    }

    async getStats(): Promise<Record<string, number>> {
        interface StatsResponse {
            data: {
                total_nfts: number;
                total_assets: number;
                pinned_assets: number;
                pending_assets: number;
                failed_assets: number;
                storage_used_gb: number;
            };
            meta: {
                timestamp: string;
            };
        }
        const response = await this.get<StatsResponse>("/api/v1/stats");
        const stats = response.data;
        console.log("[ProxyAPI] Raw stats from server:", stats);
        // Convert to the format expected by Dashboard component
        const result = {
            nft_count: stats.total_nfts,
            pinned: stats.pinned_assets,
            pending: stats.pending_assets,
            failed: stats.failed_assets,
            failed_unavailable: 0,
            disk_usage_bytes: Math.round(stats.storage_used_gb * 1024 * 1024 * 1024),
            total: stats.total_assets,
        };
        console.log("[ProxyAPI] Converted stats for Dashboard:", result);
        return result;
    }

    async getSyncProgress(): Promise<core.ServiceStatus> {
        const resp = await this.get<{ data: core.ServiceStatus }>("/api/v1/status");
        return resp.data;
    }

    async isBackupPaused(): Promise<boolean> {
        const resp = await this.get<{ data: { is_paused: boolean } }>("/api/v1/status");
        return resp.data?.is_paused ?? false;
    }

    async pauseBackup(): Promise<void> {
        await this.post("/api/v1/pause");
    }

    async resumeBackup(): Promise<void> {
        await this.post("/api/v1/resume");
    }

    // =========================================================================
    // Wallet Endpoints
    // =========================================================================

    async getWallets(): Promise<db.Wallet[]> {
        const resp = await this.get<{ data: db.Wallet[] }>("/api/v1/wallets");
        return resp.data || [];
    }

    async addWallet(address: string, alias: string): Promise<void> {
        await this.post("/api/v1/wallets", { address, alias });
    }

    async deleteWallet(address: string, keepAssets: boolean): Promise<void> {
        await this.delete(`/api/v1/wallets/${address}?keep_assets=${keepAssets}`);
    }

    async deleteWalletWithUnpin(address: string): Promise<void> {
        await this.delete(`/api/v1/wallets/${address}?keep_assets=false`);
    }

    async syncWallet(address: string): Promise<void> {
        await this.post(`/api/v1/wallets/${address}/sync`);
    }

    async updateWalletAlias(address: string, alias: string): Promise<void> {
        await this.put(`/api/v1/wallets/${address}`, { alias });
    }

    async updateWalletSettings(address: string, syncOwned: boolean, syncCreated: boolean): Promise<void> {
        await this.put(`/api/v1/wallets/${address}`, {
            sync_owned: syncOwned,
            sync_created: syncCreated,
        });
    }

    // =========================================================================
    // Asset Endpoints
    // =========================================================================

    async getAssets(page: number, limit: number, status: string, search: string): Promise<db.Asset[]> {
        const params = new URLSearchParams({
            page: page.toString(),
            limit: limit.toString(),
        });
        if (status && status !== "all") {
            params.append("status", status);
        }
        if (search) {
            params.append("search", search);
        }
        const resp = await this.get<{ data: { assets: db.Asset[] } }>(`/api/v1/assets?${params.toString()}`);
        return resp.data?.assets || [];
    }

    async getAssetStats(): Promise<Record<string, number>> {
        return this.getStats();
    }

    async getFailedAssets(): Promise<db.Asset[]> {
        const resp = await this.get<{ data: { assets: db.Asset[] } }>("/api/v1/assets/failed");
        return resp.data?.assets || [];
    }

    async retryAsset(id: number): Promise<void> {
        await this.post(`/api/v1/assets/${id}/retry`);
    }

    async retryAllFailed(): Promise<number> {
        const resp = await this.post<{ data: { count: number } }>("/api/v1/assets/retry-failed");
        return resp.data?.count || 0;
    }

    async clearFailed(): Promise<number> {
        const resp = await this.delete<{ data: { count: number } }>("/api/v1/assets/failed");
        return resp.data?.count || 0;
    }

    async deleteAsset(id: number): Promise<void> {
        await this.delete(`/api/v1/assets/${id}`);
    }

    async unpinAsset(id: number): Promise<void> {
        await this.delete(`/api/v1/assets/${id}`); // Assuming delete is equivalent to unpin if no keep_assets param
    }

    async repinAsset(id: number): Promise<void> {
        await this.post(`/api/v1/assets/${id}/repin`);
    }

    async repinZeroSizeAssets(): Promise<number> {
        // Not exposed via API yet
        return 0;
    }

    async resyncAsset(id: number): Promise<void> {
        // Not exposed via API yet
        return;
    }

    async verifyAsset(id: number): Promise<ipfs.VerifyResult> {
        const resp = await this.get<{ data: ipfs.VerifyResult }>(`/api/v1/assets/${id}/verify`);
        return resp.data;
    }

    async verifyAndFixPins(): Promise<Record<string, number>> {
        const resp = await this.post<{ data: Record<string, number> }>("/api/v1/verify-and-fix");
        return resp.data;
    }

    async getAssetGatewayURL(id: number): Promise<Record<string, string>> {
        const resp = await this.get<{ data: Record<string, string> }>(`/api/v1/assets/${id}/gateway`);
        return resp.data;
    }

    // =========================================================================
    // NFT Endpoints
    // =========================================================================

    async getNFTsWithAssets(page: number, limit: number): Promise<db.NFT[]> {
        const resp = await this.get<{ data: { nfts: db.NFT[] } }>(`/api/v1/nfts?page=${page}&limit=${limit}`);
        return resp.data?.nfts || [];
    }

    // =========================================================================
    // Activity Endpoints
    // =========================================================================

    async getRecentActivity(limit: number): Promise<db.Asset[]> {
        const resp = await this.get<{ data: db.Asset[] }>(`/api/v1/activity?limit=${limit}`);
        return resp.data || [];
    }

    // =========================================================================
    // Config Endpoints
    // =========================================================================

    async getConfig(): Promise<config.Config> {
        const resp = await this.get<{ data: config.Config }>("/api/v1/config");
        return resp.data;
    }

    async updateSettings(settings: Record<string, unknown>): Promise<void> {
        await this.put("/api/v1/config", settings);
    }

    // =========================================================================
    // Storage Endpoints
    // =========================================================================

    async getStorageInfo(): Promise<main.StorageInfo> {
        // The remote API doesn't have a dedicated /storage endpoint
        // Storage info is included in the /stats response
        interface StatsResponse {
            data: {
                storage_used_gb: number;
            };
        }
        const response = await this.get<StatsResponse>("/api/v1/stats");
        const storageUsedGB = response.data?.storage_used_gb || 0;
        const storageUsedBytes = Math.round(storageUsedGB * 1024 * 1024 * 1024);

        // Convert stats response to StorageInfo format
        // Most fields aren't available from remote API, but we provide what we can
        return {
            used_bytes: storageUsedBytes,
            used_gb: storageUsedGB,
            disk_usage_bytes: storageUsedBytes,
            disk_usage_gb: storageUsedGB,
            max_storage_gb: 0, // Not available from remote API
            warning_pct: 80, // Default
            usage_pct: 0, // Can't calculate without max
            is_warning: false,
            is_limit_reached: false,
            free_disk_space_gb: 0, // Not available from remote API
            repo_path: "", // Not available from remote API
        } as main.StorageInfo;
    }

    async getIPFSRepoPath(): Promise<string> {
        // Not available in remote mode - return empty string
        return "";
    }

    // =========================================================================
    // Desktop-only features - throw meaningful errors in remote mode
    // =========================================================================

    async browseForFolder(): Promise<string> {
        throw new Error("Browse for folder is not available in remote mode");
    }

    async cancelMigration(): Promise<void> {
        throw new Error("Migration is not available in remote mode");
    }

    async getMigrationStatus(): Promise<storage.MigrationStatus> {
        throw new Error("Migration status is not available in remote mode");
    }

    async getStorageLocation(): Promise<storage.StorageLocation> {
        throw new Error("Storage location is not available in remote mode");
    }

    async getStorageType(_path: string): Promise<string> {
        throw new Error("Storage type is not available in remote mode");
    }

    async listStorageLocations(): Promise<storage.StorageLocation[]> {
        throw new Error("List storage locations is not available in remote mode");
    }

    async migrateStorage(_path: string): Promise<void> {
        throw new Error("Migrate storage is not available in remote mode");
    }

    async previewAsset(_id: number, _maxSize: number): Promise<Record<string, unknown>> {
        throw new Error("Preview asset is not available in remote mode");
    }

    async resetDatabase(): Promise<void> {
        throw new Error("Reset database is not available in remote mode");
    }

    async showInFinder(): Promise<void> {
        throw new Error("Show in Finder is not available in remote mode");
    }

    async validateStoragePath(_path: string): Promise<void> {
        throw new Error("Validate storage path is not available in remote mode");
    }
}
