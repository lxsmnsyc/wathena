# Security Policy

## Reporting a vulnerability

Do not open a public issue for security problems.

Report vulnerabilities privately through GitHub's private vulnerability reporting on this repository, under the Security tab.

Include:

- What the problem is and what an attacker can do with it.
- Steps to reproduce it.
- The version or commit you tested.

## Response

- A first response within 7 days.
- A fix or a plan within 30 days for confirmed high or critical problems.
- Credit in the release notes, unless you ask not to be named.

## Supported versions

| Version | Supported |
|---|---|
| v0.1.x | Yes. Security fixes come as patch releases, such as v0.1.1. |
| `main` | Yes. Fixes land here first. Run a release on a live server. |
| Any other commit or fork | No. |

- A release is a git tag. The project publishes no binaries or images, so build a fix from its tag (docs/guides/setup.md, "Update to a new release").
- This table is updated with each release.
- Known issues of v0.1.0 are in its [release notes](docs/releases/v0.1.0.md) and in [docs/security/review-2026-10.md](docs/security/review-2026-10.md).
