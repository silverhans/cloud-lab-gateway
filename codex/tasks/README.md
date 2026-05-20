# Codex task prompts

Self-contained briefs for Codex agents. Each task is one prompt — paste it into Codex and let it run.

## Read first
Every Codex agent **must** read these two files before doing anything else:

1. [CLAUDE.md](../../CLAUDE.md) — house rules, structure, do-not-touch list
2. [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) — hexagonal layers, contexts, principles

## Batch 1 — parallelisable kickoff

Four tasks can start immediately. Stream A and Stream B work in different
file trees, so two Codex agents can run them in parallel without conflicts.
`A2.3` is the only follow-up task in this batch: start it after `A2.2`
lands, because it consumes the PoolRepo + UnitOfWork shipped there.

### Stream A — Backend Core (silverhans + user's Codex)

| File | Depends on | Est. effort |
|---|---|---|
| [A2.1_openstack_adapter.md](A2.1_openstack_adapter.md) | — (independent) | 3-4 h |
| [A2.2_pool_repo.md](A2.2_pool_repo.md) | — (independent) | 2-3 h |
| [A2.3_quota_and_create_lab.md](A2.3_quota_and_create_lab.md) | A2.2 merged or available in branch | 3-4 h |

### Stream B — Integrations + UX (partner + partner's Codex)

| File | Depends on | Est. effort |
|---|---|---|
| [B1.1_frontend_skeleton.md](B1.1_frontend_skeleton.md) | — (independent) | 2-3 h |
| [B1.2_moodle_emulator.md](B1.2_moodle_emulator.md) | — (independent) | 2-3 h |

## Batch 2 — adapters

| File | Stream | Depends on | Est. effort |
|---|---|---|---|
| [A2.4_pgxrepo_repos.md](A2.4_pgxrepo_repos.md) | A | A2.2 | 3-4 h |
| [A4.1_ansible_runner.md](A4.1_ansible_runner.md) | A | — (independent) | 3-4 h |

## Batch 3 — close the run-for-real gaps

The deploy + cleanup sagas, the queue adapter and the core repos are merged.
These three finish the path to a working end-to-end deploy. `A2.5` and `A2.6`
are independent backend tasks; `B1.3` is the HTTP entry point.

| File | Stream | Depends on | Est. effort |
|---|---|---|---|
| [A2.5_secrets_envkek.md](A2.5_secrets_envkek.md) | A | — (independent) | 3-4 h |
| [A2.6_deploy_step_repo.md](A2.6_deploy_step_repo.md) | A | A2.4 merged | 1-2 h |
| [B1.3_httpapi_foundation.md](B1.3_httpapi_foundation.md) | B | — (independent) | 4-5 h |

> **sqlc conflict:** `A2.5` and `A2.6` both regenerate
> `internal/adapters/storage/sqlcgen/`. Whichever PR merges second rebases on
> `main` and re-runs `make gen-sqlc`. Hand-written code never conflicts.

## Workflow for each Codex run

1. Pull latest `main`: `git pull origin main`
2. Create feature branch: `git checkout -b feat/<task-id>-<slug>`
3. Open the task file and feed it to Codex
4. Iterate until **all** acceptance criteria pass (`go test`, `go build`, `make check` where applicable)
5. Push branch, open PR against `main`
6. After PR review (Claude Code reviews) and CI green → merge

## House rules (also in CLAUDE.md)

- **Do not modify `internal/ports/`** without explicit coordination — that's the contract between Streams A and B.
- **No direct state mutation.** Aggregates use `.Transition()`, `.AllocateTo()` etc.
- **No `Co-Authored-By` in commits.**
- **Run `gitleaks detect --no-git` before pushing.**
- **No secrets in code.** Use env vars; sensitive material in DB goes through `SecretStore`.
- **One feature per PR.** No drive-by refactors.
