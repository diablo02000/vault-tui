# Security policy

vault-tui handles Vault tokens and secret values, so security reports are taken seriously.

## Supported versions

Only the latest release receives security fixes.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through GitHub:

1. Open the repository's **Security** tab.
2. Choose **Report a vulnerability** (private vulnerability reporting).

Include the affected version, steps to reproduce, and the impact you see. You can expect an
acknowledgement within a few days. Once a fix is released, the advisory is published and
you are credited if you wish.

## Scope

In scope:

- leaking a Vault token or secret value (logs, process arguments, screen, network)
- terminal escape-sequence or output injection from data returned by a Vault server
- sending the token to a host other than the configured address
- bypassing the read-only guarantee

Out of scope: issues that require a malicious local user who can already read the
victim's `~/.vault-token`, or running against a Vault server you do not trust with your token.

## Hardening already in place

See the *Security model* section of the [README](README.md#security-model).
