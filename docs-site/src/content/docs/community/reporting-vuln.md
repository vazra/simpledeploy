---
title: Reporting a vulnerability
description: How to disclose security issues in SimpleDeploy responsibly.
---

If you find a security issue, please disclose it privately so we can ship a fix before details become public.

## Report

Email **security@vazra.example** with:

- A description of the issue.
- Affected versions.
- Steps to reproduce or a proof-of-concept.
- Impact in your assessment.

Encrypt with our PGP key if your report contains sensitive details (key ID and fingerprint listed in [SECURITY.md](https://github.com/vazra/simpledeploy/blob/main/SECURITY.md)).

## Response

- We acknowledge within **48 hours**.
- We aim to provide a remediation plan within **7 days** for critical issues.
- We coordinate disclosure timelines with you. Default window is **30 days** from confirmation; longer for complex fixes, shorter if the issue is being actively exploited.

## What we promise

- We will not take legal action for good-faith research that follows this policy.
- We credit reporters in the release notes (unless you prefer to remain anonymous).
- We update [the security audit page](/operations/security-audit/) once the fix is released.

## Out of scope

- Issues in dependencies that have published advisories already.
- Theoretical attacks without a working POC.
- Self-XSS that requires the operator to paste code into their own console.
- Volumetric DoS against a public-internet endpoint.

## Hall of fame

Reporters who follow this policy and report a confirmed issue are listed in `SECURITY.md` and the release notes for the patched version. Thank you for helping keep SimpleDeploy safe.
