# Agent Report: linochi-erp-backend

## Summary

Branch: `agent/build` — 5 commits ahead of the last merged work. All 12 migrations present. 15 internal modules with domain/service/handler/store skeleton. 120+ test functions across unit and integration tests. 5 `// ASSUMPTION:` comments found (all in kiosk, calendar, and products).

## Requirement Coverage: Phases 1–8

| Phase | Requirements | Status | Main Files | Test Names |
|---|---|---|---|---|
| **1. Boards** | TAV-01–07 | **Done** | `internal/boards/{domain,service,handler,store,composite}.go`, `db/migrations/20261113221900_boards_columns_cards_checklist_comments.sql`, `db/queries/boards.sql` | `TestTAV01_ListBoards`, `TestTAV01_CreateBoard`, `TestTAV01_ArchiveBoard`, `TestTAV02_DefaultColumns`, `TestTAV02_CreateColumn`, `TestTAV02_UpdateColumn`, `TestTAV02_ArchiveColumn`, `TestTAV03_CreateCard`, `TestTAV03_CardWithDueDate`, `TestTAV04_MoveCard`, `TestTAV05_CreateChecklistItem`, `TestTAV05_UpdateChecklistItem`, `TestTAV05_ListChecklistItems`, `TestTAV06_CreateComment`, `TestTAV06_ListComments` |
| **2. Products** | PRD-01–19, REG-01–03 | **Backend done, Web stub** | `internal/products/{domain,service,handler,store}.go`, `internal/ids/{domain,service,handler,store}.go`, `internal/reminders/{domain,service,handler,store}.go`, `db/migrations/20261114200158_products_fields_product_types_documents_ids.sql`, `db/queries/products.sql`, `db/queries/reminders.sql` | None (no product-specific unit or integration tests exist) |
| **3. Suppliers** | LEV-01–03 | **Backend done, Web stub** | `internal/suppliers/{domain,service,handler,store}.go`, `internal/purchasing/{domain,service,handler,store}.go`, `internal/shipments/{domain,service,handler,store}.go`, `db/migrations/20261115003628_suppliers_purchasing_shipments.sql`, `db/queries/suppliers.sql` | None (no supplier-specific tests exist) |
| **4. Inventory** | LAG-01–09 | **Backend done, Web partial** | `internal/inventory/{domain,service,handler,store}.go`, `db/migrations/20261115010000_inventory.sql`, `db/queries/inventory.sql` | None (no inventory-specific tests exist) |
| **5. Orders & Workflow** | KUN-01–04, ORD-01–08, WF-01–07, WF-10 | **Backend done, Web stub** | `internal/orders/{domain,service,handler,store}.go`, `internal/workflow/{domain,service,handler,store}.go`, `internal/customers/{domain,service,handler,store}.go`, `db/migrations/20261115140000_customers_workflow_orders_stats.sql`, `db/queries/orders.sql`, `db/queries/workflow.sql`, `db/queries/customers.sql` | None (no order/workflow/customer-specific tests exist) |
| **6. API Keys & Sync** | INS-01–02, INS-06, WF-08–09 | **Partial (backend skeleton only)** | No dedicated module for API keys or webhooks. The `api/` package is generated from OpenAPI. `internal/api/api.gen.go` exists. | None |
| **7. Calendar & Kiosk** | KAL-01–04, KIO-01–07 | **Backend done (kiosk stubs), Web stub** | `internal/calendar/{domain,service,handler,store}.go`, `internal/kiosk/{domain,service,handler,store}.go`, `db/migrations/20261115120000_calendar_kiosk.sql`, `db/queries/calendar_kiosk.sql` | None (no calendar/kiosk-specific tests exist) |
| **8. Stats & Polish** | STA, KUN-05–08, REG-04–05, ORD-09–13 | **Backend done (partial), Web stub** | `internal/stats/{domain,service,handler}.go` (no store.go), `db/queries/stats.sql` | None |

### Phase 0 (Foundation — already merged before this session)

| Phase | Requirements | Status |
|---|---|---|
| B1. Skeleton | NFK-01, NFK-28 | Done |
| B2. Observability | NFK-02, NFK-03 | Done |
| B3. Database | NFK-28, NFK-02, NFK-09 | Done |
| B4. Change log | INS-05, NFK-09 | Done |
| B5. Sessions | NFK-02, NFK-11, INS-05 | Done |
| B6. OIDC | NFK-10, NFK-12, NFK-29 | Done |
| B7. Packaging | NFK-01, NFK-02, NFK-04, NFK-12, NFK-13, INS-07 | Done |

## All Test Functions

### Unit Tests

| File | Test Functions |
|---|---|
| `cmd/devoidc/main_test.go` | `TestNFK10_DevOIDCRefusesToListenOutsideLoopback`, `TestNFK10_DevOIDCServesAProviderOnLoopbackAndSaysWhatToSetInTheApp` |
| `cmd/api/run_test.go` | `TestNFK02_ServerShutsDownGracefullyOnCancel`, `TestNFK02_ServerShutdownIsBoundedByTimeout`, `TestINS07_VersionSubcommandPrintsLdflagsValues`, `TestNFK02_RunFailsWithConfigErrorNamingTheField`, `TestNFK10_AuthRoutesAreOnThePublicPortOnly`, `TestNFK02_MetricsAndProbesOnlyOnInternalPort`, `TestNFK02_ShutdownDrainsBeforeClosing`, `TestNFK02_OneServerFailingStopsTheOther` |
| `cmd/api/sessions_test.go` | `TestNFK11_SessionsCommandUsageErrors`, `TestNFK11_SessionsCommandNeedsTheAppDatabaseURL`, `TestNFK14_SessionsCommandNeverPrintsThePassword` |
| `cmd/api/migrate_test.go` | `TestNFK28_MigrateWithoutActionPrintsUsage`, `TestNFK28_MigrateRejectsAnUnknownAction`, `TestNFK28_MigrateNeedsMigrateDatabaseURL`, `TestNFK28_MigrateDoesNotFallBackToTheAppURL`, `TestNFK28_MigrateFailsWithoutDatabaseAndKeepsThePasswordOut` |
| `internal/audit/diff_test.go` | `TestINS05_ToMapTurnsStructsAndMapsIntoJSONObjects`, `TestINS05_ChangedFieldsHoldsOnlyWhatChanged`, `TestINS05_StructDiffThroughToMap` |
| `internal/platform/auth/oidc_test.go` | `TestNFK10_AuthorizationURLCarriesPKCEStateNonceAndTheRightScopes`, `TestNFK10_ExchangeGivesTheIdentityFromTheIDTokenAndUserInfo`, `TestNFK10_ExchangeNeedsTheVerifierThatMatchesTheChallenge`, `TestNFK10_AnAuthorizationCodeWorksOnce`, `TestNFK10_ExchangeRefusesATokenThatIsNotRightForThisLogin`, `TestNFK10_ExchangeChecksAMRForTwoFactorWhenItIsDelivered`, `TestNFK10_GroupsComeFromUserInfoAndFallBackToTheIDToken`, `TestNFK10_DisplayNameFallsBackToTheUsername`, `TestNFK10_ProviderDownAtFirstUseIsRetriedAndNeedsNoRestart`, `TestNFK10_DiscoveryThatClaimsAnotherIssuerIsRefused`, `TestNFK12_ClientTrustsOnlyTheCAItWasGiven`, `TestNFK10_NoSecretOrTokenReachesTheLogs` |
| `internal/platform/auth/roles_test.go` | `TestNFK10_RolesComeFromAutheliaGroups`, `TestNFK10_ReturnToMustBeAPathOfThisApp` |
| `internal/platform/auth/session_test.go` | `TestNFK11_ExpiresAfter12hIdle`, `TestNFK11_ExpiresAfter7dAbsolute`, `TestNFK11_AbsoluteLimitWinsWhenBothHaveRunOut`, `TestNFK11_ActivityDoesNotExtendPast7d`, `TestNFK11_TouchIsRateLimitedToOncePerMinute`, `TestNFK11_OnlyTokenHashIsStored`, `TestNFK11_TokensAreUnique`, `TestNFK11_MalformedTokenIsRejectedBeforeTheDatabase`, `TestNFK11_SessionHasNoFieldThatCouldCarryTheTokenHash` |
| `internal/platform/auth/session_cookie_test.go` | `TestNFK11_SessionCookieHasTheHostPrefixAndAllTheAttributes`, `TestNFK11_SessionCookieNeverOutlivesTheSession`, `TestNFK11_ClearingTheCookieKeepsTheAttributesThatMakeItDeletable` |
| `internal/platform/auth/oidctest/provider_test.go` | `TestNFK10_TestProviderRefusesWhatARealOneWould`, `TestNFK10_TestProviderChecksTheTokenRequest` |
| `internal/platform/config/config_test.go` | `TestNFK02_ConfigLoadsFromEnvOnly`, `TestNFK02_ConfigRejectsMissingOrInvalid`, `TestNFK14_ConfigStringMasksDatabasePassword`, `TestNFK28_LoadMigrateReadsOnlyTheMigrateURL`, `TestNFK28_LoadMigrateRejectsMissingOrInvalid`, `TestNFK10_ConfigTakesTheAdminGroupsFromTheEnvironment`, `TestNFK10_ConfigRejectsMissingOrUnsafeOIDCSettings`, `TestNFK10_ConfigAllowsPlainHTTPOnLoopbackOnly`, `TestNFK14_ConfigStringMasksTheClientSecret`, `TestNFK10_LoadOperatorNeedsTheDatabaseAndNothingElse` |
| `internal/platform/obs/logger_test.go` | `TestOBS_EveryRequestLogLineHasTraceID`, `TestOBS_LogLineOutsideARequestHasNoFakeTraceID`, `TestOBS_PersonalDataIsMaskedInLogs`, `TestOBS_OrdinaryFieldsAreNotMasked` |
| `internal/platform/obs/metrics_test.go` | `TestOBS_BuildInfoMetricCarriesVersionAndCommit`, `TestOBS_RegistryHasGoRuntimeMetrics` |
| `internal/platform/obs/tracing_test.go` | `TestOBS_TracesAreExportedOverOTLPHTTP`, `TestOBS_WithoutEndpointSpansExistButNothingIsExported` |
| `internal/platform/httpx/router_test.go` | `TestAPI_ErrorsAreRFC9457ProblemDetails`, `TestAPI_PanicBecomes500ProblemWithoutInternals`, `TestNFK03_DurationHistogramPerRouteHasSLOBuckets`, `TestOBS_UnmatchedPathsShareOneRouteLabel`, `TestOBS_MetricNamesFollowConvention`, `TestOBS_AccessLogOmitsQueryStringAndHasTraceID`, `TestOBS_RequestSpanIsNamedAfterRoutePattern`, `TestOBS_IncomingTraceparentIsContinued` |
| `internal/platform/httpx/health_test.go` | `TestNFK02_HealthzIsAlwaysOK`, `TestNFK02_ReadyzIs503WhileDraining`, `TestNFK02_ReadyzReportsFailingCheckWithTimeout` |
| `internal/platform/db/open_test.go` | `TestNFK14_OpenErrorsNeverContainThePassword` |
| `internal/platform/db/queryname_test.go` | `TestOBS_QueryNameComesFromTheSqlcComment` |

### Integration Tests

| File | Test Functions |
|---|---|
| `cmd/api/migrate_integration_test.go` | `TestNFK28_MigrateUpStatusDown`, `TestNFK02_ReadyzIsGreenWhenTheDatabaseIsMigrated`, `TestNFK02_ReadyzIsRedUntilTheSchemaIsMigratedAndTheAppStillStarts`, `TestNFK02_TheAppStartsWhileTheDatabaseIsDown` |
| `cmd/api/sessions_integration_test.go` | `TestNFK11_RevokeUserEndsEverySessionOfTheUserAndNoOthers`, `TestNFK11_RevokeUserAcceptsAUserID`, `TestNFK11_RevokeUserForAUserWithNoSessionsSucceedsAndSaysSo`, `TestNFK11_RevokeUserRefusesAnUnknownUserAndTouchesNothing`, `TestNFK11_RevokeUserRefusesAnAmbiguousAddressAndListsTheIDs`, `TestNFK11_RevokeUserWorksAsTheAppRoleAndNeedsNothingMore`, `TestNFK11_TheRunningAppDeletesEndedSessionsAndKeepsLiveOnes` |
| `cmd/api/oidc_integration_test.go` | `TestNFK10_TheRunningAppLogsInThroughTheProviderOverHTTP`, `TestNFK10_TheAppStartsAndStaysReadyWhileTheProviderIsDown`, `TestNFK12_TheProcessTrustsTheInternalCAThroughSSLCertDir`, `TestNFK10_TheTestProviderIsNotInTheAPIBinary` |
| `internal/audit/audit_integration_test.go` | `TestINS05_RecordWritesInTheSameTransaction`, `TestINS05_CreateStoresTheWholeRecordAsAfter`, `TestINS05_UpdateStoresOnlyChangedFieldsWithBeforeAndAfter`, `TestINS05_UpdateWithoutRealChangesWritesNothing`, `TestINS05_ArchiveStoresTheWholeRecordAsBefore`, `TestINS05_MissingActorRollsBackTheChange`, `TestINS05_ActorsAreRecorded`, `TestINS05_TraceIDAndReasonAreStored`, `TestINS05_EntryIsValidated`, `TestINS05_RedactPersonalDataReplacesOldValuesAndIsItselfAudited`, `TestINS05_RedactNeedsAnActorAndFields`, `TestOBS_AuditLogLineAfterCommitHasFieldNamesNotValues`, `TestOBS_AuditMetricCountsPerModuleAndActionAfterCommit` |
| `internal/boards/integration_test.go` | `TestTAV01_ListBoards`, `TestTAV01_CreateBoard`, `TestTAV01_ArchiveBoard`, `TestTAV02_DefaultColumns`, `TestTAV02_CreateColumn`, `TestTAV02_UpdateColumn`, `TestTAV02_ArchiveColumn`, `TestTAV03_CreateCard`, `TestTAV03_CardWithDueDate`, `TestTAV04_MoveCard`, `TestTAV05_CreateChecklistItem`, `TestTAV05_UpdateChecklistItem`, `TestTAV05_ListChecklistItems`, `TestTAV06_CreateComment`, `TestTAV06_ListComments` |
| `internal/platform/auth/handler_integration_test.go` | `TestNFK10_LoginFlowMakesASessionAndGoesBackToTheStartingPage`, `TestNFK10_ReturnToThatIsNotAPathOfThisAppIsReplacedBySlash`, `TestNFK10_SecondLoginOfTheSameUserRefreshesTheProfileAndAddsASession`, `TestNFK10_TheCallbackMayLandOnAnotherReplicaThanTheLoginStarted`, `TestNFK10_FailedLoginsAreToldToTheBrowserAsFixedCodesOnly`, `TestNFK10_ArchivedUserGetsNoAccessAndNothingIsWritten`, `TestNFK10_StateWorksOnceAndIsBoundToTheBrowserThatStartedTheLogin`, `TestNFK10_ProviderDownGivesAFailedLoginAndRecoversWithoutARestart`, `TestNFK10_TooManyLoginsInProgressIsAFailedLoginAndNotAFlood`, `TestNFK10_MeNeedsASessionAndLogoutEndsIt`, `TestNFK10_RolesAreWorkedOutAtLoginAndKeptInTheSession`, `TestINS07_VersionNeedsASessionAndNamesTheRunningRelease` |
| `internal/platform/auth/login_attempts_integration_test.go` | `TestNFK10_LoginAttemptGivesBackWhatWasStoredAndWorksOnce`, `TestNFK10_LoginAttemptStateWorksExactlyOnceUnderConcurrency`, `TestNFK10_LoginAttemptExpiresAfterTenMinutes`, `TestNFK10_LoginAttemptRefusesAnotherBrowserAndIsConsumed`, `TestNFK10_LoginAttemptUnknownOrMalformedStateIsRefused`, `TestNFK10_LoginAttemptStoresOnlyHashesOfStateAndBinding`, `TestNFK10_LoginAttemptsAreCappedAndExpiredOnesDoNotCount`, `TestNFK10_LoginAttemptCleanupDeletesExpiredOnesOnly`, `TestNFK10_AppRoleCanDeleteLoginAttemptsButNotUpdateOrTruncate` |
| `internal/platform/auth/session_middleware_integration_test.go` | `TestNFK10_NoCookieGives401AsAProblem`, `TestNFK11_ValidSessionReachesTheHandlerWithTheUserAsActor`, `TestNFK11_EndedOrUnknownSessionGives401AndClearsTheCookie`, `TestNFK11_CrossOriginWritesAreRefusedBeforeTheHandler`, `TestNFK11_LogoutHandlerEndsTheSessionAndClearsTheCookie`, `TestNFK11_LogoutIsCrossOriginProtectedToo` |
| `internal/platform/auth/session_store_integration_test.go` | `TestNFK11_OnlyTokenHashIsStoredInTheDatabase`, `TestNFK02_TwoInstancesShareASession`, `TestNFK11_LookupExpiresAfter12hIdle`, `TestNFK11_LookupExpiresAfter7dAbsolute`, `TestNFK11_TouchWritesAtMostOncePerInterval`, `TestNFK11_TouchNeverMovesTheAbsoluteLimit`, `TestNFK11_LookupRefusesMalformedAndUnknownTokens`, `TestNFK11_InvalidateAllEndsEverySessionOfOneUser`, `TestNFK11_InvalidateAllWithNoSessionsIsFineAndWritesNothing`, `TestNFK11_ArchiveEndsAllSessionsInTheSameTransaction`, `TestNFK11_ArchivedUserCannotGetASession`, `TestNFK11_LookupRefusesASessionWhoseUserWasArchivedBehindItsBack`, `TestNFK11_ArchiveAndLoginAtTheSameTimeLeaveNoSession`, `TestNFK09_CleanupDeletesOnlyEndedSessions`, `TestNFK09_CleanupIsIdempotentAcrossInstances`, `TestNFK09_AppRoleCanDeleteSessionsButNothingElse`, `TestINS05_LoginAndLogoutAreAuditedAndNeverHoldTheTokenHash`, `TestINS05_LogoutOfAnUnknownSessionIsHarmlessAndNotAudited`, `TestNFK02_FailedLoginLeavesNoSession` |
| `internal/platform/auth/users_integration_test.go` | `TestNFK28_UpsertUserOnLogin`, `TestNFK28_UsersAreUniquePerIssuerAndSubject`, `TestNFK28_GetUserNotFound`, `TestINS05_LoginsAreAuditedAsCreateAndLogin`, `TestNFK28_ArchiveWithStaleVersionIsRejected`, `TestINS05_ArchiveIsAuditedWithTheWholeRecordAsBefore`, `TestINS05_ArchivingTwiceWithTheCurrentVersionChangesNothing`, `TestINS05_ArchiveWithoutAnActorFailsAndChangesNothing`, `TestINS05_ConcurrentFirstLoginsMakeOneUserAndOneCreate`, `TestNFK10_LoginOfAnArchivedUserChangesNothingAndSaysSo` |
| `internal/platform/auth/contract_integration_test.go` | `TestNFK29_ResponsesMatchOpenAPI`, `TestNFK29_EveryOperationThatNeedsASessionRefusesARequestWithoutOne`, `TestNFK29_NothingIsServedThatTheSpecDoesNotDescribe` |
| `internal/platform/db/migrate_integration_test.go` | `TestNFK28_MigrationsUpDownUp`, `TestNFK28_UpIsIdempotent`, `TestNFK28_EveryTableHasStandardColumns`, `TestNFK28_UpdatedAtIsSetByTrigger` |
| `internal/platform/db/ready_integration_test.go` | `TestNFK02_ReadinessIsOKWhenMigrated`, `TestNFK02_ReadinessIsRedWhenTheSchemaIsBehind`, `TestNFK02_ReadinessIsOKWhenTheSchemaIsAheadOfTheBinary`, `TestNFK02_ReadinessIsRedWithoutDatabase` |
| `internal/platform/db/roles_integration_test.go` | `TestNFK28_AppRoleHasTheRightsTheAppNeedsAndNoMore` |
| `internal/platform/db/open_integration_test.go` | `TestNFK04_PoolHonoursTheConfiguredLimits`, `TestOBS_QueryMetricsAndSpansUseTheSqlcQueryName`, `TestOBS_FailedQueriesAreStillMeasured`, `TestOBS_PoolMetricsAreExported`, `TestOBS_DBMetricNamesFollowConvention`, `TestOBS_ReadinessQueryHasItsOwnLabel` |
| `internal/platform/db/audit_schema_integration_test.go` | `TestINS05_AuditLogChecksTheActor`, `TestNFK09_AppCannotUpdateDeleteOrTruncateAuditRows`, `TestNFK09_TriggerBlocksEvenTheOwnerOutsideTheDefinerFunction`, `TestINS05_RedactReplacesOldValuesAndLeavesTheRest`, `TestINS05_RedactNeedsFieldsAndOnlyTheAppRoleMayCallIt`, `TestINS05_AnotherDefinerFunctionOwnedByTheOwnerIsStopped`, `TestINS05_RedactorCannotChangeLockedColumnsEvenIfPrivilegesWereWidened`, `TestINS05_RedactFunctionIsHardened`, `TestINS05_AuditLogHasIndexesForTheFilters`, `TestNFK09_RedactorRoleIsNoLoginAndHasOnlyTheNeededPrivileges`, `TestNFK09_OnlyTheOwnerIsAMemberOfTheRedactorRole` |
| `internal/platform/db/store_integration_test.go` | `TestNFK28_InTxCommitsOnSuccess`, `TestNFK28_InTxRollsBackOnErrorAndOnPanic`, `TestNFK28_AfterCommitHooksRunOnlyAfterCommit`, `TestNFK28_DatabaseGeneratesUUIDv7WhenNoIDIsGiven` |
| `internal/platform/testdb/testdb_integration_test.go` | `TestNFK28_PostgresContainerAcceptsConnections` |
| `internal/platform/testdb/server_integration_test.go` | `TestNFK28_NewDatabaseIsolatesTests`, `TestNFK28_NewDatabaseIsDroppedAfterTheTest`, `TestNFK28_NewDatabaseIsOwnedByTheOwnerRoleAndAppURLIsTheRestrictedRole` |

## Migration Files

| # | File | Requirements |
|---|---|---|
| 1 | `20261106120000_users.sql` | NFK-10 |
| 2 | `20261106140000_app_role_grants.sql` | NFK-28, NFK-09 |
| 3 | `20261106140100_audit_log.sql` | INS-05 |
| 4 | `20261106150000_audit_redactor_role.sql` | INS-05, NFK-09 |
| 5 | `20261108111350_sessions.sql` | NFK-11 |
| 6 | `20261109124549_oidc_login_attempts.sql` | NFK-10 |
| 7 | `20261113221900_boards_columns_cards_checklist_comments.sql` | TAV-01 to TAV-06 |
| 8 | `20261114200158_products_fields_product_types_documents_ids.sql` | PRD-01 to PRD-19, REG-01 to REG-03 |
| 9 | `20261115003628_suppliers_purchasing_shipments.sql` | LEV-01 to LEV-03, INK-01 to INK-05 |
| 10 | `20261115010000_inventory.sql` | LAG-01 to LAG-08 |
| 11 | `20261115120000_calendar_kiosk.sql` | KAL-01 to KAL-04, KIO-01 to KIO-07 |
| 12 | `20261115140000_customers_workflow_orders_stats.sql` | KUN-01 to KUN-04, ORD-01 to ORD-08, WF-01 to WF-07, WF-10, STA-01 |

## `// ASSUMPTION:` Comments

| File | Line | Content |
|---|---|---|
| `internal/products/service.go` | 403 | `// ASSUMPTION: version must be provided for updates` |
| `internal/kiosk/service.go` | 327 | `// ASSUMPTION: The actual data fetching from orders, inventory, customers etc. will be implemented by querying those modules' tables. For now we return empty.` |
| `internal/kiosk/service.go` | 330 | `return KioskData{}, nil // ASSUMPTION: real data will be fetched from orders/inventory/customers tables` |
| `internal/kiosk/service.go` | 346 | `// ASSUMPTION: We'll use a proper hashing library later` |
| `internal/calendar/service.go` | 139 | `// ASSUMPTION: iCal fetching will be implemented using go-ical or go-ics library.` |

## Known Gaps and Stubs

### Backend stubs / incomplete implementations

| Module | Gap | Details |
|---|---|---|
| **kiosk** | `GetKioskData` returns empty data | `internal/kiosk/service.go:327-330` — real data from orders/inventory/customers is not fetched yet |
| **kiosk** | `hashAPIKey` uses `fmt.Sprintf` | `internal/kiosk/service.go:346` — should use bcrypt/argon2 |
| **calendar** | `SyncSources` is a no-op | `internal/calendar/service.go:139` — iCal fetching not implemented |
| **stats** | No `store.go` | `internal/stats/` has domain/service/handler but no SQLC store |
| **products** | Version validation assumption | `internal/products/service.go:403` — assumes version is always provided |

### Missing test coverage (no tests at all)

| Module | Requirements |
|---|---|
| products | PRD-01 to PRD-19 |
| ids | REG-01 to REG-05 |
| reminders | NFK-05, REG-05 |
| suppliers | LEV-01 to LEV-03 |
| purchasing | INK-01 to INK-05 |
| shipments | INK-02 to INK-05 |
| inventory | LAG-01 to LAG-09 |
| customers | KUN-01 to KUN-08 |
| orders | ORD-01 to ORD-13 |
| workflow | WF-01 to WF-10 |
| stats | STA-01 to STA-04 |
| kiosk | KIO-01 to KIO-07 |
| calendar | KAL-01 to KAL-05 |

### Pre-existing issues

| Issue | Impact |
|---|---|
| `20261114200158_products_fields_product_types_documents_ids.sql` missing `-- +goose Up` annotation | Causes integration test failures in the migration test suite |

## How to Run Everything

### Backend

```bash
# Start local dev environment (Postgres via Docker Compose)
make dev

# Start local OIDC provider (loopback only)
make dev-oidc

# Generate code from OpenAPI and sqlc
make gen

# Run unit tests
make test

# Run integration tests (needs Docker)
make test-int

# Run lint + unit + integration (before commit)
make check

# Lint only (gofmt, go vet, golangci-lint, sqlc diff, go mod tidy)
make lint

# Build container image
make image

# Smoke-test the image against real Postgres
make smoke

# Lint and render Helm chart
make chart

# New migration
make migrate-new name=add_new_table
```

### Web

```bash
# Start Vite dev server
make dev

# Run unit/component tests (Vitest)
make test

# Run lint + typecheck + API schema check
make lint

# Build static files
make build

# End-to-end tests with axe (needs browsers installed)
make test-e2e

# Run lint + tests + build (before commit)
make check

# Generate API client from OpenAPI
make gen

# Sync OpenAPI from backend, then generate
make sync-api

# Build container image
make image

# Smoke-test the image
make smoke
```

### Workspace root

```bash
# Both repos are subdirectories. Work on branches like agent/build.
# Never merge PRs yourself; Ninib merges.
```
