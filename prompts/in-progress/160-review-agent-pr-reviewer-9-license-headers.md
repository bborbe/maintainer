---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T21:00:21Z"
---

<summary>
- All 11 files in `agent/pr-reviewer/mocks/` are missing the standard BSD copyright header
- These are Counterfeiter-generated mock files that should have license headers like all other source files
- Fix: run `make addlicense` to add headers to all mock files
</summary>

<objective>
Add the standard BSD copyright header to all mock files in `agent/pr-reviewer/mocks/` using the project's `make addlicense` target.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- One or two existing mock files in `agent/pr-reviewer/mocks/` to confirm they lack headers
- `agent/pr-reviewer/Makefile` — check if `addlicense` target exists
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Run `make addlicense` from the agent/pr-reviewer directory**

   ```bash
   cd agent/pr-reviewer && make addlicense
   ```

   This should add the standard BSD copyright header to all files in `mocks/` that lack it.

2. **Verify the headers were added**

   ```bash
   head -5 agent/pr-reviewer/mocks/bitbucket-client.go
   head -5 agent/pr-reviewer/mocks/claude-runner.go
   head -5 agent/pr-reviewer/mocks/pr-poster.go
   ```

   Expected: Each should begin with:
   ```
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.
   ```

3. **Run `make precommit`** for final validation:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only add license headers to files in `agent/pr-reviewer/mocks/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
