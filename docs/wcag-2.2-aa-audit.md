# WCAG 2.2 AA Accessibility Audit Report

**Application:** Porcupin - Tezos NFT Backup to IPFS  
**Platform:** Wails v2 Desktop Application (Go + React WebView)  
**Audit Date:** December 11, 2025  
**Standard:** WCAG 2.2 Level AA

---

## Executive Summary

This audit evaluates Porcupin's accessibility compliance against WCAG 2.2 Level AA guidelines, with special attention to Wails native application considerations. The application demonstrates basic accessibility implementation but requires improvements in several areas to achieve full compliance.

### Risk Assessment

| Severity | Count | Summary                                                                     |
| -------- | ----- | --------------------------------------------------------------------------- |
| Critical | 4     | Blocking issues preventing assistive technology users from completing tasks |
| Major    | 6     | Significant barriers affecting user experience                              |
| Minor    | 8     | Enhancements recommended for improved accessibility                         |

---

## Wails-Specific Considerations

### WebView Accessibility Context

Wails applications use a WebView (WebKit on macOS, WebView2 on Windows) which inherits native accessibility support. However, several considerations apply:

1. **Screen Reader Support:** WebView content is accessible to VoiceOver (macOS) and Narrator/NVDA (Windows) but requires proper ARIA implementation
2. **Keyboard Navigation:** WebView traps focus by default; proper tab order management is essential
3. **Native Window Controls:** The drag region and window controls bypass WebView - users must rely on OS-level shortcuts
4. **Focus Management:** Custom focus indicators must be visible since WebView may not show native focus rings

---

## Findings by WCAG Principle

## 1. Perceivable

### 1.1 Text Alternatives

#### 1.1.1 Non-text Content (Level A) - ⚠️ MAJOR ISSUE

**Finding:** Decorative icons lack proper hiding from assistive technology, and some images have inadequate alt text.

**Location:** Multiple components

**Evidence:**

```tsx
// Assets.tsx line 363 - Empty alt text is correct for decorative,
// but may need aria-hidden for consistency
<img src={previewUrl} alt="" loading="lazy" />

// Lucide icons throughout codebase lack aria-hidden
<RefreshCw size={16} className="spin" />  // No aria-hidden
<Trash2 size={14} />  // No aria-hidden
```

**Impact:** Screen readers may announce icon names or skip important content.

**Remediation:**

```tsx
// For decorative icons
<RefreshCw size={16} className="spin" aria-hidden="true" />

// For functional icons without text labels
<button type="button" aria-label="Refresh">
    <RefreshCw size={16} aria-hidden="true" />
</button>
```

---

### 1.3 Adaptable

#### 1.3.1 Info and Relationships (Level A) - 🔴 CRITICAL

**Finding:** Form inputs in Wallets.tsx lack proper label associations.

**Location:** `Wallets.tsx` lines 148-155, 219-235

**Evidence:**

```tsx
// Add wallet inputs have no labels or aria-labels
<input
    type="text"
    placeholder="Enter Tezos Address (tz1...)"
    value={newAddress}
    onChange={(e) => setNewAddress(e.target.value)}
/>

// Toggle checkboxes wrap input but lack explicit htmlFor
<label className="toggle-label" title="Sync NFTs this wallet owns">
    <input
        type="checkbox"
        checked={wallet.sync_owned !== false}
        onChange={() => handleToggleSyncOwned(wallet)}
    />
    <span>Owned</span>
</label>
```

**Impact:** Screen readers cannot properly identify form controls, making wallet management inaccessible.

**Remediation:**

```tsx
// Option 1: Visible labels
<label htmlFor="newAddress">Tezos Address</label>
<input
    id="newAddress"
    type="text"
    placeholder="tz1..."
    value={newAddress}
    onChange={(e) => setNewAddress(e.target.value)}
/>

// Option 2: aria-label for placeholder-only design
<input
    type="text"
    aria-label="Tezos wallet address"
    placeholder="Enter Tezos Address (tz1...)"
    value={newAddress}
    onChange={(e) => setNewAddress(e.target.value)}
/>
```

---

#### 1.3.2 Meaningful Sequence (Level A) - ⚠️ MAJOR ISSUE

**Finding:** Status filters in Assets toolbar may not convey grouping to screen readers.

**Location:** `Assets.tsx` lines 488-519

**Evidence:**

```tsx
<div className="status-filters">
    <button type="button" className={statusFilter === "all" ? "active" : ""}>
        All
    </button>
    // More buttons without group indication
</div>
```

**Impact:** Users may not understand these are related filter options.

**Remediation:**

```tsx
<div className="status-filters" role="group" aria-label="Filter by status">
    <button type="button" aria-pressed={statusFilter === "all"}>
        All
    </button>
    // ...
</div>
```

---

### 1.4 Distinguishable

#### 1.4.1 Use of Color (Level A) - ⚠️ MAJOR ISSUE

**Finding:** Status indicators rely solely on color to convey meaning.

**Location:** Dashboard status badges, Asset status indicators

**Evidence:**

```css
/* Status differentiation only via color */
.status-badge.watching {
    color: var(--accent-success);
}
.status-badge.syncing {
    color: var(--accent-info);
}
.status-badge.paused {
    color: var(--accent-warning);
}
```

**Impact:** Color-blind users cannot distinguish status states.

**Remediation:** Status badges already include text labels and icons (good), but ensure icons are always present and visible.

---

#### 1.4.3 Contrast (Minimum) (Level AA) - 🟡 NEEDS VERIFICATION

**Finding:** Several text elements may not meet 4.5:1 contrast ratio.

**Potential Issues:**

-   `--text-muted: #64748b` on `--bg-dark: #0a0a0f` - Estimated 4.2:1 (fails)
-   `--text-secondary: #94a3b8` on `--bg-dark: #0a0a0f` - Estimated 6.8:1 (passes)
-   Light theme `--text-muted: #64748b` on `--bg-dark: #f8fafc` - Estimated 4.5:1 (borderline)

**Locations:**

-   Hint text in Settings
-   Wallet meta addresses
-   Timestamps in activity list

**Remediation:**

```css
/* Increase muted text contrast */
:root,
[data-theme="dark"] {
    --text-muted: #8b95a5; /* Improved contrast */
}
```

---

#### 1.4.4 Resize Text (Level AA) - 🟢 PASSES

**Finding:** Application uses relative units (`rem`, `em`) inconsistently, but font sizes are generally adequate. Wails WebView respects system text scaling.

---

#### 1.4.10 Reflow (Level AA) - ⚠️ MINOR ISSUE

**Finding:** Desktop application has fixed layout, but content should reflow at 320px CSS width equivalent.

**Impact:** Users who zoom significantly may experience horizontal scrolling.

---

#### 1.4.11 Non-text Contrast (Level AA) - ⚠️ MAJOR ISSUE

**Finding:** Interactive element boundaries may not meet 3:1 contrast ratio against adjacent colors.

**Locations:**

-   Button borders in dark theme
-   Input field borders
-   Card borders

**Evidence:**

```css
/* Border color may have insufficient contrast */
--border-color: #1e1e2e; /* Against --bg-dark: #0a0a0f */
```

---

## 2. Operable

### 2.1 Keyboard Accessible

#### 2.1.1 Keyboard (Level A) - 🔴 CRITICAL

**Finding:** Multiple interactive elements are not keyboard accessible or have unclear focus states.

**Location:** Multiple components

**Evidence:**

```tsx
// FailedAssets.tsx - Modal backdrop uses biome-ignore for click handlers
// but lacks keyboard dismiss (Escape key support)
<div className="failed-assets-modal" onClick={onClose}>

// Settings.tsx - Theme buttons lack keyboard activation indication
<button
    type="button"
    className={`theme-option ${theme === "light" ? "active" : ""}`}
    onClick={() => handleThemeChange("light")}
>
```

**Impact:** Keyboard-only users cannot dismiss modals or navigate efficiently.

**Remediation:**

```tsx
// Add useEffect for Escape key handling
useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
        if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
}, [onClose]);
```

---

#### 2.1.2 No Keyboard Trap (Level A) - 🟢 PASSES

**Finding:** Modals properly trap focus when open and release it when closed.

---

#### 2.1.4 Character Key Shortcuts (Level A) - 🟢 PASSES (N/A)

**Finding:** No single-character keyboard shortcuts are implemented.

---

### 2.4 Navigable

#### 2.4.1 Bypass Blocks (Level A) - ⚠️ MINOR ISSUE

**Finding:** No skip navigation mechanism to bypass sidebar.

**Impact:** Screen reader users must tab through all sidebar items on every page.

**Remediation:**

```tsx
// Add skip link at top of App.tsx
<a href="#main-content" className="skip-link">
    Skip to main content
</a>
// ...
<main id="main-content" className="main-content" tabIndex={-1}>
```

```css
.skip-link {
    position: absolute;
    left: -9999px;
    z-index: 9999;
}
.skip-link:focus {
    left: 50%;
    transform: translateX(-50%);
    top: 0;
    padding: 8px 16px;
    background: var(--accent-primary);
    color: white;
}
```

---

#### 2.4.3 Focus Order (Level A) - ⚠️ MAJOR ISSUE

**Finding:** Focus order in modals may not be logical.

**Location:** `ConfirmModal.tsx`, `FailedAssets.tsx`

**Evidence:** Modal content div has `tabIndex={-1}` but initial focus is not set programmatically.

**Remediation:**

```tsx
// Use useRef and useEffect to focus first interactive element
const modalRef = useRef<HTMLDivElement>(null);
useEffect(() => {
    if (isOpen) {
        const firstFocusable = modalRef.current?.querySelector(
            'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        );
        (firstFocusable as HTMLElement)?.focus();
    }
}, [isOpen]);
```

---

#### 2.4.4 Link Purpose (In Context) (Level A) - 🟢 PASSES

**Finding:** Links have clear purpose from context or text.

---

#### 2.4.6 Headings and Labels (Level AA) - ⚠️ MINOR ISSUE

**Finding:** Heading hierarchy is inconsistent.

**Evidence:**

-   Dashboard uses `<h1>` for page title, `<h3>` for "Recent Activity" (skips h2)
-   Settings uses `<h1>` for page title, `<h3>` for sections (skips h2)

**Remediation:** Use proper heading hierarchy (h1 → h2 → h3).

---

#### 2.4.7 Focus Visible (Level AA) - 🔴 CRITICAL

**Finding:** Focus indicators are removed or insufficient in multiple locations.

**Location:** Multiple CSS files

**Evidence:**

```css
/* Focus explicitly removed */
.wallet-edit-alias input:focus {
    outline: none;
}
.search-box input {
    outline: none;
}
.add-wallet input:focus,
.form-group input:focus {
    outline: none;
}
```

**Impact:** Keyboard users cannot see which element has focus.

**Remediation:**

```css
/* Replace outline: none with visible focus indicator */
.wallet-edit-alias input:focus {
    outline: 2px solid var(--accent-primary);
    outline-offset: 2px;
}

/* Or use focus-visible for mouse/keyboard distinction */
input:focus-visible {
    outline: 2px solid var(--accent-primary);
    outline-offset: 2px;
}
```

---

#### 2.4.11 Focus Not Obscured (Minimum) (Level AA) - 🟢 PASSES

**Finding:** Focused elements are not obscured by other content.

---

### 2.5 Input Modalities

#### 2.5.3 Label in Name (Level A) - 🟡 NEEDS REVIEW

**Finding:** Some buttons with icons may not have accessible names matching visible labels.

**Evidence:**

```tsx
// Button text "Retry All (5)" but action is "retry"
<button type="button" className="btn-retry-all" onClick={handleRetryAll}>
    <RefreshCw size={14} /> Retry All ({assets.length})
</button>
```

**Status:** Generally compliant, but verify with assistive technology testing.

---

#### 2.5.7 Dragging Movements (Level AA) - 🟢 PASSES (N/A)

**Finding:** No drag-and-drop functionality requiring dragging movements.

---

#### 2.5.8 Target Size (Minimum) (Level AA) - ⚠️ MINOR ISSUE

**Finding:** Some interactive targets may be smaller than 24x24 CSS pixels.

**Locations:**

-   Compact table action buttons
-   Wallet edit icon button
-   Profile remove button (X icon)

**Evidence:**

```css
/* Small touch target */
.wallet-address .btn-icon {
    padding: 2px 4px; /* Results in ~20x20 target */
}
```

**Remediation:**

```css
.wallet-address .btn-icon {
    padding: 4px 6px;
    min-width: 24px;
    min-height: 24px;
}
```

---

## 3. Understandable

### 3.1 Readable

#### 3.1.1 Language of Page (Level A) - 🟢 PASSES

**Finding:** HTML lang attribute is set.

**Evidence:** `<html lang="en">` in `index.html`

---

#### 3.1.2 Language of Parts (Level AA) - 🟢 PASSES (N/A)

**Finding:** No content in languages other than English.

---

### 3.2 Predictable

#### 3.2.1 On Focus (Level A) - 🟢 PASSES

**Finding:** No unexpected context changes on focus.

---

#### 3.2.2 On Input (Level A) - 🟢 PASSES

**Finding:** Form controls don't cause unexpected navigation.

---

#### 3.2.3 Consistent Navigation (Level AA) - 🟢 PASSES

**Finding:** Sidebar navigation is consistent across all views.

---

#### 3.2.4 Consistent Identification (Level AA) - 🟢 PASSES

**Finding:** Similar functions use consistent naming and icons.

---

### 3.3 Input Assistance

#### 3.3.1 Error Identification (Level A) - ⚠️ MINOR ISSUE

**Finding:** Error messages are displayed visually but may not be announced to screen readers.

**Location:** Settings remote connection errors

**Evidence:**

```tsx
{
    remoteError && (
        <div className="remote-error">
            <AlertTriangle size={14} />
            {remoteError}
        </div>
    );
}
```

**Remediation:**

```tsx
{
    remoteError && (
        <div className="remote-error" role="alert" aria-live="polite">
            <AlertTriangle size={14} aria-hidden="true" />
            {remoteError}
        </div>
    );
}
```

---

#### 3.3.2 Labels or Instructions (Level A) - 🔴 CRITICAL

**Finding:** Several form inputs lack visible labels.

**Locations:**

-   Wallet address input (placeholder only)
-   Wallet alias input (placeholder only)
-   Search input (icon only)
-   Assets page search

**Impact:** Users with cognitive disabilities may not understand input purpose without labels.

---

#### 3.3.3 Error Suggestion (Level AA) - ⚠️ MINOR ISSUE

**Finding:** Some error messages don't provide specific correction suggestions.

**Example:** "Connection failed" without explaining why or how to fix.

---

#### 3.3.4 Error Prevention (Legal, Financial, Data) (Level AA) - 🟢 PASSES

**Finding:** Destructive actions (Delete Wallet, Clear All Data) have confirmation dialogs.

---

## 4. Robust

### 4.1 Compatible

#### 4.1.1 Parsing (Level A) - 🟢 PASSES

**Note:** This criterion was removed in WCAG 2.2 as browsers handle parsing errors gracefully.

---

#### 4.1.2 Name, Role, Value (Level A) - ⚠️ MAJOR ISSUE

**Finding:** Custom interactive components may lack proper ARIA roles.

**Locations:**

-   Toggle switches (checkboxes styled as switches)
-   Layout toggle buttons (should be radio group)
-   Status filter buttons (should be toggle buttons or radio group)

**Evidence:**

```tsx
// Layout toggle acts as exclusive selection but lacks proper role
<div className="layout-toggle">
    <button type="button" className={layout === "grid" ? "active" : ""}>
```

**Remediation:**

```tsx
<div className="layout-toggle" role="radiogroup" aria-label="View layout">
    <button type="button" role="radio" aria-checked={layout === "grid"} onClick={() => setLayout("grid")}>
        <Grid3X3 size={18} aria-hidden="true" />
        <span className="sr-only">Grid view</span>
    </button>
</div>
```

---

#### 4.1.3 Status Messages (Level AA) - ⚠️ MINOR ISSUE

**Finding:** Status updates (sync progress, migration progress) may not be announced to screen readers.

**Locations:**

-   Sync progress banner
-   Migration progress
-   Success/error messages

**Remediation:**

```tsx
// Add live region for status updates
<div className="sync-progress-banner" role="status" aria-live="polite" aria-atomic="true">
    {/* Progress content */}
</div>
```

---

## WCAG 2.2 New Criteria

### 2.4.11 Focus Not Obscured (Minimum) - 🟢 PASSES

Covered above.

### 2.4.12 Focus Not Obscured (Enhanced) (Level AAA) - N/A

Level AAA, not required for AA compliance.

### 2.4.13 Focus Appearance (Level AAA) - N/A

Level AAA, not required for AA compliance.

### 2.5.7 Dragging Movements - 🟢 PASSES

Covered above.

### 2.5.8 Target Size (Minimum) - ⚠️ MINOR

Covered above.

### 3.2.6 Consistent Help (Level A) - 🟢 PASSES

Help/About is consistently available in sidebar.

### 3.3.7 Redundant Entry (Level A) - 🟢 PASSES

Application does not require redundant data entry.

### 3.3.8 Accessible Authentication (Minimum) (Level AA) - 🟢 PASSES

API token authentication doesn't require cognitive function tests. Token can be pasted.

### 3.3.9 Accessible Authentication (Enhanced) (Level AAA) - N/A

Level AAA, not required for AA compliance.

---

## Prioritized Remediation Plan

### Phase 1: Critical Issues (Immediate)

1. **Add visible focus indicators** (2.4.7)

    - Remove all `outline: none` declarations
    - Add consistent focus styles using CSS custom properties

2. **Add form labels** (1.3.1, 3.3.2)

    - Add `aria-label` or visible labels to all form inputs
    - Associate labels with inputs using `htmlFor`

3. **Enable keyboard modal dismissal** (2.1.1)

    - Add Escape key handler to all modals
    - Set initial focus on modal open

4. **Add ARIA attributes to icons** (1.1.1)
    - Add `aria-hidden="true"` to decorative icons
    - Add `aria-label` to icon-only buttons

### Phase 2: Major Issues (Within 2 weeks)

5. **Improve color contrast** (1.4.3)

    - Audit and adjust `--text-muted` color
    - Verify all text meets 4.5:1 ratio

6. **Add proper ARIA roles** (4.1.2)

    - Add `role="group"` to button groups
    - Add `aria-pressed` to toggle buttons
    - Use `role="radiogroup"` for exclusive selections

7. **Add live regions for status updates** (4.1.3, 3.3.1)

    - Add `role="alert"` to error messages
    - Add `role="status"` to progress indicators

8. **Improve focus order in modals** (2.4.3)
    - Programmatically focus first interactive element
    - Trap focus within modal

### Phase 3: Minor Enhancements (Within 1 month)

9. **Add skip navigation link** (2.4.1)
10. **Fix heading hierarchy** (2.4.6)
11. **Increase target sizes** (2.5.8)
12. **Add error suggestions** (3.3.3)

---

## Automated Testing Recommendations

1. **axe-core**: Integrate into CI/CD pipeline
2. **Lighthouse CI**: Run accessibility audits on builds
3. **eslint-plugin-jsx-a11y**: Add to ESLint configuration

### Recommended ESLint Configuration

```json
{
    "extends": ["plugin:jsx-a11y/recommended"],
    "rules": {
        "jsx-a11y/no-noninteractive-element-interactions": "error",
        "jsx-a11y/click-events-have-key-events": "error",
        "jsx-a11y/no-static-element-interactions": "error"
    }
}
```

---

## Manual Testing Recommendations

### Screen Reader Testing

-   **macOS**: VoiceOver (built-in)
-   **Windows**: NVDA (free) or Narrator (built-in)

### Keyboard Navigation Testing

-   Navigate entire application using only Tab, Shift+Tab, Enter, Space, Arrow keys, Escape
-   Verify all interactive elements are reachable
-   Verify focus is always visible

### Color Contrast Testing

-   Use browser DevTools or WebAIM Contrast Checker
-   Test both light and dark themes

---

## Appendix: Quick Reference CSS Fixes

```css
/* Global focus styles - add to base.css */
:focus-visible {
    outline: 2px solid var(--accent-primary);
    outline-offset: 2px;
}

/* Remove all outline: none declarations and replace with */
input:focus,
button:focus,
select:focus,
textarea:focus {
    outline: 2px solid var(--accent-primary);
    outline-offset: 2px;
}

/* Improved muted text contrast */
:root,
[data-theme="dark"] {
    --text-muted: #8b95a5;
}

/* Screen reader only class */
.sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
}

/* Skip link */
.skip-link {
    position: absolute;
    left: -9999px;
    z-index: 9999;
    padding: 8px 16px;
    background: var(--accent-primary);
    color: white;
    text-decoration: none;
    border-radius: var(--radius-md);
}

.skip-link:focus {
    left: 16px;
    top: 8px;
}

/* Minimum target size */
button,
[role="button"],
input[type="checkbox"],
input[type="radio"] {
    min-width: 24px;
    min-height: 24px;
}
```

---

## Conclusion

Porcupin demonstrates foundational accessibility support but requires targeted improvements to achieve WCAG 2.2 AA compliance. The most critical issues are:

1. Missing visible focus indicators
2. Unlabeled form controls
3. Incomplete keyboard navigation support
4. Missing ARIA attributes for custom components

Implementing the Phase 1 remediation items will significantly improve accessibility for assistive technology users. The application's use of semantic HTML elements and proper button types provides a solid foundation for these improvements.

---

_Report generated by accessibility audit. For questions, contact the development team._
