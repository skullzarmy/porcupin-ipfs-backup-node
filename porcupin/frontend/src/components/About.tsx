import { useState, useEffect } from "react";
import { Github, AlertTriangle, Mail, ExternalLink, Heart, Globe } from "lucide-react";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { GetVersion, isRemote } from "../lib/backend";
import * as WailsApp from "../../wailsjs/go/main/App";
import { Logo } from "./Logo";

export function About() {
    const [version, setVersion] = useState("");
    const [localVersion, setLocalVersion] = useState("");

    useEffect(() => {
        // Always get the backend version (remote if connected, local otherwise)
        GetVersion().then(setVersion);
        // If in remote mode, also get local app version
        if (isRemote()) {
            WailsApp.GetVersion().then(setLocalVersion);
        }
    }, []);

    return (
        <div className="about-page">
            <div className="about-hero">
                <Logo size={96} className="about-logo" />
                <div className="about-info">
                    <h1>Porcupin</h1>
                    {isRemote() ? (
                        <div className="version-info">
                            <p className="version">App: {localVersion}</p>
                            <p className="version">Server: {version}</p>
                        </div>
                    ) : (
                        <p className="version">Version {version}</p>
                    )}
                </div>
            </div>

            <p className="about-tagline">Tezos NFT Backup to IPFS</p>

            <p className="about-description">
                Automatically backup your Tezos NFT assets to a local IPFS node. Set it and forget it – Porcupin watches
                your wallets and pins new NFTs as they arrive.
            </p>

            <div className="about-links">
                <button type="button" className="about-link" onClick={() => BrowserOpenURL("https://porcupin.xyz")}>
                    <Globe size={18} />
                    <span className="link-text">
                        <span className="link-title">porcupin.xyz</span>
                        <span className="link-subtitle">Homepage, docs, and latest releases</span>
                    </span>
                    <ExternalLink size={14} />
                </button>
                <button
                    type="button"
                    className="about-link"
                    onClick={() => BrowserOpenURL("https://github.com/skullzarmy/porcupin-ipfs-bakup-node")}
                >
                    <Github size={18} />
                    <span className="link-text">
                        <span className="link-title">GitHub Repository</span>
                        <span className="link-subtitle">View source code and contribute</span>
                    </span>
                    <ExternalLink size={14} />
                </button>
                <button
                    type="button"
                    className="about-link"
                    onClick={() => BrowserOpenURL("https://github.com/skullzarmy/porcupin-ipfs-bakup-node/issues")}
                >
                    <AlertTriangle size={18} />
                    <span className="link-text">
                        <span className="link-title">Report an Issue</span>
                        <span className="link-subtitle">Found a bug? Let us know</span>
                    </span>
                    <ExternalLink size={14} />
                </button>
                <button type="button" className="about-link" onClick={() => BrowserOpenURL("mailto:info@fafolab.xyz")}>
                    <Mail size={18} />
                    <span className="link-text">
                        <span className="link-title">Contact Support</span>
                        <span className="link-subtitle">Get help with Porcupin</span>
                    </span>
                    <ExternalLink size={14} />
                </button>
            </div>

            <div className="about-credits">
                <p>
                    <Heart size={14} className="heart-icon" />
                    Developed by{" "}
                    <button type="button" className="credit-link" onClick={() => BrowserOpenURL("https://fafolab.xyz")}>
                        <strong>
                            FAFO<s>lab</s>
                        </strong>
                    </button>
                </p>
                <p className="copyright">© 2025 Porcupin. MIT License.</p>
            </div>
        </div>
    );
}
