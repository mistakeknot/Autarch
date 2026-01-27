---
name: research-brief
description: This skill guides research brief creation and Pollard hunter selection.
---

# Research Brief Skill

## When to Use

Use this skill when:
- User wants to research a topic
- Informing a PRD with external data
- Exploring competitive landscape
- User says "research this", "find out about", "what's the state of"

## Research Brief Structure

A good research brief contains:

1. **Topic**: Clear statement of what we're researching
2. **Questions**: 3-5 specific questions to answer
3. **Sources**: Which Pollard hunters to use
4. **Deliverables**: What output is expected
5. **Success criteria**: How we know research is complete

## Hunter Selection Guide

Based on the topic, recommend appropriate hunters:

| Topic Keywords | Hunter | Why |
|----------------|--------|-----|
| code, library, open source, implementation | `github-scout` | Finds real-world code examples |
| research, study, paper, academic, scientific | `openalex` | Academic papers and citations |
| medical, health, drug, clinical, disease | `pubmed` | Biomedical literature |
| framework, API, documentation, library | `context7` | Official docs from 100+ libraries |
| legal, court, law, regulation, compliance | `courtlistener` | Court cases and opinions |
| patent, invention, IP, intellectual property | `patents-view` | USPTO patent database |

For broad topics, recommend 2-3 hunters.

## Creating the Brief

### Step 1: Clarify the Topic

Ask:
- "What specifically do you want to learn?"
- "Is this for a particular PRD or feature?"
- "What decisions will this research inform?"

### Step 2: Generate Questions

Transform the topic into 3-5 specific, answerable questions:

**Topic**: "OAuth best practices"
**Questions**:
1. What are the current OAuth 2.0 security recommendations from IETF?
2. How do major providers (Google, GitHub) implement token refresh?
3. What are common OAuth implementation mistakes?
4. What libraries are recommended for our tech stack?

### Step 3: Select Hunters

Based on the questions, recommend hunters:

```
For "OAuth best practices":
- context7: Framework documentation (OAuth libraries)
- github-scout: Real implementations to study
- openalex: Security research papers (if deep dive needed)
```

### Step 4: Define Deliverables

Specify what the research should produce:
- Summary document
- Comparison table
- Recommendations with rationale
- Links to key sources

### Step 5: Execute

Run Pollard with the brief:

```bash
pollard init  # if not already initialized
pollard scan --hunter context7 --query "OAuth 2.0 implementation"
pollard scan --hunter github-scout --query "OAuth typescript library"
pollard report
```

## Linking to PRDs

If researching for a PRD:

1. Load the PRD to understand requirements
2. Focus research on validating technical decisions
3. Save insights to `.pollard/insights/`
4. Update PRD's `research` field with insight IDs
5. Recommend requirement changes if findings warrant

## Output

Save research brief to `.pollard/briefs/BRIEF-{id}.yaml`:

```yaml
id: BRIEF-001
topic: OAuth 2.0 Best Practices
created_at: 2025-01-26T12:00:00Z
linked_prd: PRD-001  # optional

questions:
  - What are current IETF recommendations?
  - How do major providers implement refresh?
  - What are common implementation mistakes?

hunters:
  - context7
  - github-scout

deliverables:
  - Security recommendation summary
  - Library comparison table
  - Implementation checklist

status: pending  # pending, in_progress, completed
```
