/**
 * Connection State Management for Porcupin
 *
 * This module provides React context and hooks for managing the connection
 * state between local (Wails) and remote (API) modes.
 */

import React, { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from "react";

import {
    PorcupinAPIClient,
    testConnection,
    isWailsEnvironment,
    type APIConfig,
    type HealthResponse,
} from "./api-client";
import { setAPIClient as setBackendAPIClient } from "./backend";

// =============================================================================
// Types
// =============================================================================

export type ConnectionMode = "local" | "remote";

export type ConnectionStatus = "disconnected" | "connecting" | "connected" | "error";

export interface RemoteServerConfig {
    host: string;
    port: number;
    token: string;
    useTLS: boolean;
    label?: string;
}

export interface ConnectionState {
    mode: ConnectionMode;
    status: ConnectionStatus;
    error?: string;
    serverVersion?: string;
    remoteConfig?: RemoteServerConfig;
}

export interface ConnectionContextValue {
    /** Current connection state */
    state: ConnectionState;

    /** The API client (null when in local mode) */
    apiClient: PorcupinAPIClient | null;

    /** Whether the app is running in Wails environment (desktop app) */
    isDesktopApp: boolean;

    /** Whether currently connected (local or remote) */
    isConnected: boolean;

    /** Whether in remote mode */
    isRemote: boolean;

    /** Connect to a remote server */
    connect: (config: RemoteServerConfig) => Promise<void>;

    /** Disconnect from remote server (switch to local if available) */
    disconnect: () => void;

    /** Test connection to a remote server without connecting */
    testRemoteConnection: (config: RemoteServerConfig) => Promise<HealthResponse>;

    /** Get saved remote configs from localStorage */
    getSavedConfigs: () => RemoteServerConfig[];

    /** Save a remote config to localStorage */
    saveConfig: (config: RemoteServerConfig) => void;

    /** Remove a saved config from localStorage */
    removeConfig: (host: string, port: number) => void;
}

// =============================================================================
// Local Storage Keys
// =============================================================================

const STORAGE_KEY_MODE = "porcupin_connection_mode";
const STORAGE_KEY_REMOTE_CONFIG = "porcupin_remote_config";
const STORAGE_KEY_SAVED_SERVERS = "porcupin_saved_servers";

// =============================================================================
// Context
// =============================================================================

const ConnectionContext = createContext<ConnectionContextValue | undefined>(undefined);

// =============================================================================
// Provider Component
// =============================================================================

interface ConnectionProviderProps {
    children: ReactNode;
}

export function ConnectionProvider({ children }: ConnectionProviderProps): React.ReactElement {
    const [state, setState] = useState<ConnectionState>(() => {
        // Initialize state from localStorage
        const savedMode = localStorage.getItem(STORAGE_KEY_MODE);
        const savedConfig = localStorage.getItem(STORAGE_KEY_REMOTE_CONFIG);

        const initialState: ConnectionState = {
            mode: "local",
            status: isWailsEnvironment() ? "connected" : "disconnected",
        };

        // If saved mode was remote and we have a config, try to restore it
        if (savedMode === "remote" && savedConfig) {
            try {
                initialState.mode = "remote";
                initialState.status = "disconnected";
                initialState.remoteConfig = JSON.parse(savedConfig);
            } catch {
                // Invalid saved config, stay in local mode
            }
        }

        return initialState;
    });

    const [apiClient, setApiClient] = useState<PorcupinAPIClient | null>(null);
    const isDesktopApp = isWailsEnvironment();

    // Attempt to reconnect to saved remote server on mount
    useEffect(() => {
        if (state.mode === "remote" && state.status === "disconnected" && state.remoteConfig) {
            // Auto-reconnect to saved remote server
            connectToRemote(state.remoteConfig);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const connectToRemote = useCallback(async (config: RemoteServerConfig): Promise<void> => {
        setState((prev) => ({
            ...prev,
            mode: "remote",
            status: "connecting",
            error: undefined,
        }));

        try {
            const apiConfig: APIConfig = {
                host: config.host,
                port: config.port,
                token: config.token,
                useTLS: config.useTLS,
            };

            // Test connection first
            const health = await testConnection(apiConfig);

            // Create client and update state
            const client = new PorcupinAPIClient(apiConfig);
            setApiClient(client);
            // Sync with backend module for routing
            setBackendAPIClient(client);

            setState({
                mode: "remote",
                status: "connected",
                serverVersion: health.version,
                remoteConfig: config,
            });

            // Persist to localStorage
            localStorage.setItem(STORAGE_KEY_MODE, "remote");
            localStorage.setItem(STORAGE_KEY_REMOTE_CONFIG, JSON.stringify(config));
        } catch (err) {
            const errorMsg = err instanceof Error ? err.message : "Connection failed";
            setState((prev) => ({
                ...prev,
                status: "error",
                error: errorMsg,
            }));
            throw err;
        }
    }, []);

    const disconnect = useCallback((): void => {
        setApiClient(null);
        // Sync with backend module for routing
        setBackendAPIClient(null);
        setState({
            mode: "local",
            status: isDesktopApp ? "connected" : "disconnected",
        });

        // Clear persisted remote state
        localStorage.removeItem(STORAGE_KEY_MODE);
        localStorage.removeItem(STORAGE_KEY_REMOTE_CONFIG);
    }, [isDesktopApp]);

    const testRemoteConnection = useCallback(async (config: RemoteServerConfig): Promise<HealthResponse> => {
        const apiConfig: APIConfig = {
            host: config.host,
            port: config.port,
            token: config.token,
            useTLS: config.useTLS,
        };
        return testConnection(apiConfig);
    }, []);

    const getSavedConfigs = useCallback((): RemoteServerConfig[] => {
        try {
            const saved = localStorage.getItem(STORAGE_KEY_SAVED_SERVERS);
            if (saved) {
                return JSON.parse(saved);
            }
        } catch {
            // Invalid JSON
        }
        return [];
    }, []);

    const saveConfig = useCallback((config: RemoteServerConfig): void => {
        const configs = getSavedConfigs();
        // Remove existing config with same host:port
        const filtered = configs.filter((c) => c.host !== config.host || c.port !== config.port);
        // Add new config at the start
        filtered.unshift(config);
        // Keep only last 10 configs
        const limited = filtered.slice(0, 10);
        localStorage.setItem(STORAGE_KEY_SAVED_SERVERS, JSON.stringify(limited));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const removeConfig = useCallback(
        (host: string, port: number): void => {
            const configs = getSavedConfigs();
            const filtered = configs.filter((c) => c.host !== host || c.port !== port);
            localStorage.setItem(STORAGE_KEY_SAVED_SERVERS, JSON.stringify(filtered));
        },
        // eslint-disable-next-line react-hooks/exhaustive-deps
        []
    );

    const value: ConnectionContextValue = {
        state,
        apiClient,
        isDesktopApp,
        isConnected: state.status === "connected" || (state.mode === "local" && isDesktopApp),
        isRemote: state.mode === "remote" && state.status === "connected",
        connect: connectToRemote,
        disconnect,
        testRemoteConnection,
        getSavedConfigs,
        saveConfig,
        removeConfig,
    };

    return <ConnectionContext.Provider value={value}>{children}</ConnectionContext.Provider>;
}

// =============================================================================
// Hooks
// =============================================================================

/**
 * Hook to access connection context
 */
export function useConnection(): ConnectionContextValue {
    const context = useContext(ConnectionContext);
    if (context === undefined) {
        throw new Error("useConnection must be used within a ConnectionProvider");
    }
    return context;
}

/**
 * Hook to check if a feature is available in current mode
 */
export function useFeatureAvailable(feature: "finder" | "browse" | "migrate" | "preview"): boolean {
    const { isRemote } = useConnection();

    // These features are only available in local mode
    const localOnlyFeatures = ["finder", "browse", "migrate", "preview"];

    if (localOnlyFeatures.includes(feature)) {
        return !isRemote;
    }

    return true;
}

/**
 * Hook to get connection status display info
 */
export function useConnectionStatus(): {
    label: string;
    color: "green" | "yellow" | "red" | "gray";
    icon: "local" | "remote" | "error" | "disconnected";
} {
    const { state, isDesktopApp } = useConnection();

    if (state.mode === "remote") {
        switch (state.status) {
            case "connected":
                return {
                    label: state.remoteConfig?.label || `${state.remoteConfig?.host}:${state.remoteConfig?.port}`,
                    color: "green",
                    icon: "remote",
                };
            case "connecting":
                return {
                    label: "Connecting...",
                    color: "yellow",
                    icon: "remote",
                };
            case "error":
                return {
                    label: state.error || "Connection error",
                    color: "red",
                    icon: "error",
                };
            default:
                return {
                    label: "Disconnected",
                    color: "gray",
                    icon: "disconnected",
                };
        }
    }

    // Local mode
    if (isDesktopApp) {
        return {
            label: "Local",
            color: "green",
            icon: "local",
        };
    }

    return {
        label: "No connection",
        color: "gray",
        icon: "disconnected",
    };
}
