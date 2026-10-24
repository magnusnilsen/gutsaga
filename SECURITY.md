# Security policy

## Reporting a vulnerability

Report security vulnerabilities privately through [GitHub Security Advisories](https://github.com/magnusnilsen/gutsaga/security/advisories/new).

Do not open a public GitHub issue, discussion, or pull request for a suspected vulnerability. A public report can expose players before there is a fix.

Describe the impact and how to reproduce it. Leave out session tokens, API keys, webhook URLs, and other live secrets. If you are unsure whether something is a vulnerability, use the advisory form anyway.

Please give the maintainer time to investigate and ship a fix before any public disclosure.

## Supported versions

Security fixes land on `main` and in the companion releases cut from that branch. Older companion builds are not maintained as a separate line.

## What belongs here

The API (`api/`), the companion (`cmd/gutsaga`), the addon (`addon/`), and the Smia tool definitions (`smia/wow-mcp`) are in scope. World of Warcraft itself is not.
