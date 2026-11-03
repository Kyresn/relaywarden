# Security

## Supported versions

Security fixes target the latest tagged release. The `main` branch is development code and is not supported as a deployment target.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting (Security → Advisories → Report a vulnerability) instead of a public issue. Include the version, CLIProxyAPI version, platform, and a minimal reproduction.

Do not include real API keys, management keys, OAuth files, or customer prompts in a report.

## Threat model

RelayWarden is a dynamic library loaded inside the CLIProxyAPI process. It is trusted in-process code: a malicious plugin can read everything the host can read. Install only plugins you trust.

- The management key is the only authentication for the plugin's management API. The embedded UI keeps it in `sessionStorage`; it is never written to disk.
- The UI is served as an unauthenticated resource page containing no runtime data. Every data request requires the Bearer key.
- Webhook URLs are operator-configured. The plugin sends signal JSON to that URL asynchronously. Treat a webhook URL as a data-egress destination.
- Compression and guard paths fail open; they must never turn a request into a failure.
- The plugin does not log request bodies, authorization headers, or management keys.

## Dependency and supply chain

- The plugin depends on `github.com/router-for-me/CLIProxyAPI/v8` for the public plugin SDK and on `tidwall/gjson`/`sjson` and `gopkg.in/yaml.v3`.
- Release artifacts are built by GitHub Actions from a tagged commit. Verify the artifact checksum against the release notes when one is published.
