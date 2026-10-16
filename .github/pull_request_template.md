<!-- markdownlint-disable MD041 -->
## What and why

<!-- What does this change, and why? Link the issue it closes: "Closes #123". -->

## Checklist

- [ ] The PR title is a [Conventional Commit](https://www.conventionalcommits.org/) (it becomes the squash commit and drives the release)
- [ ] `mise run check` passes
- [ ] Tests added or updated (a bug fix has a test that fails without it)
- [ ] Any text coming from Vault is passed through `internal/tui/safe` before display
- [ ] No tokens, personal paths or real hostnames in code, tests or docs
