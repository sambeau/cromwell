---
title: "{{feature name}} — dev plan"
type: dev_plan
owner: "{{initiative-path/feature-slug}}"
---

# {{feature name}} — dev plan

## Approach

{{How the feature will be built: the shape of the implementation, the order
of work, and any structural decisions. A few paragraphs.}}

## Tasks

The task table decomposes the work. Each task is a self-contained unit an
implementer agent will build in isolation. Keep them small and ordered by
dependency. Columns are fixed: id, title, depends_on (comma-separated ids,
or empty), description.

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | {{first task}} | | {{what to build, concretely}} |
| T2 | {{second task}} | T1 | {{what to build; runs after T1}} |

## Risks

{{Optional: known hazards or unknowns. Delete if empty.}}
