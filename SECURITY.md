# Security policy

## Supported versions

Until v1.0, only the latest tagged v0.x release receives security fixes.
Older versions are not maintained. The wine example on `main` is not a
tagged release.

## Report a vulnerability

If the repository's Security tab offers private vulnerability reporting,
use [GitHub's private report form](https://github.com/tolaniverse/opentui-go/security/advisories/new).
Otherwise, contact the repository owner privately through their GitHub profile
before disclosing details publicly. Do not put exploit details in a public issue.
Include the affected version, terminal and platform, a minimal reproduction,
and the possible impact. Do not include credentials or private data in a report.

No response-time SLA or security certification is claimed. The project runs
inside your terminal and parses untrusted input; avoid passing sensitive
terminal output or secrets through example applications without reviewing
their behavior.
