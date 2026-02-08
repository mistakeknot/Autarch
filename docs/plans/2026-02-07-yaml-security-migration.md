# YAML Security Migration Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Migrate all raw `yaml.Unmarshal` call sites to use `pkg/yamlsafe` wrappers, eliminating billion-laughs and oversize-file vulnerabilities.

**Architecture:** The `pkg/yamlsafe` package already exists with `Decode()`, `DecodeStrict()`, `UnmarshalFile()`, and `UnmarshalFileStrict()`. We need to migrate ~25 remaining call sites from raw `yaml.Unmarshal` to the appropriate yamlsafe function. Two categories: (1) file-based callers that read YAML from disk should use `UnmarshalFile`/`UnmarshalFileStrict`, (2) in-memory callers that parse YAML from strings/bytes should use `Decode`/`DecodeStrict`.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, `pkg/yamlsafe`

---

### Task 1: Migrate file-based YAML callers to yamlsafe.UnmarshalFile

These callers read YAML from disk via `os.ReadFile` then `yaml.Unmarshal`. They should use `yamlsafe.UnmarshalFile` (or `UnmarshalFileStrict` where appropriate) instead.

**Files to modify:**
- `internal/gurgeh/specs/validate.go:37` — reads spec file, unmarshals
- `internal/gurgeh/specs/metadata.go:15` — reads spec file, unmarshals
- `internal/pollard/hunters/creator.go:207` — reads hunter spec from disk
- `internal/pollard/quick/scan.go:124,149` — reads scan output files
- `internal/pollard/reports/generator.go:408,438,467,497` — reads report data files
- `internal/pollard/api/scanner.go:698,954` — reads scan data files
- `pkg/compound/search.go:87` — reads frontmatter from solution files
- `pkg/mcp/handlers.go:468,572` — reads YAML data files

**Step 1: For each file above, replace the pattern:**
```go
// Before:
data, err := os.ReadFile(path)
// ... error check ...
if err := yaml.Unmarshal(data, &out); err != nil {

// After:
if _, err := yamlsafe.UnmarshalFile(path, &out); err != nil {
```

Note: If the caller needs the raw bytes (e.g., for hashing), `UnmarshalFile` returns `([]byte, error)` — capture the first return value.

**Step 2: Update imports** — remove `"gopkg.in/yaml.v3"` if no longer used, add `"github.com/mistakeknot/autarch/pkg/yamlsafe"` (if not already present).

**Step 3: Verify**
```bash
go build ./...
go test ./internal/gurgeh/specs/... ./internal/pollard/... ./pkg/compound/... ./pkg/mcp/... -count=1
```

**Step 4: Commit**
```bash
git add -A && git commit -m "sec: migrate file-based YAML callers to yamlsafe.UnmarshalFile"
```

---

### Task 2: Migrate in-memory YAML callers to yamlsafe.Decode

These callers parse YAML from in-memory strings/bytes (e.g., LLM output, test data). They should use `yamlsafe.Decode` instead of raw `yaml.Unmarshal`.

**Files to modify:**
- `internal/gurgeh/suggestions/suggestions.go:314` — parses LLM YAML output
- `internal/gurgeh/specs/schema_test.go:47` — test helper
- `internal/coldwine/cli/init_flow.go:344,348,353` — parses LLM YAML output
- `internal/pollard/hunters/agent.go:206` — parses LLM YAML output
- `internal/pollard/hunters/creator.go:131` — parses LLM YAML output
- `internal/pollard/proposal/generator.go:195,283` — parses LLM YAML output
- `pkg/mcp/server_test.go:514` — test helper

**Step 1: For each file, replace:**
```go
// Before:
if err := yaml.Unmarshal([]byte(content), &out); err != nil {

// After:
if err := yamlsafe.Decode([]byte(content), &out); err != nil {
```

**Step 2: Update imports** — same pattern as Task 1.

**Step 3: Verify**
```bash
go build ./...
go test ./internal/gurgeh/... ./internal/coldwine/... ./internal/pollard/... ./pkg/mcp/... -count=1
```

**Step 4: Commit**
```bash
git add -A && git commit -m "sec: migrate in-memory YAML callers to yamlsafe.Decode"
```

---

### Task 3: Verify no raw yaml.Unmarshal remains outside yamlsafe

**Step 1: Run grep**
```bash
grep -rn 'yaml\.Unmarshal' --include='*.go' | grep -v yamlsafe | grep -v _test.go | grep -v docs/
```

Expected: zero results (only `pkg/yamlsafe/yamlsafe.go` should call raw `yaml.Unmarshal`).

**Step 2: Run full test suite**
```bash
go build ./...
go test ./... -count=1 -timeout=60s
```

Note: Some tests (arbiter phases) may hang — skip with `-run` flag if needed. Focus on: `./internal/gurgeh/...`, `./internal/coldwine/...`, `./internal/pollard/...`, `./pkg/...`

**Step 3: Commit (if any stragglers found)**
