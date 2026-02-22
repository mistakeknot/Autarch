# FrankenTUI Research Synthesis

> Thorough analysis of github.com/Dicklesworthstone/frankentui
> Repo cloned to `frankentui-research/` (gitignored)
> Date: 2026-02-21

## What Is It

FrankenTUI is a **Rust TUI kernel** (18 crates, ~1.4M lines including tests) by Jeffrey Emanuel, built entirely with AI agents (Claude/Codex). Implements Elm/Bubbletea architecture with layered crate structure: core -> render -> runtime -> widgets.

## Scale of the Codebase

| Crate | Key Files | Notable Sizes |
|-------|-----------|---------------|
| ftui-render | buffer.rs, cell.rs, diff.rs, presenter.rs | buffer=134K, diff=146K, presenter=164K, budget=143K |
| ftui-runtime | program.rs, resize_coalescer.rs | program=9700 lines, resize_coalescer=4340 lines |
| ftui-widgets | ~37 widgets, virtualized.rs, stateful.rs | Full widget library |
| ftui-extras | doom_fire.rs, plasma.rs, metaballs.rs | VFX rasterizer 3238 lines |
| ftui-layout | flex.rs, grid.rs, responsive.rs | Two-pass constraint solver |

---

## Tier 1: Directly Applicable to Bigend TUI

These are patterns that map directly to problems we've hit in Bigend (our Go/Bubble Tea TUI) and could be adapted to Go.

### 1. Dirty Row Tracking

Three-tier system: row bitmap -> dirty spans -> cell bitmap.
Our Bigend renders entire frames every tick. Even a simple dirty-row boolean vector could skip 90%+ of diff work on stable frames.

**Implementation**: Buffer has `dirty_rows: Vec<bool>`, `dirty_spans: Vec<DirtySpanRow>` (SmallVec<4> per row), and `dirty_bits: Vec<u8>` (per-cell bitmap). All mutations call `mark_dirty_*()` to maintain the invariant: if any cell in row y changed, `dirty_rows[y] == true`.

### 2. Budget Degradation with PID Controller

Levels: `Full -> SimpleBorders -> PlainText -> MinimalLayout -> SkipFrame`.
PID gains: Kp=0.5, Ki=0.05, Kd=0.2, tuned for 16ms/60fps target.
Anti-windup: integral clamped to +/-5.0. Settling time: 8-12 frames.

E-Process gate: `E_t = E_{t-1} * exp(lambda * r_t - lambda^2 * sigma^2 / 2)`, alert when E_t > 1/alpha (default 20). Prevents PID from thrashing on transient spikes.

We could adapt this for Bigend's frame pacing when the aggregator has many items.

### 3. Inline Mode with Scrollback Preservation

TerminalWriter protocol: save cursor, clear UI region, render, restore cursor. Logs write above UI and terminal scrolls naturally. Exactly what we'd want for agent output in Bigend.

Two atomic write points:
- `write_log()` - safe from background threads, writes above UI with cursor save/restore
- `present_ui()` - main thread only, renders entire frame in one atomic write

### 4. Virtualized Lists with Fenwick Tree

O(log n) scroll-to-index for variable-height items. Our Bigend lists currently scan linearly.

Three height strategies:
- `Fixed(u16)` - all items same height
- `Variable(HeightCache)` - LRU cache, linear scan for scroll-to-index
- `VariableFenwick` - Fenwick tree (BIT) for O(log n) operations

The `Virtualized<T>` pattern with owned/external storage and follow-mode (auto-scroll on new items) is clean and portable.

### 5. Resize Coalescing

Even without BOCPD, the basic pattern is valuable:
- Detect burst vs steady resize events
- Coalesce during bursts (40ms delay), responsive in steady state (16ms)
- Always respect hard deadline (100ms max latency)
- Fairness guard ensures input events aren't starved

BOCPD algorithm maintains run-length posterior for O(K=100) complexity, detects regime transitions between steady (mu=200ms) and burst (mu=20ms) inter-arrival times.

### 6. Widget Persistence (Stateful Trait)

`StateKey = (widget_type, instance_id)`, versioned save/restore with graceful degradation on version mismatch. Could preserve Bigend scroll positions and expanded states across sessions.

Round-trip invariant: `restore_state(save_state())` must produce equivalent observable state.

---

## Tier 2: Architectural Patterns Worth Adopting

### 7. One-Writer Rule

Only one component owns terminal output. All writes serialized through a single writer. Prevents concurrent ANSI corruption (we've hit this in Bigend with async updates).

### 8. Style Cascade via Option Merge

`child.fg.or(parent.fg)` for inheritance, `child.attrs | parent.attrs` for combining flags. Elegant, zero-cost abstraction.

### 9. Hit Grid (Interactive Regions)

Parallel grid to the cell buffer that maps (x,y) -> widget_id + region type. Enables precise mouse click routing without widget-level hit testing.

### 10. Evidence Ledger Pattern

Every probabilistic decision (diff strategy, resize coalescing, budget degradation) records its reasoning as a ledger of Bayes factors. Makes debugging deterministic.

### 11. Conformal Alerting

Distribution-free thresholds that self-calibrate from recent data. No magic numbers. Mondrian bucketed approach grouping by (rendering_mode, diff_strategy, size_bucket). Could apply to sprint token budgets or agent dispatch decisions.

---

## Tier 3: Over-Engineering to Avoid

These are impressive but represent complexity that wouldn't justify itself in our Go codebase.

### 12. BOCPD for Resize
Bayesian Online Change-Point Detection - a simple debounce timer with burst detection achieves 95% of the benefit at 1% of the complexity.

### 13. E-Process for Budget Gating
Prevents PID from thrashing, but only matters at very high frame rates with sustained load. A simple cooldown counter suffices for most TUI apps.

### 14. 16-byte Cache-Aligned Cells
Makes sense for Rust SIMD, but Go's GC and different memory model mean this doesn't translate.

### 15. Bayesian Command Palette Scoring
Mathematically beautiful but well-tuned Levenshtein + position bonus gets you 99% there.

### 16. Tile-Based SAT Diff
Summed Area Table for skipping empty regions on large screens. Only kicks in at 12,000+ cells.

---

## Render Kernel Deep Dive

### Cell Structure (16 bytes, #[repr(C, align(16))])
```
Cell { content: CellContent(4B), fg: PackedRgba(4B), bg: PackedRgba(4B), attrs: CellAttrs(4B) }
```
- CellContent: bit 31=0 for direct char (bits 0-20 = Unicode scalar), bit 31=1 for GraphemeId pool reference
- Branchless comparison via `bits_eq()`: uses `&` not `&&` for all 4 u32s -> LLVM lowers to single 128-bit SIMD compare
- GraphemeId packs width (4 bits), generation (11 bits), slot (16 bits) into 4 bytes

### Diff Algorithm
1. Skip clean rows via dirty_rows bitmap (O(1) per clean row)
2. Within dirty rows, skip 512-byte blocks (32 cells)
3. Within dirty blocks, compare 64-byte cache-line chunks (4 cells) via bits_eq()
4. Coalesce adjacent changes into ChangeRun structs

### Presenter (State-Tracked ANSI Emission)
- 64KB BufWriter with CountingWriter wrapper
- Per-row cost model (DP): sparse-run vs merged write-through strategy
- Style tracking avoids redundant SGR sequences
- DEC 2026 sync brackets for flicker-free atomic display
- Byte cost estimates: CUP=4+digits, CHA=3+digits

---

## Runtime Deep Dive

### Event Loop (6 message sources, priority order)
1. Poll events (smart timeout: min of tick deadline, resize deadline)
2. Drain all terminal events -> handle_event() -> update()
3. Process subscription messages -> update()
4. Process background task results -> update()
5. Tick on schedule -> update()
6. Resize coalescer tick

### Cmd System
- None, Quit, Msg(m), Batch, Sequence, Tick(duration), Log(string)
- Task(spec, closure) - spawn background thread or enqueue on Smith-rule scheduler
- SaveState, RestoreState, SetMouseCapture
- execute_cmd() is recursive: Cmd::Msg can produce more commands

### Subscription Lifecycle
- Each has unique SubId for deduplication
- StopSignal via Arc<(Mutex<bool>, Condvar)>
- reconcile_subscriptions() diffs desired vs running by SubId

---

## Layout & Widgets Deep Dive

### Constraint Solver (Two-Pass)
Pass 1: Hard minimums (Fixed, Min, FitMin) - non-negotiable, deducts from pool
Pass 2: Soft/relative (Percentage, Ratio, Fill) - proportional distribution of remainder
Weight system: WEIGHT_SCALE=10,000 for proportional distribution

### Widget Trait (Minimal)
```rust
trait Widget { fn render(&self, area: Rect, frame: &mut Frame); fn is_essential(&self) -> bool { false } }
trait StatefulWidget: Widget { type State; fn render(&self, area, frame, &mut State); }
```
- Composition via trait forwarding (Budgeted<W> implements both Widget and StatefulWidget)
- is_essential() for budget degradation (skip decorative widgets under pressure)

### Notable Widget Patterns
- List: binary search on filtered_indices for O(log N) navigation, BTreeSet for multi-select
- Table: intrinsic_col_widths cache avoids re-measuring per render
- Input: grapheme-aware cursor (not byte offset), IME preedit support
- Tree: guide character system with 5 styles (Ascii, Unicode, Bold, Double, Rounded)

---

## Bayesian Systems (All Fully Implemented, Not Just Docs)

1. **VOI Sampling** (voi_sampling.rs, 1416 lines) - Beta-Bernoulli model, posterior variance reduction, e-process layer
2. **Conformal Prediction** (conformal_predictor.rs, 984 lines) - Mondrian bucketed, distribution-free
3. **Bayesian Diff Strategy** - Beta prior (a=1, b=19), state machine with 4 regimes, JSONL evidence ledger
4. **Conformal Alerting** (conformal_alert.rs) - quantile_{(1-alpha)(n+1)/n}(R), e-process overlay
5. **Property-based tests**: 28 verified invariants via proptest, ~1000 inputs per invariant

---

## VFX (crates/ftui-extras/)

8+ fully coded effects: Doom Fire (PSX algorithm), Plasma (6-wave interference), Metaballs, Screen Melt, Underwater Warp, Doom Melt, Quake Console (3D rasterizer), Canvas adapters

Rasterizer architecture: cell-space rendering, quality tiers (Full/Reduced/Minimal/Off), zero per-frame allocations (grow-only buffers), WCAG-aware palettes.

---

## Mapping to Interverse Projects

| FrankenTUI Pattern | Interverse Target | Priority |
|---|---|---|
| Dirty row tracking | Bigend TUI (apps/autarch) | High |
| Budget degradation | Bigend (large aggregator views) | High |
| Inline mode | Agent output display | Medium |
| Virtualized lists | Bigend dispatch/run lists | Medium |
| Resize coalescing | Bigend + tuivision | Medium |
| One-writer rule | Bigend concurrent updates | Medium |
| Evidence ledger | intercore phase decisions | Low |
| Widget persistence | Bigend state across sessions | Low |
| Conformal alerting | interstat token budgets | Low |

---

## Key Takeaway

FrankenTUI is a masterclass in principled TUI engineering - every heuristic replaced with a proper statistical method, every optimization mathematically justified. But it's also clearly AI-generated at scale (README alone is 1700 lines of formulas). The most portable value is in the architectural patterns (dirty tracking, budget degradation, inline mode, one-writer rule) rather than the specific Rust/SIMD/Bayesian implementations.

The repo is at `research/frankentui/` (gitignored).
