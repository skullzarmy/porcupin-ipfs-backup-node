# Changelog

All notable changes to Porcupin are recorded here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.5] — 2026-10-22

### Security

Full CVE audit and dependency maintenance across the stack. Patches 55
reachable advisories reported by `govulncheck` and `bun audit`.

- **Go runtime** moves to Go 1.26.10, which resolves roughly 30 standard-library
  advisories across HTTP, TLS, X.509, HTML templating, URL parsing, archive
  handling, and ASN.1/XML decoding.
- **IPFS stack** moves to kubo v0.43.1, boxo v0.43.0, and go-libp2p v0.50.0,
  bringing in current libp2p, quic-go, webtransport-go, gRPC, and
  OpenTelemetry patches.
- **Web frontend** moves to Vite 8.3 and `@vitejs/plugin-react` 6.1, which
  pulls in patched PostCSS, nanoid, source-map-js, and launch-editor and
  clears all 10 advisories flagged by audit.
- Two residual advisories remain, both from upstream projects with no fix
  available yet: `golang.org/x/crypto/openpgp` package deprecation
  (GO-2026-5932) and go-libp2p-kad-dht content censorship via DHT abuse
  (GO-2024-3218). Both will be picked up as soon as upstream ships patches.

### Changed

- CI and release workflows pin Go 1.26 to match the module's new minimum
  toolchain.

## [1.0.4] — 2026-08-05

### Added

- Descriptive `User-Agent` on every outbound HTTP request.

### Fixed

- IPNI-only provider discovery via delegated routing, now configurable.

### Changed

- `golang.org/x/net` 0.52.0 → 0.55.0, `golang.org/x/crypto` 0.51.0 → 0.52.0.

## [1.0.3] — 2026-06-23

### Fixed

- Silent OOM crash and restart failure (BUG-1 and BUG-2).
- CodeQL Go scan repaired by replacing Autobuild with an explicit `go build`.

### Changed

- Dependency bumps: `postcss` 8.5.6 → 8.5.14, `go-ipld-prime` 0.22.0 → 0.23.0,
  `quic-go` 0.59.0 → 0.59.1, `go-chi/chi/v5` 5.2.3 → 5.2.4, esbuild patch.
- Two workflow deprecation notices silenced.

## [1.0.2] — 2026-04-20

### Changed

- Internal release. Full diff:
  https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.1...v1.0.2

## [1.0.1] — 2026-04-19

### Changed

- Internal release. Full diff:
  https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.0...v1.0.1

## [1.0.0] — 2026-04-18

### Added

- Initial public release of Porcupin, a Wails desktop app that embeds a Kubo
  IPFS node and backs up Tezos NFT assets for configured wallets.
- Settings CLI and update mechanism.
- CodeQL workflow for automated code analysis on push and pull requests.
- Deep IPFS URI discovery for non-standard metadata fields.
- Health checks, logging, and diagnostics UX improvements.

### Fixed

- Sync crash and several UX issues reported by Ryan Thompson.
- `TestGetFailedAssets` return-type assertion.
- `FetchRawMetadataURI` refactored to use `GetTokenMetadataBigMap`.

### Changed

- Raspberry Pi Zero installation guide refreshed.
- Dependency bumps: `pion/dtls/v3` 3.0.6 → 3.1.0, `rollup` 4.53.4 → 4.59.0,
  `otel/sdk` 1.38.0 → 1.40.0, `grpc` 1.75.0 → 1.79.3,
  `go-jose/v4` 4.1.3 → 4.1.4, `picomatch` 4.0.3 → 4.0.4, `vite` 6.4.1 → 6.4.2,
  kubo v0.39.0 → v0.40.1 (fixes a quic-go CVE).

[Unreleased]: https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.5...HEAD
[1.0.5]: https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.4...v1.0.5
[1.0.4]: https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.3...v1.0.4
[1.0.3]: https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/skullzarmy/porcupin-ipfs-backup-node/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/skullzarmy/porcupin-ipfs-backup-node/releases/tag/v1.0.0
