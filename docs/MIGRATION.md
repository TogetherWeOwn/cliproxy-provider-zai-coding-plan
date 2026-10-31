# Staging-first migration

This is an operator checklist, not an installer. Tests and packaging do not
change any running CLIProxy, copy credentials, or authorize inference/spend.
There is no Z.ai-specific compatibility-config importer: create one native auth
file per legitimately entitled account, following the README. Do not copy
subscription-pool scheduling settings into this provider.

## 1. Prepare and retain rollback material

1. Confirm that the deployment's CLIProxy version is v8.0.48 and the platform is
   Linux amd64 with a compatible glibc. Use the published kit dependency pinned
   in `go.mod`; do not introduce a local `replace` or modify CLIProxy core.
2. Run all README verification commands. Build with
   `scripts/package-release.sh /absolute/release-output`, check the archive
   SHA-256 against `checksums.txt`, and verify `cliproxy_plugin_init` is exported.
3. Publish the zip and `checksums.txt` in the matching GitHub release only after
   review. Save the previous plugin binaries, config, and auth files privately,
   preserving modes and ownership. Never put these backups in this repository.
4. Confirm key classification, applicable Coding Plan terms, region, and
   supported-tool scope. A Standard/PAYG key is not a subscription key. The
   plugin cannot infer billing class from key syntax; do not discover it with
   inference calls.

## 2. Prove on an isolated staging CLIProxy first

1. Use an independent staging config, auth directory, plugin directory, and
   loopback-only listening port. Do not point it at production auth/plugin paths
   or expose staging management publicly. Leave production unchanged.
2. Add the released registry URL to staging's native plugin-store sources, then
   install the provider from the host's plugin store. The store resolves the
   GitHub release and checksum and installs the platform-specific shared library.

   ```yaml
   plugins:
     enabled: true
     dir: "/absolute/staging/plugins"
     store-sources:
       - "https://raw.githubusercontent.com/TogetherWeOwn/cliproxy-provider-zai-coding-plan/main/registry.json"
     configs:
       zai-coding-plan:
         enabled: true
   ```

   This snippet is only the plugin section, not a complete server config. The
   store URL becomes usable after the manifest and release are published. A
   manual alternative is to extract the verified `zai-coding-plan.so` under
   `plugins/linux/amd64/` in this isolated directory. Never install a build made
   with the `hostcompat` test tag.
3. Remove any previous binary declaring the same plugin ID from **staging's**
   discovery directory, retaining a private backup outside that directory.
   Renaming a `.so` within a scanned directory is not a safe disable mechanism.
   Also remove overlapping Z.ai compatibility/native executor entries from the
   staging copy of the config: native executors win ownership over plugins.
4. Create the placeholder auth file from the README in staging's configured
   auth directory, mode 0600. Use a real entitled key only when separately
   authorized. Check that the native auth UI lists its provider, label, and quota
   support, and that models appear as `zai/glm-5.3` and
   `zai/glm-5.3-flash` (plus any optional host auth prefix).
5. Mock acceptance needs no subscription: the hostcompat integration suite loads
   the actual v8.0.48 host, drives all capabilities against `httptest`, verifies
   real streaming and token accounting, and exercises native two-auth failover.
   The test-only loopback upstream setting is intentionally absent in releases.
6. **Stop here without credentials and explicit live-test authorization.** When
   available, first verify the non-inference quota display, regional auth form,
   current credit shape, subscription label, and reset times. A quota success is
   not by itself proof that a key cannot incur PAYG charges. Only after billing
   class and tool scope are confirmed should explicitly authorized minimal
   non-stream/stream/tool-call inference be tried on staging. Do not deliberately
   exhaust a plan or provoke a fair-use restriction; use mocks for those cases.
7. Record staging acceptance and prove rollback before scheduling cutover. Retain
   the README's pending-live items until actual evidence resolves each one.

## 3. Per-provider production cutover, only after staging acceptance

1. Back up the current production config/auth/plugin state privately and choose
   the deployment's ordinary change window. This checklist does not authorize
   changing production or restarting any process by itself.
2. Install the exact verified release through the native store. Before enabling
   it, move conflicting old Z.ai binaries out of discovery and remove only
   overlapping Z.ai compatibility/executor entries. Do not leave two binaries
   with the same plugin ID loaded. If a multi-provider quota/scheduler plugin is
   present, remove/disable its **Z.ai block only**, not unrelated providers.
3. Enable `plugins.configs.zai-coding-plan.enabled`, retain the host's native
   `routing.strategy`, and add the private auth file(s). No scheduler, host timer,
   quota-percent router, browser-cookie scraper, or extra polling service is
   needed.
4. Confirm registration, auth persistence, model namespacing, quota UI, normal
   request errors, and authorized usage. Monitor for unexpected 401/429s and
   billing. A transient 1305 remains subject to the v8.0.48 generic 429 cooldown
   floor described in the README; do not compensate with another scheduler.
5. Update clients from any old unprefixed model names to the fixed `zai/` IDs.
   Remove old Z.ai-only config only after cutover acceptance; keep rollback
   material until the deployment's normal retention period has elapsed.

## 4. Rollback

Disable `plugins.configs.zai-coding-plan.enabled` through native configuration,
restore the previous Z.ai compatibility settings/auths/binaries from the private
backup, and remove the new auth files from the active directory if returning to
compatibility mode. Preserve unrelated provider state and avoid duplicate IDs.
On Linux the host may keep Go C-shared images resident after logical disable:
use the deployment's authorized restart procedure when a complete unload is
required. Never replace an in-use `.so` to simulate an unload. Confirm the old
model routes and normal service after rollback.

## Host-contract references

These paths refer to the pinned public CLIProxyAPI `v8.0.48` source, not a local
fork: `config.example.yaml` (plugin configuration and `store-sources`),
`internal/pluginstore/registry.go` (registry schema),
`internal/pluginstore/github.go` (asset/checksum naming),
`internal/pluginhost/adapters_executors.go` (executor ownership),
`internal/pluginhost/loader_unix.go` (resident shared libraries), and
`sdk/cliproxy/auth/conductor_cooldown.go` (native reactive cooldowns).
