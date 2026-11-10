import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["__tests__/**/*.test.ts", "__tests__/**/*.test.tsx"],
    exclude: ["__tests__/e2e/**"],
    // A test that builds the server must never append to the real rulings ledger.
    env: { AUTARCH_RULINGS_LEDGER: "off" },
    // Turns the worker's event loop after each test, so a run of synchronous tests cannot starve the runner's replies (mk-jb5ul).
    setupFiles: ["__tests__/setup/turn-event-loop.ts"],
  },
});
