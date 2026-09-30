import { defineConfig } from "vitest/config";

export default defineConfig({
  test: { include: ["__tests__/e2e/**/*.test.ts"], testTimeout: 120_000 },
});
