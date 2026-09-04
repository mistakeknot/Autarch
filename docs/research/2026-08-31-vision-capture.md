# Vision capture — Autarch as the HUD over a pace-layered garden

```yaml
artifact_type: capture
evidence_class: primary (mk verbatim, 2026-08-31)
```

## The statement

> "I think Autarch should be the UI/HUD/way for humans to play in/build/grow
> the garden of their projects/interests (especially when they are so deeply
> interrelated via pace layers/systems like my projects are)."
> — mk, 2026-08-31

## What each word carries

- **HUD**: an overlay on the game world, never the world itself. You play
  *through* it; it renders state and attention; the playing happens in the
  world (terminals, Zed, salons). This ratifies the door's standing
  discipline — transcribe, never own — as vision rather than implementation
  choice. (It also redeems the name: an autarch is a *self*-ruler; the HUD
  serves one person governing their own estate.)
- **play in / build / grow**: three verbs, one loop — flow (the Stellaris
  finding), construction, gardening. Play is load-bearing, not decoration:
  the discover stage established that dev software fails precisely at flow.
- **projects/interests**: the estate is wider than repos. Cards (docs/why.md)
  are not code-specific; roots are already plural. Scope question for derive:
  what does a non-code garden's card look like?
- **pace layers/systems**: the interrelations from the guests-talk ruling are
  not arbitrary edges — they have Brand-style structure: fast layers innovate
  and churn (prototypes, probes), slow layers stabilize and constrain
  (doctrine, canon, platform contracts). A slow-layer decision reverberates
  upward into many fast-layer gardens; a fast-layer finding occasionally
  deserves to sediment down into doctrine.

## Existing infrastructure under each piece

- **pace layers are already modeled**: mk's CanonGraph carries
  `projects_in_layer` and `serving_map` as named queries, and the
  `pace-layer-universe` project (infinite-fun-space) exists. The layer data
  the HUD needs is queryable today — the door would render a structure that
  already exists, not invent one.
- **temporal heterogeneity**: Garden Salon principle #2 ("editing, ceremony,
  and lifecycle are different kinds of time") is pace layering at room scale;
  the vision applies it at estate scale.
- **Stellaris**, re-read through this lens: a 4X *is* pace-layered (pops
  grow slowly, fleets move fast, traditions accrete over decades), and its
  speed controls are pace-layer navigation. The teardown's mechanisms sort by
  layer: outliner = fast-layer glance; situations = slow-layer watch.

## Consequences for derive (stage 5)

1. The linear 8-step journey skeleton is probably the wrong spine — mk never
   corrected it, and the vision suggests CUJs organized by **tending cadence**
   instead: the daily walk (fast), the session dive, the seasonal reshaping,
   the slow-layer canon-tending. Different layers want different visit
   rhythms — per-layer attention cadence, not one loop.
2. Cross-garden discourse (the guests-talk ruling) gets its routing rule from
   layers: edges follow the serving/layer structure, and a cross-layer
   contradiction (fast probe vs slow doctrine) is precisely the class of
   interrupt worth a popup rather than a toast.
3. The door's data plane grows a third source beside card-check and tmux:
   the layer/serving graph (CanonGraph), read-only, same transcription
   discipline.
