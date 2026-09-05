---
artifact_type: assessment
date: 2026-09-05
bead: Sylveste-fuwn
verdict: port-partially
decision_status: direction-agreed
scope: Flere AgentSession and JSONL RPC as Autarch collaborator runtime
---

# Flere as Autarch's native agent

## Recommendation

The user selected **“Flere as intended native default, with deep Clavain
integration”** after reviewing the recommendation and readiness findings.
This establishes the intended product direction. It does not activate a
runtime default or complete the project-collaborator qualification journey.
The recommended selectable Codex alternative remains compatible with this
direction; the exact alternate-runtime experience still needs design.

Flere's owned, customizable, multi-provider runtime is a good fit for a
consistent Autarch conversation and review experience as models change.
The proposed product is an Autarch interface powered by Flere; its interaction
need not reproduce Flere's terminal UI. Clavain continues to govern routing,
workflow, authority, and execution. Intercore retains operational records;
project files retain agreed product truth.

The adoption scope is the working AgentSession/JSONL RPC interface. Broader
runtime scaffolds and new control-plane infrastructure are not prerequisites
for this product discussion. The existing restricted-worker proposal serves
an execution boundary; it is not the design of the continuing collaborator.

## Current evidence

Checked the local checkout, published main, launcher, relevant source, prior
integration review, and bounded behavioral tests. No Flere or ecosystem
application code was changed, no runtime default was changed, and no provider
model call was made. Existing unrelated changes were left alone.

| Component | Observed state |
|---|---|
| Flere main | Local HEAD and live remote main both `4e494929998d6bc4fccf75e0a233f727db4b70ee`. |
| Local fork work | Identity, packaging, RPC-client repairs, and worker tests are uncommitted. `worker-rpc.ts` is untracked and contains throwing stubs. |
| Installed command | `/Users/sma/.local/bin/flere` invokes `/Users/sma/projects/Flere/pi-test.sh`, which runs the checkout's TypeScript CLI. A fresh version probe returned `0.84.3`; this does not identify a clean released fork artifact. |
| Clavain source | Main `fa70851`; its dispatch parser rejects `--to flere --help` before launching anything. No Flere backend exists in inspected dispatch/config/hook source. |
| Intercore source | Main `bc64530`; local uncommitted work repairs backend forwarding and public dispatch schema. The governed Flere worker and authoritative direct-spawn admission are not established in that inspected source. |
| Installed ecosystem | Installed `ic` reports build `bc645304f0e3`. The Clavain shim selects compiled `081f257`, containing Intercore `ac2dc66`; it differs from current source. |

The public [Flere repository](https://github.com/mistakeknot/Flere) still has
the upstream-oriented README at the verified remote revision. The local
README documents the renamed launcher and disabled self-update. Package names
and the default `.pi/agent` storage remain shared with upstream Pi; a distinct
configuration/import experience remains a product decision.

## What makes it a strong fit

- **Model flexibility within one runtime.** RPC exposes provider/model
  selection, model-change events, session state, and branch/resume operations.
  Focused faux-provider tests exercised model changes and session restoration.
  Switching models inside Flere is distinct from transferring work to the
  Codex or Claude Code runtime, which has its own instructions and tools.
- **Questions inside the host UI.** RPC emits correlated select, input,
  confirm, and editor requests. A real process probe answered select/input/
  confirm in sequence, including a false confirmation. No model was needed
  to prove that protocol round trip.
- **Custom product tools and context.** Extensions and the SDK allow the
  collaborator's capabilities to be connected to project evidence, decisions,
  and Clavain. Discovering shared skills alone does not activate Claude hooks,
  MCP servers, coordination rules, or an Autarch integration.
- **A useful existing integration boundary.** Autarch is Go, and Flere offers
  a language-independent subprocess protocol. It provides more interaction
  capability than Autarch's current one-shot `codex exec` stream adapter.
  This is a comparison with that adapter, not a claim that Codex App Server
  lacks comparable integration capabilities.

Source references: [RPC documentation](https://github.com/mistakeknot/Flere/blob/4e494929998d6bc4fccf75e0a233f727db4b70ee/packages/coding-agent/docs/rpc.md),
[SDK documentation](https://github.com/mistakeknot/Flere/blob/4e494929998d6bc4fccf75e0a233f727db4b70ee/packages/coding-agent/docs/sdk.md),
[Autarch's existing adapter](../../pkg/agenttargets/backend_codex.go).

## Why the installed default is not ready

1. **The agreed execution path is missing.** Clavain currently rejects Flere.
   Calling a subprocess directly would not prove the Clavain-governed journey
   the user agreed to. Backend identity, admission, durable outcome recording,
   and recovery need end-to-end evidence.
2. **The proposed restricted worker is still a stub.** All six worker tests
   fail at unimplemented argument validation or startup. The tests describe
   a proposed read-only worker, not functioning support.
3. **Passing repairs are local.** RPC rejection and child-exit handling now
   pass focused tests in this dirty checkout. Yesterday's review findings
   should not be repeated as if no fixes exist, but those fixes are not
   published main or a qualified distribution.
4. **The collaborator experience remains to be built and exercised.** No
   Autarch screen rendered the probe's questions; it was a raw protocol host.
   No real model completed foundation review, transferred context, or carried
   a human ruling into delivered work during this investigation.
5. **Runtime choice does not establish model access or quality.** Provider
   support is not proof that a specific account can access a specific model,
   or that the model performs equivalently through different agent harnesses.

The prior [integration review](/Users/sma/projects/Flere/docs/research/flux-drive/2026-09-04-flere-integration/summary.md)
recommended an optional host/backend under Clavain and Intercore. Today's
product recommendation goes further toward an intended default because the
user has now agreed to a continuing embedded project collaborator. It does
not erase that review's unresolved integration evidence.

## Verification and limits

From `Flere/packages/coding-agent`:

```text
node ../../node_modules/vitest/dist/cli.js --run
  test/rpc-client-responses.test.ts
  test/rpc-client-process-exit.test.ts
  test/rpc-jsonl.test.ts
  test/rpc-prompt-response-semantics.test.ts
  test/fork-distribution.test.ts
  test/suite/worker-rpc.test.ts
```

Result: **42 passed, 6 failed**. All failures are in the unimplemented
restricted-worker suite. The successful tests cover negative acknowledgements,
child death and waiter cleanup, framing, prompt acknowledgement semantics, and
selected fork identity/update behavior. They do not qualify built release artifacts.

The model/extension and runtime characterization files contain **29 tests**.
The initial run passed 19 and hit ten filesystem-permission errors because
runtime fixtures tried to create sessions in the user's `.pi` directory.
Rerunning only `agent-session-runtime.test.ts` with
`FLERE_CODING_AGENT_DIR=/private/tmp/autarch-flere-runtime-tests-20260905`
passed all 11 runtime tests. The other 18 model/extension tests passed in the
initial run. This yields **71 distinct focused passes and 6 worker failures**
across the selected files, not a full project test result.

Real-process RPC probe: [result](/private/tmp/autarch-flere-rpc-atz6amf6/result.json),
[events](/private/tmp/autarch-flere-rpc-atz6amf6/rpc-events.json),
[probe script](/private/tmp/autarch-flere-rpc-probe-20260905.py).
It used a temporary profile, no credentials, offline mode, disabled ambient
extensions/skills/templates, and an explicit deterministic fixture extension.
It proved matching question responses, declined confirmation, invalid-model
rejection, and an idle abort acknowledgement. It did not test active-turn
cancellation, restart recovery, model quality, or Autarch UI behavior.

Alwe session search was unavailable because its lexical index needs repair;
the existing review and its referenced current source supplied prior context.
The broad mutating `npm run check`, full suite, builds, installation, and
provider evaluations were outside this read-only adoption investigation.

## Product ruling and remaining design

The structured default-choice question was answered by the user:
**“Flere as intended native default, with deep Clavain integration”.**
The runtime preference is now settled. Deep Clavain integration is an explicit
requirement; its interaction and capability boundaries still need clarification.
Any eventual default promotion needs the agreed onboarding-to-delivery journey
to demonstrate usefulness, continuity, truthful execution results, and
controllable resource use.

Candidate integration concerns for the continuing brainstorm include shared
project/sprint context, guided product decisions, Clavain phase and artifact
state, execution routing, return of evidence, and interrupted-work recovery.
These are a coverage inventory rather than an approved implementation plan.

Next structured design question: should Autarch present a visible guided
sprint with conversation alongside artifacts and decisions, or lead with
conversation and reveal sprint details on demand? No answer recorded.

The prior conversation-versus-linked-sessions question remains open. Flere
makes an additional distinction useful: model changes inside one runtime
versus handoffs between different agent runtimes.
