import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["__tests__/**/*.test.ts", "__tests__/**/*.test.tsx"],
    exclude: ["__tests__/e2e/**"],
    // A test that builds the server must never append to the real rulings ledger.
    env: { AUTARCH_RULINGS_LEDGER: "off" },
  },
});
