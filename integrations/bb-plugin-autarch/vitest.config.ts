import { defineConfig } from "vitest/config";

export default defineConfig({
  test: { include: ["__tests__/**/*.test.ts", "__tests__/**/*.test.tsx"], exclude: ["__tests__/e2e/**"] },
});
