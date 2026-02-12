export function formatBytes(bytes: number): string {
    if (!bytes) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

/** Compare two semver-like strings (e.g. "0.3.3-rc4"). Returns -1 if a < b, 0 if equal, 1 if a > b. */
export function compareSemver(a: string, b: string): number {
    const normalize = (v: string) => v.replace(/^v/, "").split(/[.-]/);
    const pa = normalize(a);
    const pb = normalize(b);
    for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
        const na = parseInt(pa[i] || "0", 10);
        const nb = parseInt(pb[i] || "0", 10);
        if (!isNaN(na) && !isNaN(nb)) {
            if (na !== nb) return na < nb ? -1 : 1;
        } else {
            const sa = pa[i] || "";
            const sb = pb[i] || "";
            if (sa < sb) return -1;
            if (sa > sb) return 1;
        }
    }
    return 0;
}

/**
 * Formats an error into a user-friendly string.
 * Handles:
 * - Error objects (uses .message)
 * - Strings (returns as is)
 * - Objects with 'error' or 'message' fields (common in Wails/API)
 * - Raw objects (JSON stringifies them)
 */
export function formatError(err: unknown): string {
    if (typeof err === "string") {
        return err;
    }
    
    if (err instanceof Error) {
        return err.message;
    }
    
    // Handle Wails/API error objects that might look like { error: "..." } or { message: "..." }
    if (typeof err === "object" && err !== null) {
        const record = err as Record<string, unknown>;
        if (typeof record.error === "string" && record.error) {
            return record.error;
        }
        if (typeof record.message === "string" && record.message) {
            return record.message;
        }
        
        // Try to handle Wails specific error structs like { "RejectionError": "..." }
        // We'll iterate keys and if there's a single key that looks like an error type, return its value
        const keys = Object.keys(record);
        if (keys.length === 1 && typeof record[keys[0]] === "string") {
            return `${keys[0]}: ${record[keys[0]]}`;
        }

        try {
            return JSON.stringify(err);
        } catch {
            return "Unknown error object";
        }
    }
    
    return String(err);
}
