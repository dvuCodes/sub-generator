# Security Policy

SubGen is a local desktop application distributed as source in this repository. Please do not file public issues for suspected security problems.

## Reporting A Vulnerability

- Prefer GitHub private vulnerability reporting for this repository if it is enabled.
- If private reporting is unavailable, contact the maintainer privately through GitHub before opening a public issue.
- Include the affected commit, operating system, reproduction steps, impact, and whether the issue requires local code execution or a crafted media file.

## Scope

Security reports are most helpful when they involve one of these areas:

- bundled or repo-local service execution
- unsafe handling of local files or output paths
- accidental exposure of secrets or credentials
- dependency or supply-chain issues that affect a default source install
- remote code execution, privilege escalation, or data exfiltration paths

## What To Expect

- Good-faith reports will be reviewed and triaged.
- Please avoid publishing proof-of-concept details until a fix or mitigation is available.
