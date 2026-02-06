---
agent: fd-security
tier: 1
issues:
  - id: P0-1
    severity: P0
    section: "Seed Input Validation (Decision Log)"
    title: "Seed validation allows control characters and null bytes that propagate into FNV-1a hashing and URL fragments"
  - id: P1-1
    severity: P1
    section: "Pack Format v2 (integrity checksums)"
    title: "Plan upgrades hash from blake3 to sha256 for pack integrity but specifies no signature or authenticated hash -- any CDN or cache poisoning replaces both pack and checksum"
  - id: P1-2
    severity: P1
    section: "Runtime Validation (Ajv validation, cache poisoning)"
    title: "IndexedDB cache keyed on pack checksum is self-referential -- a poisoned pack with a matching meta checksum passes validation"
  - id: P2-1
    severity: P2
    section: "FNV-1a Hashing (collision risk)"
    title: "64-bit FNV-1a for stat key IDs has ~1-in-4-billion collision risk per pair but plan ships lookup tables without specifying collision detection at pack-build time"
  - id: P2-2
    severity: P2
    section: "Seed Input Validation (Decision Log)"
    title: "Allowed punctuation set (hyphen, underscore, period) does not match existing implementation which accepts all ASCII"
  - id: P2-3
    severity: P2
    section: "Runtime Validation (Ajv validation)"
    title: "Ajv dynamic import with no integrity check creates a supply-chain injection point for the schema validator itself"
  - id: P3-1
    severity: P3
    section: "DF Stats Model (Conflicts)"
    title: "Conflict penalty arithmetic (2% per conflict, capped at 10%) has no bounds check specified for the z-score threshold input"
improvements:
  - id: IMP-1
    title: "Add HMAC or Ed25519 signature to pack meta for authenticated integrity"
    section: "Pack Format v2"
  - id: IMP-2
    title: "Build-time collision detection for FNV-1a stat key IDs with CI enforcement"
    section: "FNV-1a Hashing"
  - id: IMP-3
    title: "Specify CSP and SRI for Ajv dynamic import"
    section: "Runtime Validation"
  - id: IMP-4
    title: "Add explicit allowed-character regex to plan, reconcile with existing ASCII-only implementation"
    section: "Seed Input Validation"
  - id: IMP-5
    title: "IndexedDB cache should validate against an independent trust anchor, not just pack-embedded checksum"
    section: "Runtime Validation"
  - id: IMP-6
    title: "Rate limit should specify client-side enforcement mechanism and note it is not a security control"
    section: "Seed Input Validation"
verdict: needs-changes
---

# Security Review: DF-Style Agent Generation Redesign (WASM)

**Reviewer:** Security Reviewer (Claude Opus 4.6)
**Date:** 2026-02-06
**Plan under review:** DF-Style Agent Generation Redesign (WASM)
**Focus areas:** Seed input validation, Pack Format v2 integrity, Runtime Validation (Ajv, cache poisoning), FNV-1a collision risk, IndexedDB caching

---

## Summary

This plan redesigns the agent generation system in Shadow Workipedia from TypeScript to Rust/WASM, introducing DF-style attributes, a binary pack format, deterministic seeded generation, and browser-side caching. The security surface is **narrow but real**: this is a public-facing static website (deployed on Vercel) where seeds arrive from URL fragments (`#/agents/{seed}`) and the pack file is fetched over HTTPS from a CDN. There is no server-side processing, no authentication, and no persistent user data beyond sessionStorage. The primary risks are (1) seed input that bypasses validation and produces unexpected behavior in the WASM module, (2) pack integrity relying on a self-referential checksum that offers no protection against CDN/cache poisoning, and (3) IndexedDB caching that trusts data it should not. The plan's seed validation section is close to correct but has gaps relative to the existing implementation and omits control characters. The FNV-1a collision risk is manageable but needs build-time detection.

---

## Threat Model Context

### What this project actually is

Shadow Workipedia is a **public static website** deployed to Vercel. It has:

- No server-side processing (fully static: `dist/` served by CDN)
- No authentication or user accounts
- No database or API backend
- Agent generation runs entirely in the browser (currently TypeScript, planned Rust/WASM)
- User input limited to: seed strings (text input + URL fragment), filter selections, year/country parameters
- State stored in `sessionStorage` (roster of recent agents) and `localStorage` (UI preferences)
- Data files (`agent-data.pack`, `agent-priors.v1.json`, `agent-vocab.v1.json`) served as static assets from `public/`

The existing implementation at `/root/projects/shadow-work/shadow-workipedia/src/agentsView.ts` already handles seed input with `normalizeSeedInput()` (line 191) and `getSeedError()` (line 195), and renders seeds via `escapeHtml()` (line 525 of `agentsView.ts`). The pack is currently v1, using blake3 for integrity hashing (confirmed in `/root/projects/shadow-work/shadow-workipedia/wasm/agent-pack/src/main.rs`, line 28).

### What the plan changes about the attack surface

1. **New binary pack format (v2):** Adds gzip-compressed blocks, lookup tables, percentile tables. Introduces a new deserialization surface in the WASM module.
2. **New seed hashing scheme:** Replaces the existing `fnv1a32(seed)` + PCG32 approach with `fnv1a32(agent_seed + stat_key)` for per-stat sub-seeds and `fnv1a32(agent_seed + "factor:" + group_key)` for factor draws.
3. **Browser-side schema validation via Ajv:** Dynamically imports Ajv to validate `df-priors` against a JSON Schema. This is a new code dependency loaded at runtime.
4. **IndexedDB caching:** Decompressed priors cached in IndexedDB, keyed by pack checksum + schema version. This is a new persistence surface.
5. **Offline percentile tables:** Pre-computed population percentiles shipped in the pack for rarity readouts.

### Primary threat actors

1. **Malicious URL sharer (realistic):** Someone shares a crafted URL like `https://site.example/#/agents/<payload>` on social media or in a message. The seed is extracted from the URL fragment and processed.
2. **CDN/hosting compromise (low probability, high impact):** If the Vercel deployment is compromised or a supply-chain attack modifies `agent-data.v2.pack`, the WASM module processes attacker-controlled binary data.
3. **XSS via seed reflection (mitigated in current code):** The seed appears in the HTML input field. The current implementation uses `escapeHtml()` consistently. The plan must preserve this.

---

## Section-by-Section Review

### Seed Input Validation (Decision Log)

The plan specifies:
- Restrict to ASCII; trim and reject non-ASCII
- Max 64 characters
- Allow spaces (trim leading/trailing, collapse repeated internal spaces)
- Allow hyphen, underscore, period
- Reject emoji (non-ASCII)
- Normalize tabs to single spaces
- Inline error + disable Generate until fixed
- 1/sec rate limit on generation

**What the existing code does** (at `/root/projects/shadow-work/shadow-workipedia/src/agentsView.ts`):

```typescript
// Line 191
function normalizeSeedInput(raw: string): string {
  return raw.replace(/\t+/g, ' ').replace(/ {2,}/g, ' ').trim();
}

// Line 195
function getSeedError(seed: string): string | null {
  if (!/^[\x00-\x7F]*$/.test(seed)) return 'ASCII only';
  if (seed.length > MAX_SEED_LENGTH) return 'Max 64 chars.';
  return null;
}
```

The existing implementation accepts **all ASCII characters** (bytes 0x00-0x7F), including control characters (0x00-0x1F), DEL (0x7F), and all printable punctuation. The plan says "allow hyphen, underscore, period" for punctuation, which is a *stricter* allowlist than what currently ships. This discrepancy is important because:

1. **Existing URLs in the wild** may contain seeds with characters the plan would reject (e.g., `@`, `#`, `!`, `+`). Tightening validation breaks backward compatibility for shared links.
2. **The existing code does not reject null bytes (0x00)** or other control characters. `\x00` in a seed string would propagate into `fnv1a32()` and produce valid but unexpected hashes. This is not a security vulnerability in the current architecture (the output is just a different agent), but it creates confusion and non-reproducibility if different platforms handle null bytes differently in URL fragments.

**Seeds arrive from URL fragments** via `readSeedFromHash()` (line 154 of `agentsView.ts`):

```typescript
function readSeedFromHash(): string | null {
  const hash = window.location.hash;
  const m = hash.match(/^#\/agents\/([^?]+)(?:\?.*)?$/);
  if (!m) return null;
  try {
    return decodeURIComponent(m[1] ?? '').trim() || null;
  } catch {
    return (m[1] ?? '').trim() || null;
  }
}
```

The `decodeURIComponent` fallback on failure returns the raw percent-encoded string, which means a malformed percent sequence like `%ZZ` results in the literal string `%ZZ` being used as a seed. This is harmless but worth noting for determinism -- the same URL could produce different seeds depending on whether `decodeURIComponent` succeeds.

### Pack Format v2 (Integrity Checksums)

The plan specifies:
- `agent-data.pack.meta.json` includes `sha256` for df-priors, `percentiles_checksum`
- Pack contains gzip-compressed blocks

**What the existing code does** (at `/root/projects/shadow-work/shadow-workipedia/wasm/agent-pack/src/main.rs`):

```rust
// Line 28
let hash = blake3::hash(&bytes).to_hex().to_string();
```

The existing pack uses blake3, not sha256. The plan switches to sha256, which is fine cryptographically but is a breaking change for the meta format. More importantly:

The current meta format (`/root/projects/shadow-work/shadow-workipedia/public/agent-data.pack.meta.json`):

```json
{
  "version": 1,
  "hash": "b33bdd73...",
  "bytes": 5448613
}
```

The hash covers the entire serialized pack. The plan's v2 meta adds `sha256` for df-priors specifically and `percentiles_checksum`, but does not specify whether there is still a whole-pack hash. If the meta only checksums individual components, an attacker who replaces the pack could swap out the lookup tables or percentile tables without detection.

**The fundamental issue:** Both the pack and the meta are served from the same CDN origin. An attacker who can modify one can modify the other. The checksum in meta provides integrity against *accidental corruption* (bit flips, truncated downloads) but not against *intentional modification*. This is the correct threat model for a static site -- you trust Vercel's TLS and deployment pipeline. But the plan should be explicit that the checksums are for corruption detection, not for tamper resistance.

### Runtime Validation (Ajv, Cache Poisoning)

The plan specifies:
- `df-priors` validated in browser using Ajv (dynamic import)
- Validated once per pack checksum + schema version (cached in IndexedDB)
- Fail fast with inline error UI + "copy debug info" button
- Decompress gzip in JS before WASM usage
- Cache decompressed `df-priors` in IndexedDB
- Cache generated agents in JS memory only (no IndexedDB)

**Ajv dynamic import:** The plan does not specify where Ajv is loaded from (CDN, bundled, npm). If loaded from a CDN (e.g., `import('https://cdn.jsdelivr.net/npm/ajv/...')`), this is a supply-chain vector. If bundled into the Vite build, it adds to bundle size but is safe. The plan should specify.

**IndexedDB cache keying:** The cache key is `pack checksum + schema version`. The pack checksum comes from the pack meta, which is served alongside the pack. If both are replaced by a CDN compromise, the new pack has a new checksum, and the cache key changes, so the poisoned data is cached under a "valid" new key. The Ajv validation would still run against the schema, but if the schema is also bundled/served from the same origin, the attacker controls the schema too. This is not a practical attack given Vercel's security model, but the plan should acknowledge that validation provides defense against data corruption, not against origin compromise.

**Gzip decompression in JS:** The plan says "Decompress gzip in JS before WASM usage." This means the decompressed data passes through JS before entering WASM. If the gzip stream is malformed (e.g., a gzip bomb -- small compressed, huge decompressed), the browser's `DecompressionStream` or manual gunzip will expand it in JS memory. The plan should specify a maximum decompressed size to prevent memory exhaustion.

### FNV-1a Hashing (Collision Risk)

The plan specifies:
- Stat keys prefixed: `attr_*`, `facet_*`, `value_*`, `need_*` (single shared namespace)
- IDs are 64-bit FNV-1a hashes computed at pack time
- Lookup tables ship in pack

**Current implementation** (at `/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/utils.rs`):

```rust
// Line 67
pub fn fnv1a32(input: &str) -> u32 {
    let mut hash: u32 = 0x811c9dc5;
    for b in input.as_bytes() {
        hash ^= *b as u32;
        hash = hash.wrapping_mul(0x01000193);
    }
    hash
}
```

The existing code uses **32-bit** FNV-1a. The plan upgrades to **64-bit** FNV-1a for stat key IDs. This is a significant improvement -- 32-bit FNV-1a has birthday-problem collisions at ~77,000 keys (50% probability), while 64-bit reaches that threshold at ~5 billion keys. For a stat namespace of likely hundreds to low thousands of keys, 64-bit FNV-1a is more than sufficient.

However, the plan also uses FNV-1a for **seed sub-seeding**: `fnv1a32(agent_seed + stat_key)`. If this remains 32-bit (the plan says `fnv1a32`), then two different `(seed, stat_key)` combinations could collide, producing identical stat values for different agents on different stats. With ~100 stats per agent, the collision probability per agent is ~100^2 / (2 * 2^32) = ~0.0001%, which is negligible. But this should be documented.

**The real risk** is in the lookup tables: if two stat keys collide to the same 64-bit hash at pack-build time, one overwrites the other in the lookup table. The plan does not specify collision detection during the pack build step. Since the pack builder is a build-time tool (not user-facing), a collision would silently produce incorrect stat definitions that persist until someone notices wrong agent output.

### IndexedDB Caching (Data Integrity)

The plan specifies:
- Cache decompressed `df-priors` in IndexedDB
- Agent cache key: seed + pack checksum + generator version; cap TBD
- Agent cache in JS memory only (no IndexedDB)

**Current implementation:** The existing code uses `sessionStorage` for roster data (at `/root/projects/shadow-work/shadow-workipedia/src/agentsView/rosterStorage.ts`). The plan adds IndexedDB for priors, which is a durability upgrade (survives tab close). This is low-risk. IndexedDB is same-origin isolated by the browser.

The "cap TBD" for agent memory cache needs a concrete number before implementation. An unbounded cache of generated agents (each likely 10-50KB of JSON) could grow indefinitely in a long-running tab. With the 1/sec rate limit, a user could generate ~3,600 agents per hour, consuming 36-180MB. This is a denial-of-self scenario, not a security issue, but it should have a bound.

---

## Issues Found

### P0-1: Seed validation allows control characters and null bytes (P0)

**Location:** Seed Input Validation (Decision Log) and the existing `getSeedError()` at `/root/projects/shadow-work/shadow-workipedia/src/agentsView.ts`, line 195

**Threat:** The existing regex `!/^[\x00-\x7F]*$/` accepts all ASCII including null (0x00), bell (0x07), backspace (0x08), escape (0x1B), DEL (0x7F), and other control characters. These propagate into `fnv1a32()` and produce valid hashes, so generation works, but:

1. **Null bytes in seed strings** cause different behavior across platforms. JavaScript handles `\x00` in strings; Rust's `&str` also allows it (it is valid UTF-8). But when the seed is serialized to URLs, JSON, or displayed in UIs, null bytes may be silently truncated. This means the same seed could produce different agents depending on where it is entered.
2. **Escape sequences (0x1B)** in seeds could interact with terminal-based rendering if agent data is ever exported or logged (not a current concern for the web UI, but relevant if the plan's traceability features expand).
3. **The plan says "restrict to ASCII; reject non-ASCII"** but does not explicitly address control characters within ASCII. This is the gap.

**Likelihood:** Medium. Seeds come from URL fragments. A user typing in the input box cannot easily enter null bytes. But a crafted URL like `#/agents/test%00evil` passes through `decodeURIComponent` and produces the string `test\x00evil`, which passes the current `getSeedError()` check.

**Mitigation:** Change the validation regex from `[\x00-\x7F]` to `[\x20-\x7E]` (printable ASCII only). This excludes all control characters (0x00-0x1F) and DEL (0x7F) while allowing all printable characters including the punctuation the plan discusses. Add this explicit regex to the plan. The current implementation in `agentsView.ts` line 196 should be updated to match:

```typescript
if (!/^[\x20-\x7E]*$/.test(seed)) return 'Printable ASCII only';
```

### P1-1: Pack integrity checksum offers no tamper resistance (P1)

**Location:** Pack Format v2, `agent-data.pack.meta.json`

**Threat:** The pack and its meta file are served from the same CDN origin. An attacker who compromises the Vercel deployment (or performs a supply-chain attack on the build pipeline) can replace both `agent-data.v2.pack` and `agent-data.pack.meta.json` simultaneously. The sha256 checksum in meta would match the replaced pack. The Ajv validation would pass if the replacement conforms to the JSON Schema (which is also served from the same origin or bundled). Result: agents generated from poisoned statistical priors.

**Likelihood:** Low. Vercel deployments are protected by their own security model (git-based, immutable deploys). But the plan introduces `pack_version`, `schema_version`, `generator_version`, `sha256`, `percentiles_checksum` as if they provide integrity guarantees. They provide **corruption detection** only.

**Mitigation:** The plan should explicitly state that the checksums are for **accidental corruption detection** (truncated downloads, CDN bit flips), not for tamper resistance. If tamper resistance is desired in the future (e.g., if the pack is ever loaded from a third-party CDN or user-provided URL), add an Ed25519 signature in meta, with the public key embedded in the WASM binary. For now, a comment in the meta schema documenting the trust model is sufficient.

### P1-2: IndexedDB cache trusts self-referential checksum (P1)

**Location:** Runtime Validation & Caching

**Threat:** The plan caches decompressed priors in IndexedDB keyed by `pack checksum + schema version`. The checksum comes from the same network fetch as the pack. If the fetch is intercepted (MITM on a network without HSTS, or compromised CDN), the attacker provides a matching checksum for their modified pack. The browser caches the poisoned data in IndexedDB. On subsequent page loads, the cached poisoned data is used without re-validation (since the checksum matches).

This is worse than a one-time compromise because IndexedDB persists across sessions. A single poisoned fetch poisons all future sessions until the cache is cleared or the pack version changes.

**Likelihood:** Low. HTTPS with HSTS (Vercel enforces this) makes MITM impractical. CDN compromise is the realistic vector, and it has the same persistence problem.

**Mitigation:** Add a `max-age` or TTL to the IndexedDB cache entry (e.g., 7 days). After TTL expiry, re-fetch and re-validate even if the checksum matches. This limits the persistence window of a cache poisoning attack. Additionally, if the `generator_version` or `schema_version` in the meta changes, invalidate the cache unconditionally.

### P2-1: No build-time collision detection for FNV-1a stat key IDs (P2)

**Location:** Data Sources & Formats, Keys & IDs

**Threat:** The plan computes 64-bit FNV-1a hashes of stat keys (`attr_*`, `facet_*`, `value_*`, `need_*`) at pack-build time and ships them in lookup tables. If two stat keys produce the same hash, one silently overwrites the other in the lookup table. The WASM module would then map one stat key to another stat's definition. This is a data integrity error that produces subtly wrong agent generation.

**Likelihood:** Low for 64-bit FNV-1a with hundreds of keys. The birthday bound for 50% collision probability at 64 bits is ~5 billion. But FNV-1a is not a cryptographic hash and has known weakness to crafted inputs. For a fixed, developer-controlled set of stat keys, this is not a concern. The risk is that someone adds a key in the future without checking for collisions.

**Mitigation:** Add a collision check to the pack builder. After computing all stat key hashes, verify uniqueness. If a collision is found, fail the build with an error message identifying the colliding keys. This is a one-time addition to the build tool (analogous to the existing `agent-pack` binary at `/root/projects/shadow-work/shadow-workipedia/wasm/agent-pack/src/main.rs`). Add a CI test that runs the collision check.

### P2-2: Allowed punctuation set inconsistent with existing implementation (P2)

**Location:** Seed Input Validation (Decision Log)

**Threat:** The plan says "Seed punctuation: allow hyphen, underscore, period." The existing implementation allows all ASCII. If the plan's stricter allowlist is implemented, existing shared URLs containing seeds with characters like `@`, `!`, `+`, `=`, or `~` will start showing validation errors. This is a breaking change for any seeds already in circulation (e.g., shared on social media, bookmarked, or stored in roster data from older sessions).

**Likelihood:** Medium. The agent generation feature is live (the `/wasm/` route exists in `vercel.json`). Users have been generating agents with the current permissive validation. Tightening it breaks their bookmarks.

**Mitigation:** Either (a) keep the full printable-ASCII allowlist (`[\x20-\x7E]`) to maintain backward compatibility, or (b) implement the restricted set but add a migration path: if a seed from a URL or roster fails the new validation, attempt generation anyway with a warning banner ("This seed uses characters that may not be supported in future versions"). Document the decision in the plan.

### P2-3: Ajv dynamic import with no integrity specification (P2)

**Location:** Runtime Validation & Caching

**Threat:** The plan says "df-priors validated in browser using Ajv (dynamic import)." If Ajv is loaded via a dynamic `import()` from a CDN URL, the CDN could serve a modified Ajv that always returns `valid: true`, bypassing schema validation entirely. If Ajv is bundled into the Vite build, this is not a concern.

**Likelihood:** Low if bundled; medium if CDN-loaded. The plan does not specify which approach.

**Mitigation:** Specify in the plan that Ajv must be bundled into the Vite build (tree-shaken, versioned in `package.json`). If CDN loading is preferred for bundle size, use Subresource Integrity (SRI) on the import or a Content Security Policy (CSP) `script-src` directive that restricts to the exact CDN path + hash.

### P3-1: No bounds check specified for z-score threshold in conflict detection (P3)

**Location:** DF Stats Model, Conflicts

**Threat:** The plan says "High detection uses z-score threshold +0.7 sigma within facet/value groups." If the z-score calculation receives a standard deviation of zero (e.g., all agents in a culture have identical facet values due to tight priors), the z-score is undefined (division by zero). Depending on the Rust implementation, this produces `NaN` or `Infinity`, which would propagate through the conflict penalty arithmetic.

**Likelihood:** Low. Priors with zero standard deviation would be unusual but possible for extreme microculture overrides.

**Mitigation:** The plan should specify that z-score calculation handles `stddev = 0` explicitly (treat as "not high" or use a minimum stddev floor). The existing codebase already has `clamp01_default()` and `clamp_fixed_01k()` functions in `/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/utils.rs` that handle `NaN`/`Infinity` -- the same pattern should be applied to z-score calculations.

---

## Improvements Suggested

### IMP-1: Add authenticated integrity to pack meta (HIGH VALUE)

The pack meta should include either:
- An Ed25519 signature of the pack bytes, with the public key embedded in the WASM binary (high assurance), or
- At minimum, an explicit comment in the meta schema documenting that checksums are for corruption detection only, not tamper resistance.

This prevents the plan from implying a security guarantee that the checksums do not actually provide.

### IMP-2: Build-time FNV-1a collision detection (MEDIUM VALUE)

Add to the pack builder (currently `/root/projects/shadow-work/shadow-workipedia/wasm/agent-pack/src/main.rs`):

```rust
let mut seen: HashMap<u64, String> = HashMap::new();
for key in stat_keys {
    let hash = fnv1a64(&key);
    if let Some(existing) = seen.insert(hash, key.clone()) {
        panic!("FNV-1a collision: '{}' and '{}' both hash to {:#x}", existing, key, hash);
    }
}
```

Add a CI step that runs the pack builder and asserts no collision.

### IMP-3: Specify Ajv loading strategy with integrity (MEDIUM VALUE)

Add to the Runtime Validation section: "Ajv is bundled into the Vite production build via `import('ajv')` (not loaded from an external CDN). The bundled version is pinned in `package.json` and updated through the normal dependency update process."

If dynamic CDN import is chosen instead, specify the SRI hash.

### IMP-4: Reconcile seed validation with existing implementation (MEDIUM VALUE)

The plan's Decision Log should include a "Migration" entry:

- **Current behavior:** All ASCII accepted (bytes 0x00-0x7F)
- **New behavior:** Printable ASCII only (bytes 0x20-0x7E), with hyphen/underscore/period explicitly called out
- **Migration:** Seeds from URL fragments that fail the new validation are still processed but shown with a deprecation warning. After 6 months, hard-reject.
- **Explicit regex:** `^[a-zA-Z0-9 ._-]{1,64}$` (if the strict allowlist is chosen) or `^[\x20-\x7E]{1,64}$` (if permissive)

### IMP-5: Add TTL to IndexedDB cache (LOW-MEDIUM VALUE)

The plan should specify: "IndexedDB cached entries expire after 7 days (configurable). On expiry, re-fetch and re-validate the pack. Cache entries are also invalidated when `generator_version` or `schema_version` changes."

### IMP-6: Document rate limit as UX control, not security control (LOW VALUE)

The plan specifies "1/sec rate limit on generation." This is implemented client-side (there is no server). A user can bypass it by opening the browser console. The plan should note that this rate limit is a UX safeguard (prevents accidental rapid clicks) and not a security control. If abuse prevention is needed (e.g., automated scraping of agent generation), it requires server-side enforcement, which is out of scope for a static site.

---

## Overall Assessment

**Real risk level: LOW**

This is a static website with no server-side processing, no authentication, no persistent user data, and no elevated privileges. The WASM module processes user-provided seeds and static data files, producing deterministic JSON output that is rendered in the browser with proper HTML escaping (confirmed: `escapeHtml()` is used consistently in `/root/projects/shadow-work/shadow-workipedia/src/agentsView/renderAgent.ts` and `/root/projects/shadow-work/shadow-workipedia/src/agentsView.ts`).

The worst realistic outcome of any finding above is:
- **P0-1 (control characters):** Inconsistent agent generation across platforms for seeds containing null bytes. No code execution, no data exfiltration.
- **P1-1/P1-2 (pack integrity):** If Vercel is compromised, agents are generated from modified priors. This affects gameplay immersion, not user safety.

**Must-fix items:**

1. **P0-1:** Tighten seed validation to printable ASCII (`[\x20-\x7E]`). This is a one-line regex change that prevents the only input-boundary issue.
2. **P2-1:** Add build-time FNV-1a collision detection. This is a low-effort addition to the pack builder that prevents silent data corruption.

**Nice-to-have hardening:**

3. **P1-1/P1-2:** Document that pack checksums are for corruption detection, not tamper resistance. Add IndexedDB cache TTL.
4. **P2-2:** Decide on the seed character allowlist and document the migration path.
5. **P2-3:** Specify that Ajv is bundled, not CDN-loaded.
6. **IMP-6:** Document the rate limit's nature as UX, not security.

**What I did NOT flag** (and why):
- XSS via seed reflection: The existing `escapeHtml()` implementation is correct and consistently used. The plan does not change the rendering path.
- WASM memory safety: Rust's memory model prevents buffer overflows in the WASM module. The `bincode` deserialization of the pack is safe (bounded by the pack size, which is a static asset).
- IndexedDB data leakage: IndexedDB is same-origin isolated. No cross-origin access is possible.
- Denial of service via large seeds: The 64-character limit is enforced before FNV-1a hashing. FNV-1a on 64 bytes is effectively instant.
- PCG32 PRNG predictability: PCG32 is not cryptographic, but agent generation does not require cryptographic randomness. Predictability is a feature (deterministic seeds).
