---
name: autarch:research
description: Run Pollard research on a topic or PRD
argument-hint: "[topic or PRD-ID]"
---

# Run Research

Execute Pollard research to gather intelligence on a topic, inform a PRD, or explore a competitive landscape.

## Usage

```bash
# Research a topic
/autarch:research "OAuth 2.0 best practices"

# Research for a specific PRD
/autarch:research PRD-001

# Research with specific hunters
/autarch:research --hunters github-scout,openalex "machine learning frameworks"
```

## Available Hunters

| Hunter | Description | Best For |
|--------|-------------|----------|
| `github-scout` | GitHub repository search | Open source, libraries, code patterns |
| `openalex` | Academic paper search | Research papers, citations |
| `pubmed` | Medical/biomedical literature | Healthcare, drugs, clinical |
| `context7` | Framework documentation | API docs, library usage |
| `courtlistener` | Legal case search | Law, regulations, compliance |
| `patents-view` | USPTO patent database | IP, inventions |
| `web-searcher` | General web search | Broad topics |

## Steps

1. Parse input to determine research mode:
   - Topic string → general research
   - PRD-ID → extract topics from PRD requirements
2. Suggest optimal hunters based on topic keywords
3. Generate research brief with:
   - Research questions
   - Target sources
   - Expected deliverables
4. Execute ranger agent to run hunters
5. Collect and validate findings
6. Write results to `.pollard/insights/`
7. Generate summary report
8. Link findings to PRD (if PRD-ID provided)

## Output

Research is saved to `.pollard/insights/{id}.yaml` with:
- Sources consulted
- Key findings with evidence
- Recommendations for action
- Links to related PRDs/features

## After Research

- Use findings to inform PRD requirements
- Run `/autarch:prd PRD-{id}` to update the PRD with research
- Run `/autarch:tasks` to generate implementation tasks
