---
status: approved
approved: 2026-11-27T05:38:59.639045444Z
kind: bug
---

## Summary

Sentry issue `NUKE-PROD-C2` is a real bug in `bborbe/maintainer` at `githubapp/githubapp.go:107`, filed automatically by the sentry-fix-agent from the High/High real bug deep verdict — that verdict is the approval gate, so this spec is approved rather than drafted.

## Problem

githubapp.MintIAT (githubapp.go:107) calls ghinstallation's transport.Token with no retry/backoff, so a transient GitHub 500 on the installation-token exchange is fatal to the whole run, and main.go:323 mints unconditionally before the task-type switch so healthcheck/oauth-probe liveness runs fail on auth they do not need.

## Reproduction

The implicated source location from the deep verdict is `githubapp/githubapp.go:107` in `bborbe/maintainer` (resolution rule: `candidate position 1 (bborbe/maintainer)`), Sentry issue `NUKE-PROD-C2`.

## Recommended Fix

Add ctx-aware bounded exponential backoff (3 attempts, ~250ms/500ms/1s, 5xx/network only) around transport.Token in githubapp.MintIAT so all consumers benefit, and skip the mint for TaskTypeHealthcheck/TaskTypeOAuthProbe in main.go while keeping it fatal for real analysis runs.
