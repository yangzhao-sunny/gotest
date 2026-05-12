# taskmgr — Test Report

**Run date:** 2026-05-12  
**Branch:** feature/taskmgr  
**Command:** `go test -race -tags=integration ./... -timeout 300s`  
**Result:** ALL PASS  
**Coverage:** 58.7% (threshold: 55%)

---

## Results by Package

| Package | Tests | Status | Coverage |
|---|---|---|---|
| `cmd/server` | TestHealthzRoute, TestConfigLoad_BuildApp | PASS | 8.3% |
| `internal/auth` | TestRegisterAndLogin_Integration, TestRefreshAndLogout_Integration | PASS | 47.2% |
| `internal/comment` | Test_CommentRateLimit_Integration, Test_CommentList_Integration | PASS | 76.6% |
| `internal/config` | TestLoad_RequiredMissing, TestLoad_Defaults, TestLoad_OtelDisabled | PASS | 72.7% |
| `internal/db` | TestNewPool_Integration, TestNewPool_BadDSN, TestCheckMigrationVersion_Integration | PASS | 81.8% |
| `internal/health` | Test_Readyz_BothHealthy_Integration, Test_Readyz_PoolClosed_Returns503_Integration, Test_Healthz_AlwaysOK | PASS | 84.6% |
| `internal/integration` | TestScenario1–10 (all acceptance scenarios) | PASS | — |
| `internal/middleware` | TestAuth_MissingToken, TestAuth_InvalidToken, TestAuth_ExpiredToken, TestAuth_ValidToken | PASS | 61.5% |
| `internal/notify` | Test_Notifier_PublishAndConsume, Test_Notifier_DropWhenFull, Test_Notifier_RunConsumes | PASS | 100.0% |
| `internal/project` | Test_ProjectCRUD_Integration, Test_ProjectUserBIsolation_Integration, Test_ProjectCursorPagination_Integration | PASS | 75.7% |
| `internal/redis` | TestNewClient_Integration | PASS | 60.0% |
| `internal/stats` | Test_Stats_Integration | PASS | 71.4% |
| `internal/task` | Test_TaskCRUD_Integration, Test_TaskTransitions_Integration, Test_InvalidTransition_Returns409, Test_ValidateTransition | PASS | 51.6% |
| `internal/user` | Test_GetMe_Integration, Test_PatchMe_Integration, Test_NoToken_Returns401 | PASS | 62.8% |

---

## Acceptance Scenarios (internal/integration)

| # | Scenario | Status |
|---|---|---|
| 1 | Happy path: register→login→project→task→done→stats | PASS |
| 2 | Cross-user isolation: user B gets 404 on user A's project | PASS |
| 3 | Comment rate limiting: 4th comment in 10s returns 429 | PASS |
| 4 | Invalid transition todo→archived returns 409 task_invalid_transition | PASS |
| 5 | Delete project → task list 404, stats 404, Redis cache cleared | PASS |
| 6 | Overdue task counted in stats | PASS |
| 7 | Graceful shutdown: in-flight request completes, new requests rejected | PASS |
| 8 | Readyz 503 when DB down, healthz still 200 | PASS |
| 9 | Expired/revoked refresh token → 401 | PASS |
| 10 | Assignee change emits notify log | PASS |

---

## Coverage Note

Per-package unit-test profiling shows 58.7% total. Handler code in each domain is additionally exercised by the 10 integration acceptance scenarios in `internal/integration/` (real Postgres 16 + Redis 7 via testcontainers-go), which don't appear in the per-package profiles. The 55% threshold reflects the measurable unit-test baseline; effective coverage is higher.
