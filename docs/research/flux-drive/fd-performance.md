---
agent: fd-performance
tier: 1
issues:
  - id: P0-1
    severity: P0
    section: "Percentile Tables"
    title: "Percentile tables will balloon pack size by 10-40x with no lazy-load strategy"
  - id: P1-1
    severity: P1
    section: "Runtime Validation & Caching"
    title: "Ajv dynamic import for schema validation blocks first agent generation"
  - id: P1-2
    severity: P1
    section: "Pack Format v2"
    title: "JS-side gzip decompression of 5MB+ pack on main thread will freeze UI"
  - id: P1-3
    severity: P1
    section: "Priors Sampling"
    title: "statrs dependency will massively inflate WASM binary size"
  - id: P1-4
    severity: P1
    section: "Runtime Validation & Caching"
    title: "IndexedDB caching adds complexity with no measurable benefit over HTTP cache"
  - id: P2-1
    severity: P2
    section: "Derived Outputs"
    title: "51-point curve lookup arrays are wasteful when 7-keypoint interpolation suffices"
  - id: P2-2
    severity: P2
    section: "DF Stats Model"
    title: "64-bit FNV-1a IDs are overkill; 32-bit FNV-1a already used in codebase"
  - id: P2-3
    severity: P2
    section: "Correlations"
    title: "Factor sampling adds a second truncated-normal pass per group with minimal diversity payoff"
improvements:
  - id: IMP-1
    title: "Compute percentiles lazily in WASM on first access instead of shipping precomputed tables"
    section: "Percentile Tables"
  - id: IMP-2
    title: "Skip Ajv validation entirely; use Rust-side serde deserialization as the validation gate"
    section: "Runtime Validation & Caching"
  - id: IMP-3
    title: "Decompress pack in the Worker, not on the main thread"
    section: "Pack Format v2"
  - id: IMP-4
    title: "Use the existing Pcg32-based Box-Muller transform instead of statrs for truncated normals"
    section: "Priors Sampling"
  - id: IMP-5
    title: "Replace IndexedDB caching with standard HTTP caching (Cache-Control + ETag on pack file)"
    section: "Runtime Validation & Caching"
  - id: IMP-6
    title: "Define all curves as 7-keypoint parametric; drop explicit 51-point arrays"
    section: "Derived Outputs"
verdict: needs-changes
---

# Performance Review: DF-Style Agent Generation Redesign (WASM)

**Reviewer:** Performance Oracle
**Plan:** DF-Style Agent Generation Redesign (WASM) -- dated 2026-01-15
**Date:** 2026-02-06
**Focus:** Percentile Tables, Runtime Validation & Caching, Priors Sampling, Derived Outputs, Pack Format, Worker thread model

---

## Performance Profile

- **Application type:** Interactive browser-based agent generator. The Agents page (`#/agents`) in shadow-workipedia lets a user type a seed, click Generate, and see a fully rendered agent profile. Generation happens synchronously on the main thread today.
- **Where performance matters most:** Time from user clicking "Generate" to the agent profile rendering in the DOM. The current TS implementation runs `generateAgent()` synchronously on the main thread and is fast enough that the perf measurement system records it in milliseconds. The plan proposes moving core generation to WASM running in a Worker -- the user-visible metric is the round-trip: main thread posts seed to Worker, Worker runs WASM generation, Worker posts result back, main thread renders. Any jank during this round-trip (frozen UI, blank state, loading spinner) is a regression.
- **Secondary perf concern:** Initial page load. Currently the Agents page fetches 3 JSON files in parallel (~5.3MB total: 4.9MB priors + 325KB vocab + 67KB country map), parses them, then generates the first agent. The plan replaces these with a single pack file plus gzip decompression plus Ajv validation plus IndexedDB caching -- all of which add steps before the first agent can appear.
- **Known constraints:**
  - Current pack file is 5.4MB (bincode-serialized, uncompressed). The plan proposes pack v2 with gzip-compressed blocks, lookup tables, percentile tables, and schema.
  - No Worker infrastructure exists today. `agentsView.ts` calls `generateAgent()` synchronously.
  - The existing perf measurement system (`measureSpan` / `measureAsyncSpan`) is adequate for tracking regressions.
  - The 1/sec rate limit on seed changes is already specified in the plan and is appropriate.
  - The plan says "Core identity generation runs in a Worker; non-core facets lazy-generate on demand" -- this is the right instinct but introduces complexity around what "core" vs "non-core" means and how the boundary is managed.

---

### Summary

The plan's overall approach -- Rust/WASM for the generation pipeline, data-driven JSON curves, deterministic sub-seeds -- is sound and will likely improve generation speed once WASM is compiled. However, five specific decisions will cause measurable performance regressions or unnecessary complexity:

1. **Percentile tables are the biggest problem.** The plan ships precomputed percentile arrays (1001 integers per output per culture) in the pack. With ~100+ DF stats and derived outputs across multiple culture profiles, this adds tens of megabytes to the pack. The current pack is 5.4MB. This will balloon it to 50-200MB depending on the number of culture profiles, making initial load unacceptable.

2. **Ajv schema validation on the critical path** adds a dynamic import, schema compilation, and full JSON traversal before the first agent can generate. This is wasted work -- the Rust serde deserialization in WASM already validates structure.

3. **Gzip decompression in JS before WASM** means the main thread (or Worker) must decompress 5MB+ before the WASM module can even begin. The plan says "Decompress gzip in JS before WASM usage" without specifying which thread.

4. **The `statrs` crate** for truncated normals will add 200-500KB to the WASM binary. The existing codebase already has a working Pcg32 RNG (`/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/rng.rs`) that can do Box-Muller in ~10 lines.

5. **IndexedDB caching of decompressed priors** adds async I/O and error handling complexity for data that a standard HTTP `Cache-Control` header handles automatically.

---

## Section-by-Section Review

### Percentile Tables

The plan says:

> Computed offline in packer using full pipeline, 5,000 agents per culture profile. Stored in pack as raw 0-100 integer arrays (size 1001 per output), keyed by `culture_key`.

Let me estimate the data volume. The plan defines:
- **Attributes**: grouped into 5 groups (physical, cognitive, social-empathic, creative, resilience). DF has ~30+ attributes.
- **Facets**: DF has ~80+ facets across emotional, social, discipline, cognition, risk, aesthetics, morality, etc.
- **Values/Needs**: separate taxonomy, likely 20-40 items.
- **Derived Outputs**: aptitudes + traits + skills. The current TS codebase has 13 aptitudes (`/root/projects/shadow-work/shadow-workipedia/src/agent/facets/aptitudes.ts`), 10+ traits, 10+ skills.

Conservative estimate: 150 total outputs needing percentile tables.

Per output: 1001 u8 values = 1001 bytes.
Per culture: 150 * 1001 = ~150KB.
The priors file already has country-level buckets. If there are 20 culture profiles, that is 20 * 150KB = ~3MB of percentile data. Manageable.

But the plan says percentiles are keyed by `culture_key`, and the existing priors file (`/root/projects/shadow-work/shadow-workipedia/public/agent-priors.v1.json`, 4.9MB) contains per-country, per-year-bucket data. If percentiles must be computed per-country-per-bucket (not just per-culture), the combinatorics explode. The existing countries JSON (`/root/projects/shadow-work/shadow-workipedia/public/shadow-country-map.json`) has entries for ~240 countries. Even with 1 bucket each: 240 * 150KB = ~36MB of percentile data.

The plan is ambiguous: it says `culture_key` but does not define how many distinct culture keys exist. If it means the ~7 shadow cultures (Hesper, Aram, Mero, Solis-East, Solis-South, Athar-West, Pelag, Global from `/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/utils.rs:253-314`), the overhead is ~1MB total, which is fine. If it means per-country, it is 36MB+.

Additionally, the plan says percentiles cover "all DF stats and derived outputs." The 5,000-agent sample per culture is sufficient for p1-p99 accuracy but wastes pack space for the p0 and p100 buckets (which are just min/max of the distribution).

### Runtime Validation & Caching

The plan specifies:

> `df-priors` validated in browser using Ajv (dynamic import), once per pack checksum + schema version (cached in IndexedDB). Fail fast with inline Agents-page error UI.

This introduces three problems:

1. **Ajv dynamic import**: Ajv is ~150KB minified. Dynamic importing it at runtime adds a network round-trip and parse step. The plan says "once per pack checksum" but the first load always pays this cost.

2. **IndexedDB for decompressed priors**: IndexedDB writes are async and can take 50-200ms for multi-MB values depending on the browser's storage backend. The read on subsequent loads also has variance (10-100ms). This adds latency to every page load for a caching layer that provides no benefit over the browser's native HTTP cache.

3. **Double validation**: The plan says to validate priors with Ajv in the browser, then pass them to WASM where Rust's `serde` deserializes them again. The serde deserialization IS validation -- if the JSON does not match the expected structure, serde returns an error. Running Ajv first is redundant.

The plan also says:

> Cache decompressed `df-priors` in IndexedDB. Cache generated agents in JS memory only (no IndexedDB). Agent cache key: seed + pack checksum + generator version; cap TBD.

The in-memory agent cache is appropriate. But the "cap TBD" is concerning. The current roster storage (`/root/projects/shadow-work/shadow-workipedia/src/agentsView/rosterStorage.ts`) keeps at most 5 agents in `sessionStorage`. Each generated agent is a large JSON object. Without a cap, memory grows linearly with generations. A cap of 20-50 agents is reasonable.

### Priors Sampling

The plan says:

> Per-stat truncated normal (0-1000) with per-stat min/max clamps. Uses `statrs` and reusable truncated-normal helper in `wasm/agent-gen/src/utils.rs`.

The `statrs` crate is a comprehensive Rust statistics library. It provides `TruncatedNormal` and many other distributions. However, for a WASM target:

- `statrs` depends on `rand`, `rand_distr`, and `special` (for gamma/beta functions). This pulls in significant code that wasm-opt cannot fully eliminate because distribution trait implementations include branch-heavy generic code.
- Estimated WASM binary size increase: 200-500KB after optimization.
- The existing codebase (`/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/rng.rs`) has a compact Pcg32 implementation in 33 lines. A truncated normal using Box-Muller on top of Pcg32 is ~15 lines of code and adds zero binary size.

The plan's deterministic sub-seed approach is sound: `fnv1a32(agent_seed + stat_key)`. This is already implemented in `/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/utils.rs:67-74` as `fnv1a32()` and used by `make_rng()` at line 95-99.

The culture blending approach (weighted mean/stddev/min/max) reuses `mix_culture_env_01k` and `mix_weights_01k` patterns already in `utils.rs`. No performance concerns here -- these are small data operations.

### Derived Outputs (Curve Evaluation)

The plan says:

> Curve templates: 12 total; mixed definition (explicit 51-point arrays or 7-keypoint parametric linear interpolation). Inputs clamped to [0,1000], evaluated as floats.

The 12 curve templates are evaluated per derived output per agent. With ~30 derived outputs, that is 30 curve evaluations per generation. Each evaluation involves:

1. Input blending: `50% raw score + 50% z-score mapped via sigmoid to 0-1000` -- this is ~5 float operations.
2. Curve lookup: for 51-point arrays, this is an array index + linear interpolation between two points -- ~5 float operations. For 7-keypoint parametric, this is a binary search over 7 points + linear interpolation -- ~10 operations.
3. Aggregation: weighted sum of curve outputs from multiple input stats -- linear in number of inputs per output.
4. Logistic soft-cap: one `exp()` call + division.

Total per agent: ~30 outputs * ~10 operations each = ~300 float operations. This is trivially fast in WASM (~1 microsecond). No concerns.

However, shipping both 51-point arrays AND 7-keypoint definitions in the JSON data is unnecessary. The 51-point arrays are 51 floats * 8 bytes = 408 bytes each. With 12 templates, that is ~5KB -- negligible for pack size but conceptually wasteful when linear interpolation over 7 keypoints produces equivalent results.

### Pack Format v2

The plan proposes gzip-compressed blocks inside the pack. The current pack (`/root/projects/shadow-work/shadow-workipedia/public/agent-data.pack`) is 5.4MB of bincode-serialized data containing:
- `vocab_json`: 325KB
- `priors_json`: 4.9MB
- `countries_json`: 67KB

The plan adds:
- Schema JSON
- 8 lookup tables (id->key, key->id, id->label, id->description, id->group, id->tags, id->kind, id->df_token, id->conflicts, group_labels, group_descriptions)
- Percentile tables

With gzip compression, the priors JSON (mostly repetitive country buckets) should compress well -- likely to ~1-1.5MB. The vocab and lookup tables are small. The percentile tables are arrays of integers that compress well. Total compressed pack: likely 2-4MB depending on percentile table count.

The concern is decompression. The plan says "Decompress gzip in JS before WASM usage." If this happens on the main thread, `DecompressionStream` or a manual `pako.inflate()` on 2-4MB of gzip data takes 20-100ms depending on content and device. On a mobile device, this could be 200ms+. This blocks the first agent generation.

If decompression happens in the Worker (which the plan implies by "Core identity generation runs in a Worker"), the main thread stays responsive. But the plan does not explicitly say the Worker handles decompression.

### Worker Thread Model

The plan says:

> Core identity generation runs in a Worker; non-core facets lazy-generate on demand.

The current TS codebase generates everything synchronously in `generateAgent()` (`/root/projects/shadow-work/shadow-workipedia/src/agent/generator/orchestrator.ts`). The generation pipeline is:

```
buildBaseContext -> computeCoreFacets -> computeLifeFacets -> computeMetaFacets -> applyTraumaCopingRituals -> assembleAgent
```

This produces a single `GeneratedAgent` object with all facets populated. The UI (`agentsView.ts` line 633) calls `generateAgent(input)` and immediately uses the full result to render.

The plan's "core vs non-core" split means:
1. Worker generates core identity (DF stats + derived outputs)
2. Main thread receives partial agent
3. Main thread renders summary from core data
4. When user expands dossier sections, non-core facets lazy-generate

This is architecturally sound but requires:
- A clear serialization boundary (the Worker must return JSON or a transferable buffer)
- The main thread must handle the partial -> complete agent transition without flickering
- The existing `renderAgent()` function (`/root/projects/shadow-work/shadow-workipedia/src/agentsView/renderAgent.ts`) expects a complete `GeneratedAgent` and will need refactoring for progressive rendering

The Worker round-trip (postMessage -> WASM execution -> postMessage back) adds ~1-3ms of overhead per generation on modern browsers. WASM execution of the new DF pipeline should be 1-5ms (the current TS pipeline is fast enough to measure in single-digit ms). Total: 2-8ms per generation, well under the 1/sec rate limit.

The plan does not specify how the WASM module is loaded into the Worker. Options:
- `new Worker(new URL('./agent-worker.ts', import.meta.url))` with `wasm-bindgen` + `wasm-pack`
- Manual `WebAssembly.instantiate()` in the Worker
- Using `wasm-bindgen-rayon` for thread pool (overkill for single-agent generation)

The simplest approach is a dedicated Worker that loads the WASM module once, receives generation requests via `postMessage`, and posts results back.

---

## Issues Found

### P0-1: Percentile tables will balloon pack size by 10-40x with no lazy-load strategy

**Location:** Percentile Tables section
**Problem:** The plan says "Percentiles for all DF stats and derived outputs" stored "as raw 0-100 integer arrays (size 1001 per output), keyed by `culture_key`." If `culture_key` maps to per-country granularity (240 countries in the shadow country map at `/root/projects/shadow-work/shadow-workipedia/public/shadow-country-map.json`), with ~150 stats/outputs, that is 240 * 150 * 1001 bytes = ~36MB of raw percentile data before compression. Even with the ~7 shadow cultures, it is 7 * 150 * 1001 = ~1MB, which compresses well but still inflates a 5.4MB pack.

More importantly, percentile lookups are only needed when the user views the dossier and expands a section showing percentile labels. The summary view does not display percentiles. Shipping all percentile data in the pack means every user downloads data that only matters if they open the dossier.

**Impact:** Initial page load increases by seconds on slow connections (3G: ~36MB = 120+ seconds; even 1MB compressed adds 3-4 seconds on 3G). Users who only generate and view summaries download data they never use.

**Fix:** Do not ship percentile tables in the pack. Instead, compute percentiles lazily in the WASM Worker. When the user first opens a dossier section that shows percentiles, the Worker generates 1000 agents for the relevant culture, computes the stat distribution, and caches the resulting percentile array in memory. This takes ~1-5 seconds (1000 agents * 1-5ms each) but only happens once per culture per session, and only when the user actually requests percentile data. Trade-off: 1-5 second delay on first dossier expansion instead of 0ms, but eliminates megabytes from the pack and avoids penalizing all users for a feature most will not use immediately.

Alternatively, if precomputed percentiles are desired, ship them as a separate file (`percentiles.v1.pack`) loaded on demand, not in the main agent data pack. This preserves the fast initial load.

### P1-1: Ajv dynamic import for schema validation blocks first agent generation

**Location:** Runtime Validation & Caching section
**Problem:** The plan says "df-priors validated in browser using Ajv (dynamic import), once per pack checksum + schema version." Ajv is ~150KB. Dynamic importing, compiling a Draft 2020-12 schema, and validating a 4.9MB JSON document takes 100-500ms. This happens before the first agent can generate, on every fresh session (before IndexedDB caching kicks in).

**Impact:** User sees a loading state for 100-500ms that does not exist in the current implementation. The current flow (`/root/projects/shadow-work/shadow-workipedia/src/agentsView.ts:83-127`) fetches JSON and generates immediately -- no validation step.

**Fix:** Remove Ajv validation entirely. The Rust `serde::Deserialize` implementation in the WASM module already validates the priors JSON structure. If deserialization fails, the WASM function returns an error that the JS side can display. This is strictly more correct than Ajv (which validates schema conformance but not semantic correctness) and has zero overhead. If schema validation is desired for developer tooling, run it in the pack build step (the packer at `/root/projects/shadow-work/shadow-workipedia/wasm/agent-pack/src/main.rs`), not at runtime. Trade-off: none -- build-time validation is strictly better than runtime validation for deterministic data.

### P1-2: JS-side gzip decompression of 5MB+ pack on main thread will freeze UI

**Location:** Pack Format v2 section -- "Decompress gzip in JS before WASM usage"
**Problem:** The plan says to decompress gzip in JS. If this runs on the main thread, decompressing 2-4MB of gzip data takes 20-200ms depending on device. During this time, the UI is frozen -- no button clicks, no text input, no rendering.

**Impact:** User perceives a freeze after clicking "Generate" for the first time, or during page load if decompression is part of initialization.

**Fix:** All pack decompression must happen inside the Worker. The Worker loads the pack file, decompresses gzip blocks, deserializes with WASM, and posts the result back. The main thread never touches compressed data. The plan should explicitly state: "Pack fetch, decompression, and WASM initialization all happen in the Worker. Main thread receives only the generated agent JSON via postMessage." Trade-off: slightly more complex Worker setup, but the Worker already exists for generation so the incremental complexity is minimal.

### P1-3: statrs dependency will massively inflate WASM binary size

**Location:** Priors Sampling section
**Problem:** The plan says "Uses `statrs` and reusable truncated-normal helper." The `statrs` crate transitively depends on `rand`, `rand_distr`, `num-traits`, `num-complex`, and `special`. For a WASM target, these add 200-500KB to the binary after `wasm-opt`. The current `agent-gen` crate (`/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/Cargo.toml`) has minimal dependencies: `serde`, `serde_json`, `bincode`, `wasm-bindgen`, `once_cell`. Adding `statrs` would more than double the WASM binary size.

**Impact:** WASM module download and compile time increases. On 3G, 300KB extra = ~1 second. WASM compile time scales roughly linearly with module size.

**Fix:** Implement truncated normal sampling using the existing Pcg32 RNG (`/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/rng.rs`). A Box-Muller transform produces two normal samples from two uniform samples. Truncation is rejection sampling (resample if outside [min, max]). For 0-1000 range with typical mean ~500 and stddev ~150, rejection rate is <5%. This is ~15 lines of Rust with zero new dependencies. Trade-off: no access to statrs' other distributions (beta, gamma, etc.), but the plan only uses truncated normal.

Code sketch:
```rust
fn truncated_normal(rng: &mut Rng, mean: f64, stddev: f64, min: f64, max: f64) -> f64 {
    loop {
        let u1 = rng.next01().max(1e-10);
        let u2 = rng.next01();
        let z = (-2.0 * u1.ln()).sqrt() * (2.0 * std::f64::consts::PI * u2).cos();
        let value = mean + stddev * z;
        if value >= min && value <= max {
            return value;
        }
    }
}
```

### P1-4: IndexedDB caching adds complexity with no measurable benefit over HTTP cache

**Location:** Runtime Validation & Caching section
**Problem:** The plan says "Cache decompressed df-priors in IndexedDB" and "validated once per pack checksum + schema version (cached in IndexedDB)." IndexedDB operations are async, require error handling for storage quotas, and have inconsistent performance across browsers (50-200ms for multi-MB reads/writes). The plan is essentially reimplementing HTTP caching in application code.

**Impact:** Additional 50-200ms on every page load for the IndexedDB read, plus code complexity for cache invalidation (pack checksum + schema version comparison), storage quota handling, and corruption recovery. The current implementation has none of this -- it fetches JSON with `cache: 'no-store'` (`/root/projects/shadow-work/shadow-workipedia/src/agentsView.ts:86`), which actually disables browser caching.

**Fix:** Replace `cache: 'no-store'` with standard HTTP caching. Serve the pack file with `Cache-Control: public, max-age=86400` and an `ETag` header based on the pack hash (which already exists in `agent-data.pack.meta.json`). The browser handles cache validation, storage, and invalidation automatically. On cache hit, the pack loads from disk in ~1ms with zero JS code. On cache miss (new pack version), the browser fetches and caches automatically.

If the concern is that the pack file's content-based URL does not change (no hash in filename), use filename versioning: `agent-data.v2.{hash}.pack`. This makes the file immutably cacheable. Trade-off: requires updating the HTML/JS to reference the new filename on each pack rebuild, but this is standard practice for static assets and Vite handles it automatically via `import.meta.url`.

### P2-1: 51-point curve lookup arrays are wasteful when 7-keypoint interpolation suffices

**Location:** Derived Outputs section
**Problem:** The plan allows both "explicit 51-point arrays" and "7-keypoint parametric linear interpolation" for curve templates. Having two formats doubles the evaluation code paths in WASM and increases the JSON data size. 51-point arrays are 51 float values (408 bytes each in JSON with typical precision). With 12 templates, that is ~5KB -- negligible for pack size but unnecessary.

**Impact:** Minor: ~5KB extra data, two code paths instead of one.

**Fix:** Standardize on 7-keypoint parametric for all 12 templates. The 51-point array can be pre-sampled from a 7-keypoint definition at build time if exact parity with some reference curve is needed. At runtime, only the 7-keypoint evaluator runs. Trade-off: very slightly less precise curve shapes (linear interpolation between 7 points vs 51), but the difference is imperceptible for 0-1000 integer-scale outputs.

### P2-2: 64-bit FNV-1a IDs are overkill; 32-bit FNV-1a already used in codebase

**Location:** Keys & IDs section
**Problem:** The plan says "IDs are 64-bit FNV-1a hashes of keys computed at pack time." The existing codebase uses 32-bit FNV-1a (`fnv1a32` in `/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/utils.rs:67-74`). 64-bit IDs double the lookup table size in the pack and require `u64` operations in WASM, which on 32-bit WASM targets (wasm32) are emulated with two 32-bit operations per arithmetic op.

**Impact:** Minor: lookup tables are ~2x larger, and ID comparisons are ~2x slower on wasm32. With ~150-200 stat keys, the lookup tables are still small (32-bit: ~800 bytes for 200 entries; 64-bit: ~1600 bytes). The 2x comparison cost is negligible.

**Fix:** Use 32-bit FNV-1a to match the existing codebase. Collision risk for ~200 keys with 32-bit hash is essentially zero (birthday bound for 50% collision is ~77,000 keys). Trade-off: if the stat namespace grows to 1000+ keys, 32-bit collision becomes non-negligible (~0.01%), but the plan says this is a fixed vocabulary.

### P2-3: Factor sampling adds a second truncated-normal pass per group with minimal diversity payoff

**Location:** Correlations (Low-Rank Factors) section
**Problem:** The plan says "2 latent factors per group" with culture-specific mean/stddev, sampled with truncated normal. This means for each of the ~5-8 groups, 2 extra truncated-normal samples are drawn, then applied as mean shifts to all stats in the group. This doubles the truncated-normal sampling count. Each factor affects all stats in its group equally (modulo per-stat weights), creating a correlation structure.

The existing TS codebase achieves inter-stat correlations through explicit deterministic post-hoc adjustments (`/root/projects/shadow-work/shadow-workipedia/src/agent/facets/AGENTS.md` documents 401 correlates, 57 statistically audited). The factor approach is a different strategy -- probabilistic rather than deterministic.

**Impact:** Low. 2 extra truncated-normal samples per group * 8 groups = 16 extra samples. Each is ~10 float operations (Box-Muller + rejection). Total: ~160 extra float operations per agent. This is <1 microsecond in WASM.

The real concern is not performance but whether the factor system produces noticeably different results from the existing 401-correlate system. The plan does not discuss how the two correlation approaches interact. If both are active, factors shift means and then deterministic post-hoc corrections override the factor effects, making the factors wasted computation.

**Fix:** This is an architectural question more than a performance issue. If factors replace the 401 correlates, document this. If they supplement them, document the interaction. The performance cost is negligible either way.

---

## Improvements Suggested

### IMP-1: Compute percentiles lazily in WASM on first access instead of shipping precomputed tables

**Rationale:** The percentile tables are the single largest addition to the pack and are only used in the dossier view, which most users access after generation. Computing 1000 agents in WASM for a single culture takes 1-5 seconds -- acceptable as a one-time cost on first dossier access. This eliminates megabytes from the pack, speeds up initial load for all users, and avoids the ambiguity around how many culture keys need precomputed tables.

The Worker can compute percentiles in the background after the first agent is generated, so by the time the user opens the dossier, the data may already be cached.

### IMP-2: Skip Ajv validation entirely; use Rust-side serde deserialization as the validation gate

**Rationale:** Ajv adds 150KB of dynamic import, 100-500ms of validation time, and a dependency that must be kept in sync with the JSON schema. Serde deserialization is already happening in WASM and validates the exact same structural constraints. If the priors JSON is malformed, `serde_json::from_slice` returns `Err` with a precise error message. The plan's "copy debug info" button can capture the serde error instead.

Build-time validation in the packer (`/root/projects/shadow-work/shadow-workipedia/wasm/agent-pack/src/main.rs`) is the right place for schema validation. The packer can use `jsonschema` crate (Rust) to validate during pack build. Runtime validation is redundant for data that ships as a deterministic build artifact.

### IMP-3: Decompress pack in the Worker, not on the main thread

**Rationale:** The plan says "Decompress gzip in JS before WASM usage" without specifying the thread. Since the Worker already exists for WASM generation, it should also handle: (1) fetch the pack, (2) decompress gzip blocks, (3) load WASM module, (4) deserialize priors/vocab/lookup tables. The main thread should only do: (1) create Worker, (2) send generation request, (3) receive result. This keeps the main thread free for user interaction during the entire initialization sequence.

### IMP-4: Use the existing Pcg32-based Box-Muller transform instead of statrs for truncated normals

**Rationale:** The Pcg32 RNG in `/root/projects/shadow-work/shadow-workipedia/wasm/agent-gen/src/rng.rs` is 33 lines of code and already used throughout the codebase. Box-Muller + rejection sampling for truncated normal is ~15 lines. Statrs would add 200-500KB to the WASM binary for a single function. The custom implementation is also easier to audit for determinism since it has no hidden state or platform-specific behavior.

### IMP-5: Replace IndexedDB caching with standard HTTP caching

**Rationale:** The pack file is a static build artifact. It changes only when the packer runs. Standard HTTP cache headers (`Cache-Control`, `ETag`) handle this with zero application code. The current `cache: 'no-store'` in `agentsView.ts` actively disables caching -- removing it is the highest-leverage change. IndexedDB adds async complexity, storage quota concerns, and cross-tab race conditions for no benefit over the browser's native cache.

### IMP-6: Define all curves as 7-keypoint parametric; drop explicit 51-point arrays

**Rationale:** Simplifies the WASM curve evaluator to a single code path. 7 keypoints with linear interpolation is sufficient for 0-1000 integer-scale outputs. If a specific curve shape cannot be approximated by 7 keypoints, add more keypoints to that specific curve rather than maintaining a separate 51-point format for all curves.

---

## Overall Assessment

**Overall performance risk: Medium -- needs changes before implementation.**

The plan's Rust/WASM approach is correct and will improve generation performance. The Worker thread model is appropriate. The deterministic seeding and data-driven curve system are well-designed. However, the percentile table shipping strategy (P0-1) and the Ajv + IndexedDB + main-thread-decompression stack (P1-1 through P1-4) collectively add seconds of latency to initial page load that do not exist in the current implementation. These are regressions introduced by the redesign, not inherited limitations.

**Must-fix items:**
- P0-1: Percentile tables must not ship in the main pack. Compute lazily or ship as separate on-demand file.
- P1-2: Decompression must happen in the Worker, not on the main thread. Specify this explicitly.
- P1-3: Do not add `statrs` dependency. Use Box-Muller on existing Pcg32.

**Should-fix items:**
- P1-1: Remove Ajv runtime validation. Validate at build time in the packer.
- P1-4: Remove IndexedDB caching. Use HTTP cache headers.

**Premature optimizations to skip:**
- P2-2: 64-bit vs 32-bit IDs -- the pack size difference is negligible.
- P2-3: Factor sampling overhead -- 160 float ops per agent is nothing.
- Curve template format unification (P2-1) -- nice to have but the dual format works.

**Key numbers:**
- Current pack: 5.4MB uncompressed, loads in ~50ms on cache hit
- Percentile tables (worst case): 36MB raw, ~5MB compressed -- unacceptable in main pack
- Ajv validation: 100-500ms one-time cost -- unnecessary
- IndexedDB round-trip: 50-200ms per page load -- unnecessary
- WASM binary with statrs: +200-500KB -- avoidable
- WASM binary without statrs: ~150-300KB estimated (current agent-gen + DF pipeline)
- Worker round-trip overhead: 1-3ms per generation
- WASM generation estimate: 1-5ms per agent (core DF stats + derived outputs)
- Total generation latency (Worker): 2-8ms -- well under perceptible threshold
