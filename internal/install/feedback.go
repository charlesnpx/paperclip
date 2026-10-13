package install

const feedbackSkill = `---
name: skill-feedback
description: Record and review concrete feedback from using skills, including ergonomics, instructions, APIs, bugs, improvements, and workarounds, using Paperclip's local feedback ledger and include/exclude configuration.
---

# Skill Feedback

After using an available skill, record concrete feedback worth acting on. A successful task may still expose awkward steps, confusing flags, missing output, avoidable retries, or useful improvements. Do not manufacture criticism or require a report for every invocation. Record one observation per distinct issue, after the relevant step or at task completion, without interrupting the user's work.

Use the exact registered skill name, including its namespace. Include its source path or URL and version when already known; do not delay the task to discover them. The CLI does not verify registry membership. Distinguish what you observed from an inferred cause or proposed improvement. State any workaround actually attempted and whether it worked. Describe API or instruction changes concretely enough for a maintainer to act on them.

Run paperclip feedback check --skill NAME to check eligibility when useful. The CLI always enforces includes/excludes on capture. Configuration lives at ${PAPERCLIP_FEEDBACK_HOME:-~/paperclip/skill-feedback}/config.json; use paperclip feedback --config PATH COMMAND to override it. Missing or empty includes allows all skills, missing or empty excludes excludes none, and exclusions win. Names are exact and case-sensitive; no installed registry or mise-en-place dependency is required. A skipped result is intentional, not a failure. Do not change configuration to bypass exclusions.

Run paperclip feedback add --skill NAME --note TEXT. The note is free-form: explain what happened and why it matters. Optional fields: --category, --workaround, --suggestion, --source, --version, --severity, --idempotency-key. Category is free-form (for example ergonomics, api, bug, instructions, improvement). Severity defaults to medium; use low or info for minor friction or ideas. Do not fill in an artificial failure or expected/observed/impact form.

Example:

    paperclip feedback add --skill figma-fetch --note 'Export succeeded, but each frame needed a second lookup to obtain its export ID.' --category api --workaround 'Looked up each selected frame; export then succeeded' --suggestion 'Include export IDs in frame listing' --severity low

For multiline text, use paperclip feedback add --input-json - with a JSON object containing skill and note. Field names match flags, using underscores instead of hyphens. Do not combine JSON input with capture flags. Omit secrets and unnecessary private content: this is a local plaintext ledger with no automatic redaction.

Attach available conversation references with --harness, --session-id, --session-path, and --session-url (or harness, session_id, session_path, session_url in JSON). PAPERCLIP_HARNESS and PAPERCLIP_SESSION_ID/PATH/URL provide environment defaults; explicit values override them. Supply only references known to belong to this conversation. Do not guess from the newest transcript or read transcript contents to populate a report. Missing references are fine; recording does not depend on finding them.

Review with paperclip feedback review or list. Both include all repositories by default. Add --skill NAME to select a skill, --repo current for the current project, or --json for structured output. To close an obsolete or duplicate observation, use paperclip feedback dispose ID --reason TEXT. Use claim-fixed and verify-fixed only when the corresponding work or verification actually occurred.

Feedback lives at ${PAPERCLIP_FEEDBACK_HOME:-~/paperclip/skill-feedback}/PAPERCLIP.md, independently of PAPERCLIP_HOME. Review ignores capture configuration so earlier feedback remains visible. Do not edit ledger event blocks manually. Use command output as the source of truth for IDs and write success. Conversation references are retained when an identical observation is reported from another session.

Keep skill-related feedback here instead of duplicating it in the general Paperclip ledger. Other operational friction belongs in the paperclip skill. Recording feedback does not authorize modifying the skill, publishing issues, or contacting maintainers. If recording fails, mention it briefly and continue the original task; do not recursively report feedback-tool failures through itself.
`
