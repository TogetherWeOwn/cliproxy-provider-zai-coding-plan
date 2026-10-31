# Z.ai wire-protocol sources

Source review date: 2026-10-31. These are source observations, **not captured live
responses**. No real credential or subscription was used, and no live provider
inference or quota request was made. Public GitHub search identified the projects below; files were read at
immutable commits. Independent projects are counted by repository, not by number
of files or forks. The earlier provider in this organization is historical
corroboration, not counted as independent evidence.

## Pinned open-source implementations

| Ref | Repository and commit | Files inspected and relevant behavior |
| --- | --- | --- |
| S1, first-party | [zai-org/zai-coding-plugins](https://github.com/zai-org/zai-coding-plugins), `0446d0bb0bc537d97d3ab3664c4b8b9c4a0e1254` | [`plugins/glm-plan-usage/skills/usage-query-skill/scripts/query-usage.mjs`](https://github.com/zai-org/zai-coding-plugins/blob/0446d0bb0bc537d97d3ab3664c4b8b9c4a0e1254/plugins/glm-plan-usage/skills/usage-query-skill/scripts/query-usage.mjs): both regional Anthropic bases; regional monitor quota route; **raw Authorization**; old token/MCP display mapping, unfamiliar rows passed through. |
| S2, independent | [can1357/oh-my-pi](https://github.com/can1357/oh-my-pi), `c29f7c56f5530d805bb2887ade964aa55970d8e1` | [`packages/ai/src/usage/zai.ts`](https://github.com/can1357/oh-my-pi/blob/c29f7c56f5530d805bb2887ade964aa55970d8e1/packages/ai/src/usage/zai.ts): raw quota auth, `CREDIT_LIMIT`, exact consumed/capacity ratio, unit 3 hours / unit 6 weekly, reset parsing; [`packages/ai/src/providers/anthropic-client.ts`](https://github.com/can1357/oh-my-pi/blob/c29f7c56f5530d805bb2887ade964aa55970d8e1/packages/ai/src/providers/anthropic-client.ts): `authToken` means Bearer, whereas `apiKey` means `X-Api-Key`. |
| S3, independent | [SamyRai/go-z-ai](https://github.com/SamyRai/go-z-ai), `e276fbe5e0d8b7f1357029bc328295360e863f2c` | [`pkg/client/region.go`](https://github.com/SamyRai/go-z-ai/blob/e276fbe5e0d8b7f1357029bc328295360e863f2c/pkg/client/region.go): both hosts and Anthropic/monitor paths; [`pkg/client/quota.go`](https://github.com/SamyRai/go-z-ai/blob/e276fbe5e0d8b7f1357029bc328295360e863f2c/pkg/client/quota.go): credit shape, capacity/used semantics, weekly number 1 **or 7**, epoch-ms resets/idle zero; [`pkg/client/transport.go`](https://github.com/SamyRai/go-z-ai/blob/e276fbe5e0d8b7f1357029bc328295360e863f2c/pkg/client/transport.go): Bearer monitor transport; [`pkg/client/errors.go`](https://github.com/SamyRai/go-z-ai/blob/e276fbe5e0d8b7f1357029bc328295360e863f2c/pkg/client/errors.go): exact 1302/1305/1308–1321 constants; [`internal/coding/tool_claude.go`](https://github.com/SamyRai/go-z-ai/blob/e276fbe5e0d8b7f1357029bc328295360e863f2c/internal/coding/tool_claude.go): Coding Plan `ANTHROPIC_AUTH_TOKEN` setup and explicit Bearer expectation. |
| S4, independent | [robinebers/openusage](https://github.com/robinebers/openusage), `dd94b651cd70b1e373b9a89620463094f6414c41` | [`Sources/OpenUsage/Providers/ZAI/ZAIUsageClient.swift`](https://github.com/robinebers/openusage/blob/dd94b651cd70b1e373b9a89620463094f6414c41/Sources/OpenUsage/Providers/ZAI/ZAIUsageClient.swift): global quota route, Bearer; [`Sources/OpenUsage/Providers/ZAI/ZAIUsageMapper.swift`](https://github.com/robinebers/openusage/blob/dd94b651cd70b1e373b9a89620463094f6414c41/Sources/OpenUsage/Providers/ZAI/ZAIUsageMapper.swift): credit/token rows, unit 3 hours / unit 6 weeks, epoch-ms resets, percentage-based credit display. |
| S5, independent | [dmmdea/meta-router](https://github.com/dmmdea/meta-router), `7aaac9235c3dd879d09eb12ad8f23348f73c719f` | [`internal/orch/glmlane/glmlane.go`](https://github.com/dmmdea/meta-router/blob/7aaac9235c3dd879d09eb12ad8f23348f73c719f/internal/orch/glmlane/glmlane.go): global Anthropic base and token env; [`internal/orch/glmlane/errors.go`](https://github.com/dmmdea/meta-router/blob/7aaac9235c3dd879d09eb12ad8f23348f73c719f/internal/orch/glmlane/errors.go): 1302/1305 transient vs exhaustion vs 1311 config vs 1313 hard-stop, structured `next_flush_time` seconds. Its comments explicitly say its error fixtures are synthetic. |
| S6, independent | [jqknono/coding-plans-for-copilot](https://github.com/jqknono/coding-plans-for-copilot), `4935d547fa2c35706f11d09a3d4a0f0acbc1558f` | [`README.md`](https://github.com/jqknono/coding-plans-for-copilot/blob/4935d547fa2c35706f11d09a3d4a0f0acbc1558f/README.md): both regional Anthropic bases; configurable Bearer vs `x-api-key` auth, with `x-api-key` as the generic Messages default. |
| S7, independent | [happy-loki/codexhub](https://github.com/happy-loki/codexhub), `9154e9cd4283f7b4f2142cee5f089c1f2b5e72fe` | [`docs/ai-gateway-glm-anthropic-integration.zh-CN.md`](https://github.com/happy-loki/codexhub/blob/9154e9cd4283f7b4f2142cee5f089c1f2b5e72fe/docs/ai-gateway-glm-anthropic-integration.zh-CN.md): China Anthropic base + `/v1/messages`, `Authorization: Bearer`, protocol-version header, Coding Plan/API distinction. |

Historical corroboration: [TogetherWeOwn/cpa-plugin-subscription-pool](https://github.com/TogetherWeOwn/cpa-plugin-subscription-pool),
commit `b59dd73f68ba6b1a18d7c8286b6676e1bd160a77`,
[`internal/providers/zai/quota.go`](https://github.com/TogetherWeOwn/cpa-plugin-subscription-pool/blob/b59dd73f68ba6b1a18d7c8286b6676e1bd160a77/internal/providers/zai/quota.go):
Bearer quota auth, credit windows, exact integral decimals, independent-window
validation, and nullable unused-window reset. The present parser is independently
implemented; it does not import that scheduler or copy its quota engine.

## Official documentation used as the authority

- [International Coding Plan quick start](https://docs.z.ai/devpack/quick-start.md)
  and [China quick start](https://docs.bigmodel.cn/cn/coding-plan/quick-start.md):
  regional Anthropic bases and other protocol bases.
- [Coding Plan Claude Code setup](https://docs.z.ai/devpack/tool/claude.md) and
  [HTTP authentication](https://docs.z.ai/guides/develop/http/introduction.md):
  token-based inference configuration / Bearer header.
- [API error table](https://docs.z.ai/api-reference/api-code.md): exact business
  codes, meanings, HTTP statuses, and `{next_flush_time}` message templates.
- [Model guide](https://docs.z.ai/devpack/latest-model.md): static `glm-5.3` and
  `glm-5.3-flash`; no `[1m]` pseudo-model ID is registered here.
- [Key classification](https://docs.z.ai/devpack/notice/api-key-management.md):
  endpoint alone does not prove subscription billing rather than PAYG.
- [Subscription terms](https://docs.z.ai/legal-agreement/subscription-terms.md):
  supported coding tools and individual entitlement, no assumed third-party
  proxy/resale permission.

Documentation URLs are mutable; repository citations above are commit-pinned.

## Field-level cross-check and decisions

| Wire detail | Cross-check | Implementation decision / disagreement |
| --- | --- | --- |
| Global + China hosts | S1, S3, S6, S7 and both official quick starts agree | `api.z.ai` / `open.bigmodel.cn`, chosen by validated region; no user-supplied host. S3 explicitly marks the China monitor mirror as not live-verified in its project, so its source is not evidence of a live probe here. |
| Inference path/header | S3 Coding Plan setup and S7 say Bearer; S5 uses the global base/token env; S6 allows either auth header and defaults generic Messages to `x-api-key` | Follow official token setup and explicit GLM implementations: POST `/api/anthropic/v1/messages`, Bearer, version `2023-06-01`. Do not send both credentials or retry inference with another form (could duplicate a billable call). Bearer is supported by the cited implementations, not asserted to be the exclusive accepted form. |
| Quota path | S1, S2, S3, S4 agree | GET `/api/monitor/usage/quota/limit`, same region as inference. |
| Quota auth | **Disagreement:** S1/S2 raw key; S3/S4 and the old provider Bearer | Authority takes precedence over a third-party vote: first-party S1 + independent S2 establish raw-first. A 401-only Bearer fallback preserves the other independently implemented form. No fallback for 403/429 or JSON business failures. Changed from the original design's Bearer-first order; neither form is claimed universally accepted. |
| Envelope + rows | S2/S3: `data.level`, `limits[]`, `CREDIT_LIMIT`; S4 accepts credit/token; S1 predates credit rendering and passes unfamiliar rows through | Recognize credit windows only, retain optional `data.planName`, reject `success:false` or non-success `code` (0/200 supported). Unknown/legacy shapes fail honestly. No hardcoded plan capacities or token-credit estimator. |
| `usage` / `currentValue` | S2/S3 explicitly capacity / consumption; S4 uses that convention for its count lane but rounded `percentage` for credit display | Exact ratio wins over rounded percentages. Both absolute values must be valid integral numbers (including `.0`). Do not swap these misleadingly named fields. |
| `unit` / `number` | S2/S3/S4 agree unit 3 = hours, 6 = weeks. S3 records weekly `(6,1)` and `(6,7)`; S2 ignores number for unit 6. S4 multiplies the number into the period | Support `(3,5)` as five hours, `(6,1)` and `(6,7)` as one weekly window, following two independent implementations. Do not label other units/durations as weekly or invent their allowances. This extends the original design's strict `(6,1)` parser. |
| `nextResetTime` | S3/S4 and historical parser say epoch-ms; S3 treats zero as idle. S2 also heuristically accepts seconds | Follow majority epoch-ms interpretation, without the seconds heuristic for this field. Missing/null/zero means unknown, not 1970 or an estimated future reset; negative or malformed values invalidate only that row. |
| Error classes | S3 enumerates every documented code below. S5 separately distinguishes transient, exhaustion, model config, and fair-use | Exact meanings come from official docs plus S3; S5 corroborates behavioral classes, not every enterprise code (it leaves some unknown). No assumption that every HTTP 429 means credits exhausted. |
| `next_flush_time` | S5 extracts structured epoch seconds; official docs insert it into human-readable reset messages. S3 describes error codes but does not establish a universal reset format | Support structured numeric/string seconds or milliseconds, RFC3339 timestamps, and the official reset prose, including zoneless date-time strings. No guarantee the field exists outside the message. UTC for zoneless values is an explicit unverified assumption. |

## Error meanings and policy

The official error table and S3 agree on the following meanings. S5 independently
corroborates 1302/1305, 1308/1310, 1311/1313 and 1316–1321 as distinct classes,
but groups **1318 and 1320 as weekly**. Official docs and S3 both say **five-hour**;
those two sources win. This plugin uses the supplied reset, not S5's fixed
five-hour/weekly scheduling or hard-stop filesystem state.

| Code | Meaning | Plugin policy |
| --- | --- | --- |
| 1001 / 1003 | Missing authentication / expired authentication token | Non-retryable 401 |
| 1113 | Insufficient balance or no resource package | Non-retryable 429, long hint |
| 1302 | Request rate limit | Transient, short retryable 429 |
| 1305 | Temporary service overload | Transient, short retryable 429 |
| 1308 | Usage-window limit (`number`, `unit`, reset) | Exhaustion, reset-aware 429 |
| 1309 | Coding Plan expired | Non-retryable 429, long hint |
| 1310 | Weekly/monthly allowance exhausted | Exhaustion, reset-aware 429 |
| 1311 | Model not included in subscription | Entitlement, non-retryable 429 |
| 1312 | Not specified by the inspected official table or these implementations | Do not invent semantics; generic HTTP classification |
| 1313 | Fair Usage Policy restriction; restoration requires action | Non-retryable 429, long hint; not a normal quota window |
| 1314 | Enterprise plan expired | Entitlement, non-retryable 429 |
| 1315 | Wrong enterprise Coding Plan key scope | Entitlement, non-retryable 429 |
| 1316 / 1317 | Five-hour / seven-day allowance reached, insufficient balance for extra usage | Exhaustion, reset-aware 429 |
| 1318 / 1319 | Five-hour / seven-day allowance reached, extra usage unavailable due to monthly spend limit | Exhaustion, reset-aware 429 |
| 1320 / 1321 | Five-hour / seven-day allowance reached, extra usage unavailable due to monthly spend limit | Exhaustion, reset-aware 429 |

Reset hints are policy inputs to **native** host selection, not permission to
circumvent subscription or fair-use restrictions. Where a reset is absent or
unusable, this plugin uses a conservative 60-second exhaustion hint; it does not
infer an entire billing window. The entitlement/balance/fair-use long hint is one
hour and non-retryable in the plugin error envelope, not a claim of automatic
upstream recovery.

## Host evidence and verification boundaries

Host contracts were checked against public CLIProxyAPI **v8.0.48**:
`internal/pluginstore/registry.go` / `github.go` (schema/asset names),
`internal/pluginhost/loader_unix.go` (C ABI and resident runtime),
`internal/pluginhost/adapters.go` / `adapters_executors.go` (models/executors),
`internal/pluginhost/rpc_client.go` (status and retry-hint error adapter), and
`sdk/cliproxy/auth/conductor_cooldown.go` (native selection/cooldown behavior).
The real-loader integration consumes that exact upstream module, not a copied
host or a local core patch.

The native transient-rate-limit interface is not carried by v8.0.48's RPC error
adapter; its generic 429 path applies a 30-second floor to short transient hints.
This is covered by the real-host integration and documented in the README; it is
not fixed by adding another scheduler. Quota is display-only throughout.

The tests use new synthetic placeholder-key fixtures derived from these field
contracts, not copied real payloads or credentials. None of the independently
researched GLM client projects was executed. Source agreement does not prove account entitlement, auth acceptance,
actual model availability, real reset timezone, or successful production install.
Those remain in the README's **Pending live verification** section.
