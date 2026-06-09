---
status: completed
summary: Added GithubBuildV1SchemaID to lib/maintainer_cdb-schema.go and matched test block, registered in CDBSchemaIDs, CHANGELOG updated
container: maintainer-cqrs-trigger-build-exec-256-cqrs-github-build-schema-id
dark-factory-version: v0.175.0
created: "2026-06-09T16:00:00Z"
queued: "2026-06-09T16:21:18Z"
started: "2026-06-09T16:21:19Z"
completed: "2026-06-09T16:29:33Z"
branch: dark-factory/cqrs-trigger-github-build
---

# Spec 068 Prompt 1 — Add GithubBuildV1SchemaID to lib

## Context

This is prompt 1 of 5 for spec 068 (CQRS trigger for the github-build watcher). It is the leaf: every later prompt imports `lib.GithubBuildV1SchemaID` to wire the new request topic, sender, executor, and command consumer. Only `lib/maintainer_cdb-schema.go` and its test file change in this prompt.

Mirror spec 067 prompt 1 (`/workspace/prompts/completed/252-trigger-release-check-command-and-sender.md` precedent) for tone and structure. The two deltas vs the spec 067 schema work: (a) the schema ID is for the github-build watcher, not github-release; (b) `lib/maintainer_cdb-schema_test.go` already has Describe blocks for the existing two schema IDs — add a parallel block for the new one.

## Goal

- Add a new exported `GithubBuildV1SchemaID` constant in `lib/maintainer_cdb-schema.go` with `Group:"maintainer"`, `Kind:"githubbuild"`, `Version:"v1"`.
- Append it to the `CDBSchemaIDs` slice in declaration order (after `GithubReleaserV1SchemaID`, before the closing `}`).
- Add the matching `Describe` block in `lib/maintainer_cdb-schema_test.go` plus a registry-contains assertion.

## Files to change

- **Modify:** `/workspace/lib/maintainer_cdb-schema.go` — append the new constant after `GithubReleaserV1SchemaID` (currently the last var in the file) and extend the `CDBSchemaIDs` slice initializer to include it.
- **Modify:** `/workspace/lib/maintainer_cdb-schema_test.go` — add a `Describe("GithubBuildV1SchemaID", ...)` block mirroring the existing two; add an `It("contains GithubBuildV1SchemaID", ...)` entry under the `CDBSchemaIDs registry` Describe.

## Out of scope

- Do NOT touch `watcher/github-build/` at all in this prompt. Senders/executors/factories land in prompts 2-5.
- Do NOT register the schema in `trading/strimzi/topic-controller/topics.go` — that is a sibling PR in another repo (spec § Constraints).
- Do NOT change the group/kind/version values. They are FROZEN — the topic-controller side PR targets this exact identifier (`"maintainer-githubbuild-v1"`).
- Do NOT rename any existing schema ID. Order in `CDBSchemaIDs` stays: PRReview, Releaser, Build.

## Implementation

1. Read the current `lib/maintainer_cdb-schema.go` (lines 1-33) and `lib/maintainer_cdb-schema_test.go` (lines 1-57) fully. Mirror the existing style — Group, Kind, Version on separate lines; comment above the `var` declaration in the same shape as the two existing ones.

2. Append the new constant after `GithubReleaserV1SchemaID`:

   ```go
   // GithubBuildV1SchemaID is the schema for the github-build watcher's
   // command topic. Carries Trigger-style commands consumed by the watcher
   // pod to drive the build-fixer pipeline.
   var GithubBuildV1SchemaID = cdb.SchemaID{
       Group:   "maintainer",
       Kind:    "githubbuild",
       Version: "v1",
   }
   ```

3. Extend the `CDBSchemaIDs` slice initializer (currently lines 12-15) to include the new ID:

   ```go
   var CDBSchemaIDs = cdb.SchemaIDs{
       GithubPRReviewV1SchemaID,
       GithubReleaserV1SchemaID,
       GithubBuildV1SchemaID,
   }
   ```

4. In `lib/maintainer_cdb-schema_test.go`, add a new `Describe` block after the existing `Describe("GithubReleaserV1SchemaID", ...)` block (line 38). Mirror the existing two blocks line-for-line, only the ID name and the expected string change:

   ```go
   Describe("GithubBuildV1SchemaID", func() {
       It("has expected group/kind/version", func() {
           Expect(lib.GithubBuildV1SchemaID.Group).To(Equal(cdb.Group("maintainer")))
           Expect(lib.GithubBuildV1SchemaID.Kind).To(Equal(cdb.Kind("githubbuild")))
           Expect(lib.GithubBuildV1SchemaID.Version).To(Equal(cdb.Version("v1")))
       })

       It("serializes to canonical string", func() {
           Expect(lib.GithubBuildV1SchemaID.String()).To(Equal("maintainer-githubbuild-v1"))
       })
   })
   ```

5. Add one entry to the `CDBSchemaIDs registry` Describe (currently lines 40-56), after the existing `It("contains GithubReleaserV1SchemaID", ...)`:

   ```go
   It("contains GithubBuildV1SchemaID", func() {
       Expect(lib.CDBSchemaIDs.Contains(lib.GithubBuildV1SchemaID)).To(BeTrue())
   })
   ```

   The `It("has no duplicate entries", ...)` block already covers uniqueness; no edit needed there.

6. Verify `cdb.SchemaIDs.Contains` is a real method (grep `func (s SchemaIDs) Contains` against `/home/node/go/pkg/mod/github.com/bborbe/cqrs@v0.5.3/cdb/`). If absent, fall back to a linear search using `for _, id := range lib.CDBSchemaIDs { if id == lib.GithubBuildV1SchemaID { return } }`. The existing test file already calls `.Contains(...)` on `CDBSchemaIDs` (line 42) so the method MUST exist — this is just a sanity grep before relying on the test.

## Tests

No new test file. Just extend the existing `lib/maintainer_cdb-schema_test.go` as described above. The test runner picks up the new `Describe` block via the existing Ginkgo suite (no suite_test.go exists in `lib/` — confirm; if absent, the test runs as a plain Go test).

## Verification

```
cd /workspace/lib && make precommit
echo "exit=$?"
grep -n 'GithubBuildV1SchemaID' /workspace/lib/maintainer_cdb-schema.go
```

Both must succeed: precommit exit 0; grep returns ≥ 2 lines (the `var` declaration and the slice entry).

## Lessons from spec 067 audit (apply at write time)

1. The schema ID is a `cdb.SchemaID` value (struct, not a string). Use the struct-literal form `cdb.SchemaID{Group:..., Kind:..., Version:...}` — do NOT declare it as a `const`. Mirrors the two existing siblings.
2. Order in `CDBSchemaIDs` matters for human readers and for the topic-controller PR — append `GithubBuildV1SchemaID` at the END of the slice initializer (after the Releaser entry), NOT in the middle. The spec's "after `GithubReleaserV1SchemaID`" wording is deliberate.
3. The test file already has the `It("has no duplicate entries", ...)` block. You do NOT need to add a new duplicate-check; that block iterates `lib.CDBSchemaIDs` and asserts uniqueness across all entries automatically.
4. BSD header on the file stays. No new file here — only edits to two existing files. Do NOT add a new file.
5. Do NOT change the import block of either file. `cdb` is already imported in `maintainer_cdb-schema.go`; `lib` and `cdb` are already imported in the test.

## Improvements

(empty — YOLO fills in after running)
