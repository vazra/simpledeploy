---
title: "Privacy Policy"
description: "How the SimpleDeploy project handles data on its website and in the software itself."
---

_Last updated: 2026-04-26_

SimpleDeploy is an open source, self-hosted application. This policy covers two surfaces:

1. **The SimpleDeploy software** that you install and run on your own server.
2. **The SimpleDeploy website and documentation** at `vazra.github.io/simpledeploy`.

## 1. The software (self-hosted)

The SimpleDeploy binary runs on infrastructure you control. The project maintainers do **not** receive any data from your installation.

- **No telemetry.** SimpleDeploy does not phone home, send usage metrics, or report crashes to the maintainers.
- **No accounts on our side.** User accounts, API keys, and credentials live only in your local SQLite database under your `data_dir`.
- **Outbound connections** initiated by SimpleDeploy are limited to: container registries you configure, Let's Encrypt (for ACME TLS, if enabled), webhook URLs you configure, S3-compatible endpoints you configure for backups, and `git remote` endpoints you configure for GitSync. None of these are operated by the SimpleDeploy maintainers.

You, as the operator, are the data controller for any personal data processed by your SimpleDeploy instance. You are responsible for your own privacy disclosures to your users.

## 2. The website and documentation

The documentation site is hosted on **GitHub Pages**. We do not run our own analytics on it.

- **Server logs.** GitHub may collect IP addresses and request metadata for the documentation site under [GitHub's Privacy Statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).
- **No cookies set by us.** The site does not set tracking cookies. Browser local storage may be used for theme preference (light/dark) only.
- **External resources.** Pages may embed images, icons, or fonts from third parties as documented; visiting those resources is subject to their own policies.
- **Issues, PRs, discussions.** When you participate on the GitHub repository, GitHub's policies apply.

## Children

SimpleDeploy is a developer tool not directed at children under 13. We do not knowingly collect data from children.

## Changes

We may update this policy. Material changes will be reflected by updating the "Last updated" date above and noted in the changelog where relevant.

## Contact

Questions: open an issue at [github.com/vazra/simpledeploy](https://github.com/vazra/simpledeploy/issues), or for security-sensitive matters see [SECURITY.md](https://github.com/vazra/simpledeploy/blob/main/SECURITY.md).
