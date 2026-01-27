# Pollard Hunters Reference

> Complete reference for all available research hunters

Pollard uses **hunters** to gather research data from various APIs and sources. This document covers all available hunters, their configuration, rate limits, and usage.

---

## Quick Reference

### Tech-Focused Hunters (Enabled by Default)

| Hunter | Purpose | API | Auth Needed |
|--------|---------|-----|-------------|
| `github-scout` | Find OSS implementations | GitHub Search | Optional (GITHUB_TOKEN) |
| `hackernews` | Industry discourse & trends | HackerNews Algolia | None |
| `arxiv` | Academic CS/ML papers | arXiv | None |
| `competitor-tracker` | Monitor competitor changes | HTML scraping | None |

### General-Purpose Hunters (Disabled by Default)

| Hunter | Purpose | API | Auth Needed |
|--------|---------|-----|-------------|
| `openalex` | 260M+ academic works, all disciplines | OpenAlex | Optional (OPENALEX_EMAIL) |
| `pubmed` | 37M+ biomedical citations | NCBI E-utilities | Optional (NCBI_API_KEY) |
| `usda-nutrition` | 1.4M+ foods, nutrients, allergens | USDA FoodData Central | **Required** (USDA_API_KEY) |
| `legal` | 9M+ US court decisions | CourtListener | **Required** (COURTLISTENER_API_KEY) |
| `economics` | Global economic indicators | World Bank | None |
| `wiki` | Millions of entities, all domains | Wikipedia/Wikidata | None |

### Framework Documentation Hunter

| Hunter | Purpose | API | Auth Needed |
|--------|---------|-----|-------------|
| `context7` | Framework docs for 100+ libraries | Context7 MCP | None |

### Agent-Native Hunter

| Hunter | Purpose | API | Auth Needed |
|--------|---------|-----|-------------|
| `agent` | AI agent as primary research tool | Spawns Claude/etc. | Agent command configured |

---

## Tech-Focused Hunters

### github-scout

**Purpose:** Find relevant open-source implementations and libraries.

**API:** [GitHub Search API](https://docs.github.com/en/rest/search)

**Output:** `.pollard/sources/github/YYYY-MM-DD-<query-slug>.yaml`

**Features:**
- 4-stage pipeline: Search → Fetch README → Synthesize → Score
- Quality scoring with confidence levels
- Stars-based filtering
- Topic and language detection

**Configuration:**
```yaml
github-scout:
  enabled: true
  queries:
    - "task orchestration golang"
    - "AI agent coordination"
  max_results: 50
  min_stars: 100
  output: sources/github
```

**Rate Limits:**
| Auth Status | Limit |
|-------------|-------|
| Unauthenticated | 10 req/min (search), 60 req/hr (general) |
| With GITHUB_TOKEN | 30 req/min (search), 5000 req/hr (general) |

**Environment Variable:** `GITHUB_TOKEN` or `POLLARD_GITHUB_TOKEN`

---

### hackernews (trend-watcher)

**Purpose:** Track industry discourse and trending discussions.

**API:** [HackerNews Algolia API](https://hn.algolia.com/api)

**Output:** `.pollard/sources/hackernews/YYYY-MM-DD-<query-slug>.yaml`

**Features:**
- Story and comment search
- Points-based filtering
- Date range support

**Configuration:**
```yaml
hackernews:
  enabled: true
  queries:
    - "AI coding assistant"
    - "LLM agents"
  min_points: 50
  output: sources/hackernews
```

**Rate Limits:** Generous (no strict documentation), but be polite.

---

### arxiv (research-scout)

**Purpose:** Track academic research in computer science and related fields.

**API:** [arXiv API](https://arxiv.org/help/api)

**Output:** `.pollard/sources/research/YYYY-MM-DD-<query-slug>.yaml`

**Features:**
- Category filtering (cs.AI, cs.CL, etc.)
- Date sorting
- PDF URL extraction

**Configuration:**
```yaml
arxiv:
  enabled: true
  queries:
    - "large language model agents"
    - "code generation neural network"
  categories:
    - cs.AI
    - cs.CL
    - cs.SE
  max_results: 30
  output: sources/research
```

**Rate Limits:** 1 request per 3 seconds (enforced by Pollard).

**Common Categories:**
| Category | Description |
|----------|-------------|
| cs.AI | Artificial Intelligence |
| cs.CL | Computation and Language (NLP) |
| cs.SE | Software Engineering |
| cs.LG | Machine Learning |
| cs.PL | Programming Languages |
| cs.HC | Human-Computer Interaction |

---

### competitor-tracker

**Purpose:** Monitor competitor product changes, changelogs, and documentation.

**API:** HTML scraping (no API required)

**Output:** `.pollard/insights/competitive/YYYY-MM-DD-<competitor>.yaml`

**Features:**
- Changelog monitoring
- Documentation scraping
- GitHub release tracking

**Configuration:**
```yaml
competitor-tracker:
  enabled: true
  targets:
    - name: "Cursor"
      changelog: "https://cursor.com/changelog"
      github: "getcursor/cursor"
    - name: "Codeium"
      docs: "https://codeium.com/docs"
    - name: "Windsurf"
      changelog: "https://windsurf.ai/changelog"
  output: insights/competitive
```

**Rate Limits:** Polite scraping (1 request per target, reasonable intervals).

---

## General-Purpose Hunters

### openalex

**Purpose:** Multi-discipline academic research across 260M+ works.

**API:** [OpenAlex API](https://docs.openalex.org/)

**Output:** `.pollard/sources/openalex/YYYY-MM-DD-openalex.yaml`

**Features:**
- 4-stage pipeline: Search → Fetch → Synthesize → Score
- Citation counts
- Open access detection
- Topic classification
- Author information
- PDF URL extraction (when available)

**Configuration:**
```yaml
openalex:
  enabled: true
  queries:
    - "machine learning code generation"
    - "human computer interaction programming"
  max_results: 100
  output: sources/openalex
```

**Rate Limits:**
| Auth Status | Limit |
|-------------|-------|
| Unauthenticated | 10 req/s |
| With OPENALEX_EMAIL | 10 req/s + polite pool (100k/day priority) |

**Environment Variable:** `OPENALEX_EMAIL` (your email for polite pool access)

**Use Cases:**
- Cross-disciplinary research
- Finding foundational papers
- Exploring emerging research areas

---

### pubmed

**Purpose:** Biomedical and medical literature search.

**API:** [NCBI E-utilities](https://www.ncbi.nlm.nih.gov/books/NBK25501/)

**Output:** `.pollard/sources/pubmed/YYYY-MM-DD-pubmed.yaml`

**Features:**
- ESearch + EFetch two-step retrieval
- MeSH term extraction
- Author and journal info
- Abstract retrieval
- DOI linking

**Configuration:**
```yaml
pubmed:
  enabled: true
  queries:
    - "cognitive load programming"
    - "developer productivity"
  max_results: 50
  output: sources/pubmed
```

**Rate Limits:**
| Auth Status | Limit |
|-------------|-------|
| Without API key | 3 req/s |
| With NCBI_API_KEY | 10 req/s |

**Environment Variable:** `NCBI_API_KEY` (get free at [NCBI](https://www.ncbi.nlm.nih.gov/account/settings/))

**Use Cases:**
- Health tech products
- Ergonomic research
- Cognitive science for UX
- Medical informatics

---

### usda-nutrition

**Purpose:** Food and nutrition data including allergens.

**API:** [USDA FoodData Central](https://fdc.nal.usda.gov/api-guide.html)

**Output:** `.pollard/sources/nutrition/YYYY-MM-DD-usda.yaml`

**Features:**
- 1.4M+ food items
- Nutrient profiles
- Ingredient lists
- Allergen detection
- Brand/manufacturer data

**Configuration:**
```yaml
usda-nutrition:
  enabled: true
  queries:
    - "protein powder"
    - "gluten free bread"
  max_results: 100
  output: sources/nutrition
```

**Rate Limits:** 12,000 req/hr with API key (required).

**Environment Variable:** `USDA_API_KEY` (**required** - get free at [FoodData Central](https://fdc.nal.usda.gov/api-key-signup.html))

**Use Cases:**
- Food/recipe apps
- Nutrition tracking
- Dietary restriction features
- Allergen warnings

---

### legal

**Purpose:** US court decisions and legal opinions.

**API:** [CourtListener API](https://www.courtlistener.com/help/api/)

**Output:** `.pollard/sources/legal/YYYY-MM-DD-legal.yaml`

**Features:**
- 9M+ court opinions
- Federal and state courts
- Citation tracking
- Date filtering

**Configuration:**
```yaml
legal:
  enabled: true
  queries:
    - "software patent"
    - "intellectual property API"
  max_results: 50
  output: sources/legal
```

**Rate Limits:** Generous with API key.

**Environment Variable:** `COURTLISTENER_API_KEY` (**required** - register at [CourtListener](https://www.courtlistener.com/))

**Use Cases:**
- Legal tech products
- Compliance research
- Terms of service analysis
- Patent/IP investigation

---

### economics

**Purpose:** Global economic indicators and data.

**API:** [World Bank API](https://datahelpdesk.worldbank.org/knowledgebase/topics/125589)

**Output:** `.pollard/sources/economics/YYYY-MM-DD-economics.yaml`

**Features:**
- Country-level data
- Time series indicators
- Development metrics
- Trade statistics

**Configuration:**
```yaml
economics:
  enabled: true
  queries:
    - "software industry gdp"
    - "technology investment"
  max_results: 50
  output: sources/economics
```

**Rate Limits:** Polite use expected (no strict limits documented).

**Use Cases:**
- Market sizing
- Geographic expansion research
- Economic trend analysis
- B2B product positioning

---

### wiki

**Purpose:** Entity lookup and general knowledge.

**API:** [Wikipedia](https://www.mediawiki.org/wiki/API:Main_page) + [Wikidata](https://www.wikidata.org/wiki/Wikidata:Data_access)

**Output:** `.pollard/sources/wiki/YYYY-MM-DD-wiki.yaml`

**Features:**
- Entity summaries
- Structured data from Wikidata
- Cross-language linking
- Category relationships

**Configuration:**
```yaml
wiki:
  enabled: true
  queries:
    - "version control system"
    - "integrated development environment"
  max_results: 20
  output: sources/wiki
```

**Rate Limits:** 5 req/s recommended.

**Use Cases:**
- Entity disambiguation
- Background research
- Competitive landscape mapping
- Feature naming research

---

## Framework Documentation Hunter

### context7

**Purpose:** Fetch up-to-date documentation for frameworks and libraries.

**API:** [Context7 MCP](https://context7.com/) - Documentation for 100+ popular libraries.

**Output:** `.pollard/sources/context7/YYYY-MM-DD-context7.yaml`

**Features:**
- Library name resolution (e.g., "react" → "facebook/react")
- Topic-based documentation queries
- Supports React, Vue, Angular, Tailwind, Prisma, and 100+ more libraries
- Current, version-specific documentation

**Configuration:**
```yaml
context7:
  enabled: true
  queries:
    - "react hooks"
    - "tailwind responsive design"
    - "prisma relations"
  max_results: 10
  output: sources/context7
```

**Rate Limits:** 10 req/s (polite use).

**Use Cases:**
- Framework documentation lookup
- Learning new libraries
- Best practices research
- API reference gathering

**Supported Libraries (examples):**
- Frontend: React, Vue, Angular, Svelte, Solid
- CSS: Tailwind, Bootstrap, Material UI
- Backend: Express, Fastify, NestJS, Django, Rails
- Databases: Prisma, Drizzle, Mongoose, SQLAlchemy
- Build: Vite, Webpack, esbuild, Turbopack
- And 100+ more...

---

## Agent-Native Hunter

### agent

**Purpose:** Use the user's AI agent (Claude, Codex, etc.) as the primary research capability.

**Why Agent-Native:**
- Agents can synthesize complex information
- Agents understand context better than keyword APIs
- Agents can follow research threads dynamically
- API hunters supplement but don't replace agent judgment

**Configuration:**
```yaml
agent:
  enabled: true
  command: "claude"
  args: ["--print", "--dangerously-skip-permissions"]
  parallelism: 2
  timeout: 5m
```

**Usage via Orchestrator:**

The `ResearchOrchestrator` in `internal/pollard/api/` intelligently selects hunters based on PRD content:

```go
// Agent analyzes PRD and determines which hunters to use
selections := scanner.SuggestHunters(vision, problem, requirements)

// For complex research, agent conducts the investigation
result, err := scanner.IntelligentResearch(ctx, vision, problem, requirements)
```

---

## Rate Limit Reference

| API | Unauthenticated | Authenticated |
|-----|-----------------|---------------|
| GitHub Search | 10 req/min | 30 req/min |
| GitHub General | 60 req/hr | 5000 req/hr |
| HackerNews | Generous | N/A |
| arXiv | 1 req/3s | N/A |
| OpenAlex | 10 req/s | 10 req/s (polite pool 100k/day) |
| PubMed | 3 req/s | 10 req/s |
| USDA | N/A | 12k req/hr |
| CourtListener | N/A | Generous |
| World Bank | Polite use | N/A |
| Wikipedia | 5 req/s | N/A |

---

## Environment Variables Summary

| Variable | Hunter(s) | Required | Description |
|----------|-----------|----------|-------------|
| `GITHUB_TOKEN` | github-scout | No | GitHub personal access token |
| `POLLARD_GITHUB_TOKEN` | github-scout | No | Alternative to GITHUB_TOKEN |
| `OPENALEX_EMAIL` | openalex | No | Email for polite pool |
| `NCBI_API_KEY` | pubmed | No | NCBI E-utilities API key |
| `USDA_API_KEY` | usda-nutrition | **Yes** | USDA FoodData Central key |
| `COURTLISTENER_API_KEY` | legal | **Yes** | CourtListener API key |

---

## Creating a Custom Hunter

See [AGENTS.md](./AGENTS.md#adding-a-new-hunter) for the complete guide. Quick checklist:

1. **Implement the Hunter interface:**
   ```go
   type Hunter interface {
       Name() string
       Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error)
   }
   ```

2. **Add rate limiting:**
   ```go
   rateLimiter: NewRateLimiter(requests, perDuration, authenticated)
   ```

3. **Register in DefaultRegistry:**
   ```go
   reg.Register(NewMyHunter())
   ```

4. **Add to config schema** (optional)

5. **Write tests:**
   ```bash
   go test ./internal/pollard/hunters -v -run TestMyHunter
   ```

---

## Pipeline Modes

Hunters that implement the 4-stage pipeline (github-scout, openalex) support these modes:

| Mode | Search | Fetch | Synthesize | Score |
|------|--------|-------|------------|-------|
| `quick` | ✅ | ❌ | ❌ | ✅ |
| `balanced` | ✅ | ✅ | Top N only | ✅ |
| `deep` | ✅ | ✅ | All items | ✅ |

Configure via CLI:
```bash
pollard scan --hunter github-scout --mode deep
```

Or in config:
```yaml
github-scout:
  mode: balanced
  pipeline:
    fetch_readme: true
    synthesize: true
    synthesize_limit: 10
    agent_cmd: "claude --print"
    agent_parallelism: 3
    agent_timeout: 2m
```

---

## Output Format

All hunters output YAML with this structure:

```yaml
query: "search terms used"
collected_at: 2026-01-26T10:30:00Z
<items>:  # varies by hunter: repos, works, articles, etc.
  - id: "unique-id"
    title: "Item Title"
    url: "https://..."
    # hunter-specific fields
    quality_score:  # if pipeline enabled
      value: 0.85
      level: "high"
      factors:
        recency: 0.9
        relevance: 0.8
      confidence: 0.75
    synthesis:      # if synthesize enabled
      summary: "AI-generated summary"
      key_features: [...]
      recommendations: [...]
```
