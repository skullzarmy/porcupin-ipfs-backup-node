# Porcupin Security Audit Report

**Date**: December 11, 2025  
**Auditor**: Adversarial Code Review  
**Scope**: Full codebase security analysis  
**Version**: Current uncommitted changes on `main` branch

---

## Executive Summary

Porcupin is a Wails-based desktop application with a headless server mode for backing up Tezos NFT assets to IPFS. The codebase demonstrates **strong security fundamentals** in several areas (token handling, rate limiting, authentication middleware), but has **several areas requiring attention**, particularly around input validation, CORS configuration, and request body size limiting.

### Risk Rating Summary

| Category                        | Rating                   | Findings                                                          |
| ------------------------------- | ------------------------ | ----------------------------------------------------------------- |
| Authentication & Token Security | ✅ **Strong**            | Bcrypt hashing, constant-time comparison, secure file permissions |
| Input Validation                | ⚠️ **Needs Improvement** | Missing wallet address validation, no request body size limits    |
| CORS Configuration              | 🔴 **Critical**          | Wildcard CORS (`*`) contradicts documented mitigation             |
| Rate Limiting                   | ✅ **Strong**            | Dual-layer rate limiting implemented                              |
| Command Injection               | ✅ **Strong**            | All exec.Command calls use hardcoded commands                     |
| SQL Injection                   | ✅ **Strong**            | GORM ORM with parameterized queries                               |
| XSS                             | ✅ **Strong**            | No dangerous innerHTML with user input                            |
| Secrets Storage                 | ⚠️ **Medium**            | API tokens stored in localStorage (browser)                       |

---

## Critical Findings

### 1. CORS Wildcard Configuration (CRITICAL)

**File**: `porcupin/backend/api/middleware.go` (lines 23-35)

**Issue**: The CORS middleware sets `Access-Control-Allow-Origin: *`, which allows any website to make authenticated API requests if a token is leaked.

```go
func CORSMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")  // CRITICAL: Wildcard origin
        // ...
    })
}
```

**Documentation Contradiction**: The `docs/audit.md` states:

> "CORS allows only localhost origins by default (`http://localhost:*`, `http://127.0.0.1:*`)"

This is **not implemented** - the actual code uses `*`.

**Risk**: If an attacker obtains a valid API token, they can make authenticated requests from any malicious website the victim visits.

**Recommendation**:

-   Implement origin whitelist with configurable origins
-   Default to localhost only (`http://localhost:*`, `http://127.0.0.1:*`)
-   Add `--cors-origins` flag as documented

---

### 2. Missing Request Body Size Limit (HIGH)

**File**: `porcupin/backend/api/handlers.go`

**Issue**: JSON request bodies are decoded without size limits, allowing potential DoS via large payloads.

```go
func (h *Handlers) AddWallet(w http.ResponseWriter, r *http.Request) {
    var req AddWalletRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {  // No size limit
        // ...
    }
}
```

**Documentation Claim**: `docs/audit.md` states "All requests are limited to 1MB maximum body size" - but this is **only implemented for headers**, not body:

```go
// server.go line 131
MaxHeaderBytes: 1 << 20, // 1MB - This is HEADER limit only
```

**Risk**: Attackers can send multi-gigabyte request bodies to exhaust server memory.

**Recommendation**:

```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
```

---

### 3. Missing Wallet Address Validation (HIGH)

**File**: `porcupin/app.go` (lines 157-173), `porcupin/backend/api/handlers.go` (lines 244-303)

**Issue**: Wallet addresses are accepted without validation against Tezos address format.

```go
func (a *App) AddWallet(address string, alias string) error {
    wallet := &db.Wallet{
        Address: address,  // No validation
        // ...
    }
    // Saved directly to database
}
```

**Documentation Claim**: `docs/audit.md` states addresses are validated with regex `^(tz[1-3]|KT1)[a-zA-Z0-9]{33}$` - but **no such validation exists in the code**.

**Risk**:

-   Invalid addresses waste TZKT API calls
-   Potential for injection if addresses are used in string interpolation
-   Database pollution with invalid entries

**Recommendation**: Add validation function:

```go
func IsValidTezosAddress(addr string) bool {
    matched, _ := regexp.MatchString(`^(tz[1-3]|KT1)[a-zA-Z0-9]{33}$`, addr)
    return matched
}
```

---

## Medium Findings

### 4. API Token Storage in localStorage (MEDIUM)

**File**: `porcupin/frontend/src/lib/connection.tsx` (lines 289-292)

**Issue**: Remote server configurations including API tokens are stored in browser localStorage.

```typescript
localStorage.setItem(STORAGE_KEY_SAVED_SERVERS, JSON.stringify(limited));
// RemoteServerConfig includes token field
```

**Risk**:

-   XSS attacks could extract stored tokens
-   Tokens persist after logout
-   No encryption of sensitive data

**Recommendation**:

-   Don't store tokens in localStorage
-   Use session-only memory storage
-   If persistence is needed, encrypt tokens or store hashes only

---

### 5. Missing HTTPS Enforcement (MEDIUM)

**Issue**: The remote connection feature allows connecting to remote servers over HTTP without warning.

**File**: `porcupin/frontend/src/lib/proxy-api-client.ts`

```typescript
async addWallet(address: string, alias: string): Promise<void> {
    await this.post("/api/v1/wallets", { address, alias });
    // Token sent in Authorization header - potentially over HTTP
}
```

**Risk**: API tokens transmitted in plaintext over HTTP can be intercepted.

**Recommendation**:

-   Warn users when connecting over HTTP
-   Default to HTTPS
-   Add configuration option to require TLS

---

### 6. Path Traversal in Storage Migration (MEDIUM)

**File**: `porcupin/backend/storage/storage.go`

**Issue**: User-provided paths for storage migration aren't sanitized for path traversal.

**Risk**: Potential directory traversal attacks (e.g., `../../etc/passwd` on Linux).

**Recommendation**:

-   Use `filepath.Clean()` on all user-provided paths
-   Validate paths are within allowed directories
-   Reject absolute paths that don't match expected patterns

---

## Low Findings

### 7. Verbose Error Messages (LOW)

**File**: `porcupin/backend/api/handlers.go`

**Issue**: Some error messages expose internal details:

```go
WriteInternalError(w, "failed to get wallets: "+err.Error())
```

**Risk**: Information disclosure about internal database structure.

**Recommendation**: Log detailed errors server-side, return generic messages to clients.

---

### 8. Missing Security Headers (LOW)

**Issue**: The API doesn't set security headers like:

-   `X-Content-Type-Options: nosniff`
-   `X-Frame-Options: DENY`
-   `Strict-Transport-Security` (when TLS enabled)

**Recommendation**: Add security headers middleware.

---

### 9. Debug Mode Environment Variable (LOW)

**File**: `porcupin/main.go` (line 24)

```go
debugMode := os.Getenv("PORCUPIN_DEBUG") == "1"
```

**Risk**: If accidentally left enabled in production, exposes DevTools.

**Recommendation**: Log a warning when debug mode is enabled.

---

## Positive Security Observations

### ✅ Token Security (Excellent)

-   Tokens hashed with bcrypt before storage (`bcryptCost = 10`)
-   Constant-time comparison prevents timing attacks
-   Tokens shown only once at generation
-   Secure file permissions (0600) for token file
-   Format validation with `prcpn_` prefix and alphanumeric charset

### ✅ Rate Limiting (Good)

-   Dual-layer: per-IP (10/sec) and global (100/sec)
-   Token bucket algorithm with automatic cleanup
-   Rate limit headers returned for visibility

### ✅ SQL Injection Prevention (Excellent)

-   GORM ORM used throughout
-   All queries use parameterized statements
-   No raw SQL string concatenation

### ✅ Command Injection Prevention (Good)

-   All `exec.Command` calls use hardcoded command names
-   User input never passed directly to commands
-   Arguments validated before use

### ✅ Authentication Middleware (Good)

-   Bearer token validation on all non-health endpoints
-   Health endpoint exemption is intentional (for monitoring)
-   Failed auth attempts logged with IP

### ✅ IP Filtering (Good)

-   Private IP ranges correctly identified
-   `--allow-public` flag required for public access
-   X-Forwarded-For handling with proxy trust flag

---

## Recommendations Priority

| Priority      | Finding                             | Effort | Impact |
| ------------- | ----------------------------------- | ------ | ------ |
| P0 (Critical) | Fix CORS wildcard                   | Low    | High   |
| P1 (High)     | Add request body size limit         | Low    | High   |
| P1 (High)     | Implement wallet address validation | Low    | Medium |
| P2 (Medium)   | Secure localStorage token storage   | Medium | Medium |
| P2 (Medium)   | Add HTTPS enforcement for remote    | Medium | Medium |
| P3 (Low)      | Add security headers                | Low    | Low    |
| P3 (Low)      | Sanitize verbose error messages     | Low    | Low    |

---

## Code Samples for Critical Fixes

### Fix 1: CORS with Origin Whitelist

```go
// middleware.go
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
    allowedSet := make(map[string]bool)
    for _, o := range allowedOrigins {
        allowedSet[o] = true
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")

            // Check if origin is allowed
            if origin == "" || allowedSet[origin] || isLocalhostOrigin(origin) {
                if origin != "" {
                    w.Header().Set("Access-Control-Allow-Origin", origin)
                }
            }

            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization")
            w.Header().Set("Access-Control-Allow-Credentials", "true")

            if r.Method == "OPTIONS" {
                w.WriteHeader(http.StatusNoContent)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

func isLocalhostOrigin(origin string) bool {
    return strings.HasPrefix(origin, "http://localhost") ||
           strings.HasPrefix(origin, "http://127.0.0.1") ||
           strings.HasPrefix(origin, "https://localhost") ||
           strings.HasPrefix(origin, "https://127.0.0.1")
}
```

### Fix 2: Request Body Size Limit

```go
// handlers.go - add to beginning of each POST/PUT handler
func (h *Handlers) AddWallet(w http.ResponseWriter, r *http.Request) {
    // Limit request body to 1MB
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

    var req AddWalletRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        if err.Error() == "http: request body too large" {
            WriteBadRequest(w, "request body too large (max 1MB)")
            return
        }
        WriteBadRequest(w, "invalid JSON")
        return
    }
    // ...
}
```

### Fix 3: Wallet Address Validation

```go
// Add to porcupin/backend/db/validation.go
package db

import "regexp"

var tezosAddressRegex = regexp.MustCompile(`^(tz[1-3]|KT1)[a-zA-Z0-9]{33}$`)

func IsValidTezosAddress(address string) bool {
    return tezosAddressRegex.MatchString(address)
}

// Use in handlers:
if !db.IsValidTezosAddress(req.Address) {
    WriteBadRequest(w, "invalid Tezos address format")
    return
}
```

---

## Conclusion

The Porcupin codebase demonstrates good security awareness in most areas, particularly around authentication and token handling. However, the **critical CORS misconfiguration** and **missing input validation** contradict the security claims in the documentation and should be addressed immediately before any production deployment.

The discrepancy between documented mitigations and actual implementation suggests a need for automated security testing as part of the CI/CD pipeline to ensure documented security controls are actually present in the code.
