import React from "react";
import { createRoot } from "react-dom/client";
import "./App.css";
import App from "./App";

// Wait for Wails runtime before rendering React
// In production builds, there's a race condition where React can initialize
// before Wails injects window.go
function waitForWails(timeout = 10000): Promise<boolean> {
    return new Promise((resolve) => {
        // @ts-expect-error Wails runtime is injected globally
        if (typeof window !== "undefined" && typeof window.go !== "undefined") {
            resolve(true);
            return;
        }

        const startTime = Date.now();
        const checkInterval = setInterval(() => {
            // @ts-expect-error Wails runtime is injected globally
            if (typeof window !== "undefined" && typeof window.go !== "undefined") {
                clearInterval(checkInterval);
                resolve(true);
            } else if (Date.now() - startTime > timeout) {
                clearInterval(checkInterval);
                console.error("[Porcupin] Timeout waiting for Wails runtime");
                resolve(false);
            }
        }, 50);
    });
}

async function init() {
    const wailsReady = await waitForWails();
    if (!wailsReady) {
        document.body.innerHTML =
            '<div style="padding: 20px; color: red;">Failed to initialize Wails runtime. Please restart the app.</div>';
        return;
    }

    const container = document.getElementById("root");
    if (!container) {
        document.body.innerHTML = '<div style="padding: 20px; color: red;">Root element not found.</div>';
        return;
    }
    const root = createRoot(container);

    root.render(
        <React.StrictMode>
            <App />
        </React.StrictMode>
    );
}

init();
