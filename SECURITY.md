# Security Policy

## Supported versions

Security fixes are applied to the latest commit on `master` / `main` and to the most recent release tag.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.

Report privately instead:

1. Use [GitHub Security Advisories](https://github.com/mewisme/discloud-go/security/advisories/new) if available, or
2. Email the maintainer via the contact method listed on [github.com/mewisme](https://github.com/mewisme).

Include steps to reproduce, impact, and any suggested fix. You should receive an acknowledgement within a few days.

## Scope notes

DisCloud stores file chunks as Discord attachments. Keep `DISCORD_BOT_TOKEN` and database credentials out of commits and public logs. Rotate any token that may have been exposed.
