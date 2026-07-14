# paperclip

`paperclip` provides the local `papercut` CLI for recording, reviewing, and
resolving operational friction encountered by coding agents.

The ledger is local-first and personal:

```text
${PAPERCUT_HOME:-~/Papercuts}/PAPERCUTS.md
```

The ledger is append-only Markdown containing schema-versioned fenced JSON
events. `papercut` validates the complete ledger before every write, appends new
events through a same-directory temporary file, fsyncs the file, atomically
renames it, and fsyncs the directory.

## Commands

```sh
papercut add --expected "..." --observed "..." --impact "..." --locus repo
papercut add --input-json -
papercut list
papercut review
papercut claim-fixed <observation-id>
papercut verify-fixed <observation-id>
papercut dispose <observation-id> --reason "not actionable"
papercut instructions --format agents
```

`add` requires `expected`, `observed`, `impact`, and `locus`. Optional fields are
`severity`, `scope`, `suggestion`, and `idempotency_key`. Valid locus values are
`repo`, `machine`, `harness`, `model`, `service`, and `process`. Valid severities
are `critical`, `high`, `medium`, `low`, and `info`; missing severity defaults to
`medium`.

For sensitive reports, prefer stdin JSON so values do not enter shell history:

```sh
printf '%s\n' '{
  "expected": "agent can run tests",
  "observed": "test harness exits before compiling",
  "impact": "blocks verification",
  "locus": "harness",
  "scope": "go test",
  "severity": "high",
  "suggestion": "preserve compiler stderr"
}' | papercut add --input-json -
```

## Filtering

`list` and `review` show active observations for the current repository by
default. Filters:

```sh
papercut list --repo current
papercut list --repo repo-0123456789abcdef
papercut list --repo none
papercut list --repo all
papercut review --locus harness --scope "go test" --json
```

Repository IDs are sanitized. Remote URLs are reduced to a non-secret hash of
their credential-stripped, query-free host/path value; raw remotes, usernames,
tokens, hostnames, and working-directory paths are never persisted. Repositories
without a remote use `git-local:no-remote`. Directories outside Git use `none`.

## Exit Codes

- `0`: success
- `1`: unexpected or I/O failure
- `2`: usage error
- `3`: policy denied
- `4`: idempotency conflict or invalid lifecycle transition
- `5`: malformed ledger
- `6`: lock timeout

## Supported Targets

`papercut` is supported on Darwin and Linux for `amd64` and `arm64`.
