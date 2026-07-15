# Secret Handling

`papercut` is a local personal ledger. It does not provide runtime secret
detection, redaction, encryption, or leak prevention in the current version.
Values passed to `papercut add` are stored as user-provided operational notes in
the local ledger at:

```text
${PAPERCUT_HOME:-~/papercut}/PAPERCUTS.md
```

The ledger is created with private file permissions where the platform supports
them, and repository context is sanitized before it is persisted. That is not a
secret-management boundary. Do not paste passwords, tokens, private keys,
session cookies, signed URLs, customer secrets, or other sensitive material into
papercut. Use it at your own risk.

For sensitive reports, prefer `--input-json -` over shell flags, and provide the
JSON through stdin after `papercut` starts or through a protected input file. Do
not embed the report body in the shell command, because that can still enter
shell history. This only avoids one local exposure path; it does not make the
ledger safe for secrets.

## Why Runtime Secret Detection Was Removed

The earlier custom scanner tried to block private key blocks, credential-bearing
URLs, token assignments, and provider-shaped tokens before writing events. That
created a maintenance burden and could give false confidence while still missing
many real secrets. For a local-only V1 tool, the simpler contract is clearer:
papercut stores what the user gives it, and the user should not give it secrets.

Malformed-ledger diagnostics still avoid echoing payload content, and Git remote
context is still reduced to sanitized repository IDs. Those protections are
about diagnostics and repo identity, not general secret detection.

## Future Option: Gitleaks Plus Obvious Guards

If papercut later needs stricter secret handling, the preferred direction is to
use Gitleaks for real secret detection rather than rebuilding a broad scanner in
this repository. Gitleaks maintains a purpose-built rule set and finding model
for common credential patterns, and it can be evaluated as a CLI integration or
library dependency when that requirement is worth the extra dependency and false
positive handling.

If a lightweight guard is added alongside Gitleaks, keep it intentionally small:

- private key block markers
- URL userinfo such as `https://user:password@example.com`
- common provider token regexes

Document any such guard as best-effort only: do not paste secrets.
