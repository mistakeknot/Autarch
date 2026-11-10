import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // Most of the deploy-tool and install-failure tests spawn the tool, git and flock several times, so on a loaded host they take
    // several times their idle time. Measured under a 32-process busy-loop hog (load average 18 rising to 47, 8 workers): the slowest
    // of 399 tests took 12.97 s (install-failure-paths), 65 more took 3-7 s; alone, the slowest takes about 4 s (a health-timeout test waits out its own 3 s). The default 5 s
    // limit timed out whenever the host was busy. 30 s is 2.3 times the worst measured case; a test that hangs still fails.
    testTimeout: 30_000,
    include: ["__tests__/**/*.test.ts", "__tests__/**/*.test.tsx"],
    exclude: ["__tests__/e2e/**"],
    // A test that builds the server must never append to the real rulings ledger.
    env: { AUTARCH_RULINGS_LEDGER: "off" },
    // Turns the worker's event loop after each test, so a run of synchronous tests cannot starve the runner's replies (mk-jb5ul).
    setupFiles: ["__tests__/setup/turn-event-loop.ts"],
  },
});
