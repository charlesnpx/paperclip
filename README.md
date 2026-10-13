# paperclip

`paperclip` is a local CLI for recording, reviewing, and resolving operational
friction encountered by coding agents.

The ledger is local-first and personal:

```text
${PAPERCLIP_HOME:-~/paperclip}/PAPERCLIP.md
```

`PAPERCUT_HOME` is still accepted as a deprecated fallback when
`PAPERCLIP_HOME` is unset.

The ledger is append-only Markdown containing schema-versioned fenced JSON
events. `paperclip` validates the complete ledger before every write, appends new
events through a same-directory temporary file, fsyncs the file, atomically
renames it, and fsyncs the directory.

## Agent Skills

The installer supplies two skills for Claude Code and Codex, plus the
`paperclip` binary:

- `paperclip` records operational friction with `paperclip add`.
- `paperclip:skill-feedback` records feedback about a skill with
  `paperclip feedback add`.

Both skills are write-only: they tell agents to record observations and continue,
and never to list or review the ledger unless the user asks. Their descriptions
are short so agents can select them automatically without global `AGENTS.md` or
`CLAUDE.md` instructions. Each skill loads its full instructions only when used.

Install, upgrade, or remove them with mise-en-place:

```sh
mise-en-place install paperclip
mise-en-place upgrade paperclip
mise-en-place uninstall paperclip
```

The repository's `install-skill.sh` implements the mise-en-place delegated
installer contract (`--plan`, `--install`, `--uninstall`, `--target`, `--json`,
and `--install-root`). Earlier versions installed a `~/.claude/rules/paperclip.md`
rule and asked users to paste instructions into global instruction files; those
instructions can be removed after upgrading.

## Commands

```sh
paperclip add --expected "..." --observed "..." --impact "..." --locus repo
paperclip add --input-json -
paperclip list
paperclip review
paperclip claim-fixed <observation-id>
paperclip verify-fixed <observation-id>
paperclip dispose <observation-id> --reason "not actionable"
```

`add` requires `expected`, `observed`, `impact`, and `locus`. Optional fields are
`severity`, `scope`, `suggestion`, and `idempotency_key`. Valid locus values are
`repo`, `machine`, `harness`, `model`, `service`, and `process`. Valid severities
are `critical`, `high`, `medium`, `low`, and `info`; missing severity defaults to
`medium`.

Each report creates a new observation, so repeated reports of the same problem
remain visible as separate entries. The exception is a report whose fields match an active
observation created less than 10 minutes earlier, which returns that observation
with `duplicate: true`. Reusing an idempotency key with the same request content
returns the original observation.

For sensitive reports, run `paperclip` first and enter JSON through stdin so the
report body does not enter shell history:

```sh
$ paperclip add --input-json -
{
  "expected": "agent can run tests",
  "observed": "test harness exits before compiling",
  "impact": "blocks verification",
  "locus": "harness",
  "scope": "go test",
  "severity": "high",
  "suggestion": "preserve compiler stderr"
}
```

Then press `Ctrl-D` to finish stdin.

`paperclip` does not perform runtime secret detection or redaction. It stores the
text you provide in a local personal ledger, so do not paste secrets. See
[docs/secret-handling.md](docs/secret-handling.md) for the current risk model
and the future Gitleaks direction.

## Filtering

`list` and `review` show active observations for the current repository by
default. Filters:

```sh
paperclip list --repo current
paperclip list --repo repo-0123456789abcdef
paperclip list --repo none
paperclip list --repo all
paperclip review --locus harness --scope "go test" --json
```

Repository IDs are sanitized. Remote URLs are reduced to a non-secret hash of
their credential-stripped, query-free host/path value; raw remotes, usernames,
tokens, hostnames, and working-directory paths are never persisted. Repositories
without a remote use `git-local:no-remote`. Directories outside Git use `none`.

## Skill Feedback

`paperclip feedback` records free-form feedback about skills in a separate ledger:

```text
${PAPERCLIP_FEEDBACK_HOME:-~/paperclip/skill-feedback}/PAPERCLIP.md
```

It is independent of `PAPERCLIP_HOME` and does not require mise-en-place at runtime.
Use the exact exposed skill name, including namespaces. Only a name and note are
required; successful tasks can still produce useful feedback.

```sh
paperclip feedback add --skill relay:steer --note 'The command worked, but required repeating the session ID.'
paperclip feedback add --input-json -
paperclip feedback check --skill relay:steer
paperclip feedback config validate
paperclip feedback list --skill relay:steer --json
paperclip feedback review
paperclip feedback dispose <observation-id> --reason 'obsolete'
```

Optional capture fields: `category` (free-form), `workaround`, `suggestion`, `source`,
`version`, `severity`, and `idempotency_key`. CLI flags use hyphens; JSON keys use
underscores. JSON input cannot be mixed with capture flags. Severity defaults to
`medium`. `claim-fixed` and `verify-fixed` share the general Paperclip lifecycle.
Review includes all repositories by default, groups feedback by skill, and accepts
`--repo current` to narrow the results. List and review show active observations.

### Capture Configuration

The default configuration is `config.json` in the feedback directory.

The ledger requires its directory to be private. Capture creates it with mode
`0700`; use that mode if creating the directory manually before writing config.

To override the configuration path:

```sh
paperclip feedback --config /absolute/path/config.json check --skill relay:steer
```

```json
{
  "includes": [],
  "excludes": ["paperclip:skill-feedback"]
}
```

Missing or empty `includes` allows all skills. Missing or empty `excludes` excludes
none. Exclusions take precedence. Names match exactly and case-sensitively: `relay`
does not match `relay:steer`. No package expansion, registry membership validation,
or harness-specific configuration is performed.

A missing default file allows all skills; an explicit path must exist. Unknown
properties, null lists, incorrect types, and empty or whitespace-containing names
are usage errors. `config validate` prints the effective configuration as JSON;
`check` reports eligibility. Neither touches the ledger. Excluded capture returns
exit code 0 with `skipped: true` and a reason, without ledger changes. Configuration
never hides or removes historical entries, and review does not load it.

### Conversation References

Both `paperclip add` and `paperclip feedback add` accept `--harness`, `--session-id`,
`--session-path`, and `--session-url` (underscore-named fields in JSON). Defaults come
from `PAPERCLIP_HARNESS`, `PAPERCLIP_SESSION_ID`, `PAPERCLIP_SESSION_PATH`, and
`PAPERCLIP_SESSION_URL`. Explicit input overrides defaults, including empty strings.
References are optional and supplied by the agent or its environment; the CLI does
not discover transcripts, read their contents, or check that paths still exist.

Conversation references do not affect deduplication or idempotency digests. A new
reference on a duplicate or idempotent capture is appended to the same observation;
repeating an identical reference adds no event. Text and JSON review include them.

### Compatibility and Installation

Existing v1 events remain readable and unchanged. Native feedback and conversation
associations introduce `paperclip.event.v2` events in the same append-only Markdown
format. Old binaries cannot read ledgers containing those events: upgrade all
writers before capturing conversation references in an existing shared ledger.
There is no migration or automatic rewrite of old fingerprints or request digests.

## Exit Codes

- `0`: success
- `1`: unexpected or I/O failure
- `2`: usage error
- `3`: reserved
- `4`: idempotency conflict or invalid lifecycle transition
- `5`: malformed ledger
- `6`: lock timeout

## Supported Targets

`paperclip` is supported on Darwin and Linux for `amd64` and `arm64`.
