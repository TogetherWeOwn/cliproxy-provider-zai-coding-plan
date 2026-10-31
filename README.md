# Z.ai Coding Plan provider for CLIProxyAPI

API-key authentication, a static namespaced model catalog, Anthropic Messages
execution, and a read-only Coding Plan quota view. Multiple auth files are
selected by the host's native routing; this plugin does not install a scheduler.

## Status and safety

**Pending live verification.** Quota authorization variants, response shapes,
model availability, reset-time conventions, and entitlement have not been
verified with a real Coding Plan credential. A Standard key may incur
pay-as-you-go charges even at Coding Plan endpoints. Check key classification
and applicable subscription terms before use. No inference call is required for
this repository's tests. The plugin does not prevent spend if given a Standard key.

Create a private, mode-0600 auth file in the host auth directory, for example
`{"type":"zai-coding-plan","api_key":"example-key","region":"global"}`.
`region` may be `global` (default) or `cn`. Optional host fields include `label`,
`prefix`, `weight`, `priority`, and `disabled`. The default auth ID is the
provider name and first 12 hexadecimal digits of SHA-256 of the trimmed key.
Keep credentials out of source control. Models are `zai/glm-5.3` and
`zai/glm-5.3-flash`; the `zai/` namespace is stripped before inference.

Quota displays only recognized CREDIT_LIMIT rows, allowing one valid window
when the other is invalid. It does not invent limits for older plan shapes or
estimate quota after a polling failure. Bearer authorization is tried first and
raw-key authorization only after HTTP 401. Quota values are cached for 60 seconds
by the shared kit; errors are not cached. Quota is informational, not a
preemptive routing policy. Reset timestamps lacking a timezone in error messages
are interpreted as UTC; verify this assumption before depending on it.

## Build and tests

Go 1.26.10, CGO and a C compiler are required. The provider kit is pinned in
`go.mod` but its required commit is currently local-only. The temporary relative
`replace` expects a sibling checkout `../cliproxy-provider-kit` at commit
`785a03e0bc7323e8f32496465e72edbd5ea28f43`; publish that commit and remove the
replace before distributing this module independently. The host integration
module uses the unmodified v8.0.48 loader, not a live service.

```sh
go vet ./...
go test -race -count=1 ./...
CGO_ENABLED=1 go build -buildmode=c-shared -o /path/to/zai-coding-plan.so .
go -C integration/hostcompat vet ./...
go -C integration/hostcompat test -race -count=1 ./...
```

The host integration builds a **test-only** `hostcompat`-tagged library whose
upstream is restricted to a literal HTTP loopback address. Production builds
must not use that tag. Tests use fake keys and httptest servers only.

## Sources

- [Official Coding Plan quick start](https://docs.z.ai/devpack/quick-start.md)
- [Official key classification](https://docs.z.ai/devpack/notice/api-key-management.md)
- [Official error codes](https://docs.z.ai/api-reference/api-code.md)
- [Official usage query implementation](https://github.com/zai-org/zai-coding-plugins/blob/0446d0bb0bc537d97d3ab3664c4b8b9c4a0e1254/plugins/glm-plan-usage/skills/usage-query-skill/scripts/query-usage.mjs)
- [Independent existing quota parser](https://github.com/TogetherWeOwn/cpa-plugin-subscription-pool/blob/b59dd73f68ba6b1a18d7c8286b6676e1bd160a77/internal/providers/zai/quota.go)
- [Subscription terms](https://docs.z.ai/legal-agreement/subscription-terms.md)

The credit parser is a new implementation based on the documented wire shape
and existing parser behavior; it is not a copy of the earlier parser's code.
