---
title: Reporting a bug
description: What to include in a bug report so it can be triaged quickly.
---

Open an issue at [github.com/vazra/simpledeploy/issues](https://github.com/vazra/simpledeploy/issues). For security issues, see [Reporting a vulnerability](/community/reporting-vuln/) instead.

## Before opening

1. Search existing issues. Comment on one that matches rather than opening a duplicate.
2. Try the latest released version. The bug may be fixed.
3. Strip the report of secrets (`master_secret`, API keys, registry passwords, customer data).

## Include in every report

- **Version**: output of `simpledeploy version`.
- **OS and arch**: e.g., Ubuntu 24.04 amd64.
- **Docker version**: `docker version`.
- **Install method**: brew, apt, binary, source, etc.
- **What you expected to happen.**
- **What actually happened.** Include the exact error message.
- **Repro steps**: minimum sequence that triggers the bug.
- **Logs**: relevant lines from `journalctl -u simpledeploy` or the dashboard's System logs page. Only the last 100 lines is usually enough.

## Helpful extras

- A minimal `docker-compose.yml` that triggers the bug.
- A screenshot if the bug is in the dashboard.
- Whether you can reproduce on a fresh install.

## Bug vs question

If you are not sure something is broken (vs. confusing), open a [Discussion](https://github.com/vazra/simpledeploy/discussions) first. Maintainers will move it if it turns out to be a bug.

## Triage

Bugs are labeled `bug`, severity `S1`-`S4`, and area (`api`, `cli`, `ui`, `proxy`, `backup`, ...). Maintainers respond within a few days for S1/S2; lower severity may take longer.
