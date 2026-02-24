# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in ForgeVM, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, email: **security@forgevm.dev** (or open a private security advisory on GitHub).

We will acknowledge receipt within 48 hours and aim to release a fix within 7 days for critical issues.

## Security Model

ForgeVM provides hardware-level isolation via Firecracker microVMs:

- Each sandbox runs in its own KVM virtual machine with a dedicated kernel
- No shared kernel between sandboxes or between sandbox and host
- Per-VM rootfs — destroyed on teardown
- Host-guest communication via virtio-vsock (no network exposure)
- Optional API key authentication on all endpoints
- TTL-based auto-expiry prevents resource leaks

## Scope

The following are in scope for security reports:

- VM escape vulnerabilities
- API authentication bypass
- Unauthorized access to host filesystem or network
- Denial of service via resource exhaustion
- Command injection through the API

## Out of Scope

- The mock provider intentionally runs commands on the host (development only)
- Self-hosted deployments without API key auth enabled
