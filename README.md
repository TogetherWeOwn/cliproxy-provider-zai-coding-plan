# Z.ai Coding Plan provider for CLIProxyAPI

A CLIProxyAPI v8.0.48 dynamic provider plugin using
[`cliproxy-provider-kit`](https://github.com/TogetherWeOwn/cliproxy-provider-kit) at `f24cf1f` (v0.1.0 plus quota proxy support).
It supplies API-key authentication, a static namespaced GLM catalog, Anthropic
Messages execution, and a read-only credit-quota card. It does **not** install a
scheduler, estimate credits, refresh quota on inference, or route by quota percent.
Multiple auth files use the host's native selection and reactive cooldowns.

## Authentication and endpoints

Create a private, mode-0600 JSON file in the host's auth directory. This example
contains a placeholder, not a usable credential.

```json
{
  "type": "zai-coding-plan",
  "api_key": "example-key",
  "label": "Example account",
  "region": "global"
}
```

`region` is `global` (default) or `cn`. The selected region must match the key's
entitlement. The plugin deliberately does not accept arbitrary upstream URLs.
Optional host fields are `id`, `label`, `prefix`, `proxy_url`, `weight`, `priority`,
and `disabled`. An optional `plan` hint is retained but never supplies quota
limits. Without `id`, the auth ID is `zai-coding-plan:` plus the first 12 hex
digits of SHA-256 of the trimmed key; repeated keys produce the same ID.
Login methods report API-key-only; refresh preserves the auth record.

| Region | Inference POST | Quota GET |
| --- | --- | --- |
| `global` | `https://api.z.ai/api/anthropic/v1/messages` | `https://api.z.ai/api/monitor/usage/quota/limit` |
| `cn` | `https://open.bigmodel.cn/api/anthropic/v1/messages` | `https://open.bigmodel.cn/api/monitor/usage/quota/limit` |

Inference sends `Authorization: Bearer <key>` and `anthropic-version:
2023-06-01` through the host HTTP client. Only protocol version/beta headers are
forwarded; caller credentials and URLs are not. The canonical format is `claude`;
CLIProxy translates other client formats. Token counting is the kit's local,
explicitly estimated count, not a billable upstream request.

Quota sends `Authorization: <key>` first, following the first-party usage script,
then tries `Bearer <key>` **only after HTTP 401**. No fallback on 403, 429, transport
failure, or a failed JSON envelope. The kit bounds polling to 15 seconds/1 MiB,
refuses redirects, redacts transport failures, and caches successful per-auth
results for 60 seconds; errors are not cached. Its separate quota transport is not the
executor's host transport, but it follows the same proxy choice: the auth's
`proxy_url`, else the host's global `proxy-url`, else the environment.

## Model prefixes

| Client/registered model | Model sent upstream |
| --- | --- |
| `zai/glm-5.3` | `glm-5.3` |
| `zai/glm-5.3-flash` | `glm-5.3-flash` |

Every registered model has the fixed `zai/` namespace, independently of the
optional host auth-file `prefix`. Both execution paths and the low-level HTTP
executor strip that namespace. No legacy aliases or `[1m]` pseudo-models are
advertised. Model availability is static, not inferred by making provider calls.

## Quota and errors

Quota displays only recognized `CREDIT_LIMIT` rows. `usage` is capacity;
`currentValue` is consumption. Five hours is `unit=3, number=5`; weekly is
`unit=6, number=1` or the observed `number=7` variant. The remaining fraction is
`(usage-currentValue)/usage`, not the rounded `percentage` field. Reset times are
epoch milliseconds; null, absent, and zero mean unknown. Integral decimal numbers
are accepted; fractional credits and malformed/unknown rows are not guessed. A
valid window is retained when another is invalid. Legacy token/MCP-only shapes
return an unknown-quota error rather than fabricated credit allowances. Plan
metadata comes from `data.level`/`data.planName`, if present.

| Upstream code | Classification | HTTP / retry hint |
| --- | --- | --- |
| `1001`, `1003` | Authentication | 401, non-retryable |
| `1302`, `1305` | Transient rate/overload, not window exhaustion | 429, retryable, `Retry-After` or 10 s |
| `1308`, `1310`, `1316`–`1321` | Window/spend exhaustion | 429, retryable, parsed reset or `Retry-After`, otherwise 60 s |
| `1113`, `1309`, `1313` | Balance, expired plan, or fair-use restriction | 429, non-retryable, 1 h |
| `1311`, `1314`, `1315` | Model/enterprise/key-scope entitlement | 429, non-retryable, 1 h |

Structured `next_flush_time` and official “reset at”/“Resets at” message templates
are recognized, as epoch seconds/milliseconds or timestamps. Zoneless timestamps
are interpreted as UTC; that assumption still needs live confirmation. Error
messages exposed by this plugin never include the upstream body or credential.
Unrecognized errors retain their HTTP status (invalid statuses become 502).

**Host limitation:** v8.0.48's plugin RPC error adapter carries status, code, and
retry hints but does not implement the native `TransientRateLimitError` interface.
Consequently native selection applies its generic 429 path, including a
30-second minimum cooldown, even to the plugin's short transient hint. The
host-loader test verifies this remains short, and verifies exhaustion rotates to
a sibling auth. Neither the plugin's `retryable` flag nor its distinct code should
be mistaken for a guarantee that this host disables all retries or avoids all
quota-state markings. No local host patch or custom scheduler is included.

## Build, verify, and package

Requires Go **1.26.10**, CGO, a C compiler, Python 3 (release packaging), and a
Linux amd64 builder for release assets. No local module replacements are used.

```sh
go mod verify
go vet ./...
go test -race -count=1 ./...
go -C integration/hostcompat vet ./...
go -C integration/hostcompat test -race -count=1 -v ./...
CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -o /absolute/output/zai-coding-plan.so .
nm -D --defined-only /absolute/output/zai-coding-plan.so | grep ' T cliproxy_plugin_init$'
./scripts/package-release.sh /absolute/release-output
```

The script builds a **production** image (clears build flags/test tags), emits
`zai-coding-plan_0.1.0_linux_amd64.zip` containing `zai-coding-plan.so`, and writes
`checksums.txt` containing the archive's SHA-256. It creates and removes its own
scratch directory under the user's cache. It does not upload or install anything.
The real plugin-store parser and the archive contents/checksum/exported ABI symbol
are tested in `integration/hostcompat`. Upload both assets in a matching `v0.1.0`
GitHub release after review; `registry.json` is a schema-v1 GitHub-release source.

The host-compatibility module imports the **unmodified pinned v8.0.48 host** and
builds a test-only `hostcompat`-tagged library, restricted to literal HTTP loopback
upstreams. Production builds must not use that tag. Tests use placeholder keys
and `httptest` only, including auth persistence, model catalogs, usage accounting,
streaming, error propagation, native two-auth failover, quota caching, and release
packaging. CI runs formatting, vet, race tests, shared-library build/symbol checks,
and pinned gitleaks plus generic credential-pattern scans, without secrets.

## Pending live verification

No Z.ai subscription or credential was available. **No real provider inference or
quota endpoint was called.** Source corroboration is not live verification.

- Confirm Coding Plan key classification and supported-tool entitlement before
  any live inference. A Standard key may incur pay-as-you-go charges even at a
  Coding Plan URL; this plugin cannot identify the key class or prevent spend.
- Confirm raw/Bearer quota acceptance on each region and actual Personal/Team/
  legacy quota shapes, reset timezone, and model availability for that account.
- Confirm tool-call/streaming behavior, usage accounting, and actual error/reset
  bodies on an isolated staging CLIProxy with explicitly authorized traffic.
- Publish release assets and prove store installation and rollback on staging
  before any per-provider production cutover. Nothing is deployed by these tests.

See [source evidence and disagreements](docs/SOURCES.md) and the
[staging-first migration procedure](docs/MIGRATION.md). Check the
[subscription terms](https://docs.z.ai/legal-agreement/subscription-terms.md):
supported coding-tool use and individual entitlement matter; technical proxy
compatibility does not authorize general-purpose API use or third-party resale.
