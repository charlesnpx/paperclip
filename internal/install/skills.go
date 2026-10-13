package install

const paperclipSkill = `---
name: "paperclip"
description: "Record operational friction (repo, machine, harness, tool, model, service, or process problems; broken links; repeated blockers) with paperclip add, then continue. Write-only unless the user asks to review. Skill-specific feedback: paperclip:skill-feedback."
---

# Paperclip

Record friction when it happens, then continue the task. Agents only write to Paperclip; do not list, review, dispose, or mark observations fixed unless the user explicitly asks.

    paperclip add --expected TEXT --observed TEXT --impact TEXT --locus LOCUS [--severity LEVEL] [--scope TEXT] [--suggestion TEXT]

- ` + "`--locus`" + `: repo, machine, harness, model, service, or process.
- ` + "`--severity`" + `: critical, high, medium (default), low, or info.
- ` + "`--scope`" + `: the affected command, tool, or area.
- For multiline or sensitive text, pipe a JSON object with the same fields (underscores instead of hyphens) to ` + "`paperclip add --input-json -`" + `. Do not combine it with capture flags.
- Attach known conversation references with ` + "`--harness`" + `, ` + "`--session-id`" + `, ` + "`--session-path`" + `, and ` + "`--session-url`" + ` (defaults: ` + "`PAPERCLIP_HARNESS`" + `, ` + "`PAPERCLIP_SESSION_ID`" + `/` + "`PATH`" + `/` + "`URL`" + `). Do not guess them or copy transcript contents.

Report a problem each time it occurs; repeats are useful signal. An identical report within 10 minutes is dropped as a retry. Never include secrets: the ledger is local plaintext without redaction. If recording fails, mention it briefly and continue.
`

const feedbackSkill = `---
name: "paperclip:skill-feedback"
description: "After using a skill, record actionable feedback on it (friction, unclear instructions, bugs, workarounds, improvements) with paperclip feedback add, even if the task succeeded. Do not invent issues. Write-only unless the user asks to review."
---

# Paperclip Skill Feedback

After using a skill, record feedback worth acting on: awkward steps, unclear or wrong instructions, confusing flags or output, bugs, avoidable retries, workarounds, or improvements. A successful task can still produce feedback. Do not invent issues or report every use. Record one entry per distinct issue, then continue the task. Do not list or review feedback unless the user explicitly asks.

    paperclip feedback add --skill NAME --note TEXT [--category TEXT] [--workaround TEXT] [--suggestion TEXT] [--source PATH_OR_URL] [--version TEXT] [--severity LEVEL]

- ` + "`--skill`" + `: the exact invoked name, including its namespace (` + "`relay:steer`" + `, not ` + "`relay`" + `).
- ` + "`--note`" + `: what happened and why it matters. Separate what you observed from an inferred cause.
- ` + "`--workaround`" + `: what you tried and whether it worked.
- ` + "`--severity`" + `: critical, high, medium (default), low, or info. Use low or info for minor friction.
- For multiline text, pipe a JSON object (` + "`skill`" + `, ` + "`note`" + `, and optional fields with underscores) to ` + "`paperclip feedback add --input-json -`" + `. Do not combine it with capture flags.
- Attach known conversation references with ` + "`--harness`" + `, ` + "`--session-id`" + `, ` + "`--session-path`" + `, and ` + "`--session-url`" + `. Do not guess them or copy transcript contents.

A result with ` + "`skipped: true`" + ` means the user's feedback configuration excludes that skill; do not change the configuration. Repeats are useful signal; an identical report within 10 minutes is dropped as a retry. Never include secrets. Other operational friction belongs in the paperclip skill. Recording feedback does not authorize editing the skill or contacting its maintainers. If recording fails, mention it briefly and continue; do not report that failure through this skill.
`
