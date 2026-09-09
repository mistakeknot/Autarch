# Guidance, plan preparation and implementation are separate actions

Status: accepted design after independent Fable review; not implemented.
Authority: [mk's September 9 instruction](2026-09-09-human-ratification.md).
Work: `Sylveste-fuwn`; Clavain prerequisite: `Sylveste-k8ht`.

Accepting a proposal must create no new Execution. Preserve every Execution
already authorized under the feedback pilot. A visit finishes when a meaningful
outcome has an independently reviewed plan; implementation starts separately.

Clavain owns three preparation operations, with separate receipts from the
existing execution-oriented review API:

- `prepare ratify` persists and commits the explicitly accepted guidance paths
  on clean main. Preview that effect before acceptance. Use the execution
  worker's same per-project lock, validate all base hashes before writes, and
  journal expected file hashes, parent/branch, commit identities, ruler,
  synthesis revision, receipt and transcriber. Recover partial writes and
  interrupted commits by exact evidence; conflicts remain blocked. Do not push
  or dispatch implementation.
- `prepare submit` persists an idempotent preparation request and may start or
  resume its preparation supervisor. Use a submission lock, a lifetime worker
  lock, durable launch intent and exact Intercore dispatch reconciliation.
  Unknown liveness never permits a duplicate dispatch.
- `prepare status` only reads the receipt. Polling never launches a worker.

The visit synthesis extends the existing Proposal type. Preparation binds its
accepted revision, committed guidance hashes, source coverage, scoped budget
and artifacts. Plan and execution-specification digests bind separate actual
planner and reviewer routing receipts, policy hash and review verdict. Same
model, missing evidence, changed sources or wrong plan revision cannot pass.
Persist prior attempts and deficiencies.

Use a preparation-scoped Intercore run with a hashed scope ID, one agent at a
time and six maximum dispatches: initial planning/review and two bounded repair
attempts. A per-phase 15-minute watchdog leaves a blocked receipt with the
dispatch ID retained; it does not authorize replacement work. The overall token
budget still limits all attempts. Resolve planning with its accountable context
and verify the returned frontier classification; review uses the actual
planner identity.

Explicit `execution.start` is a durable human-approved operation bound to the
reviewed plan bundle and displayed implementation budget/build specification.
It atomically creates a deterministic implementation Proposal and Execution.
The Proposal contains no guidance blocks because they are already committed;
the original synthesis retains them. Carry guidance hashes on the Execution
and recheck them immediately before submission to Clavain. A mismatch blocks.
Retries return the same records, never additional work.

Legacy-kind proposals accepted after this change also use ratify and prepare;
implementation is unavailable until a reviewed preparation exists. Existing
Execution records continue with their original Proposal and historical rules.

Alternatives rejected: reuse review submit/status for preparation, or merely
remove execution creation from acceptance. Existing status can launch an
implementation worker, and removing execution alone strands canonical guidance.

Current prerequisite: usage-required dispatch rejects Fable, while Astra cannot
review its own plan. `Sylveste-k8ht` returns that failure to the Clavain owner.
Do not bypass budgets or substitute the producer as reviewer. Independently
scheduled CI, actual budgeted review, evaluation and signed-build journey remain
delivery gates. Design approval does not satisfy them.
