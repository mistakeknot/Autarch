// mk-jb5ul: turn the worker's event loop once after every test.
//
// A vitest worker sends its task updates to the runner over birpc and gives each call 60 s to be answered. The reply is read
// in the event loop's poll phase. Many tests here run a synchronous child process (spawnSync of an installer or a gate script),
// and vitest awaits between tests on microtasks only, so a file of them never lets the loop turn. Under host load such a file
// ran for 70-107 s with its reply unread, and the worker raised `Timeout calling "onTaskUpdate"` with every test green.
// Waiting on a zero-delay timer after each test is a full loop turn (timers, poll, check); it costs a millisecond and bounds the unread time to one test.
//
// The timer comes from node:timers, not the global, so a test that installed fake timers cannot stall it.
import { setTimeout as realSetTimeout } from "node:timers";
import { afterEach } from "vitest";

afterEach(() => new Promise<void>((resolve) => { realSetTimeout(resolve, 0); }));
