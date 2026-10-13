# Global Instructions

Add the following section to the global `CLAUDE.md` or `AGENTS.md` after installing
the updated Paperclip package. The installer does not modify these shared files.
`paperclip feedback instructions` prints the same instruction body.

## Skill Feedback

After using a skill, use the skill-feedback skill to record useful observations about ergonomics, instructions, APIs, bugs, improvements, or workarounds, subject to its include/exclude configuration. Include the skill name and a concrete note; attach available conversation references. Record feedback even when the task succeeds, but do not invent issues or require a report for every use. Keep feedback local, omit secrets, avoid duplicating it in the general Paperclip ledger, and continue the user's task.

## Paperclip Conversation References

The updated Paperclip skill supplies the detailed capture instructions. If your
global instructions currently describe direct CLI use instead, add:

For Paperclip reports, attach the current harness, session ID, and transcript path or conversation URL when known. Do not guess the current session or copy transcript contents; missing references do not block recording.
