import type { WakeAttribution } from "./wake-attribution.js";
import type { ThreadsLike } from "./wakes.js";

// Compiler-only regression checks, exercised by npm run typecheck.
type SendArgs = Parameters<ThreadsLike["send"]>[0];
const attribution = { wakeCause: "card_pick", wakeRef: "pick:example" } satisfies WakeAttribution;
({ threadId: "example", input: [], ...attribution }) satisfies SendArgs;
// @ts-expect-error Misspelled attribution keys must be rejected by the adapter contract.
({ threadId: "example", input: [], wakeCuase: "card_pick" }) satisfies SendArgs;
// @ts-expect-error Misspelled reference keys must be rejected too.
({ threadId: "example", input: [], wakeReff: "pick:example" }) satisfies SendArgs;
