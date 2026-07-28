# Design Brief — Cromwell Web App Visual Treatment

## 1. What Cromwell is (context)

Cromwell is a planning, workflow, and tracking tool for small teams building
software with AI agents. The web app is a **command centre**: a browser surface
where a human plans work, watches it progress, and reviews it. It is
**document-led** — you plan and read through documents (design notes, specs) —
and **read-heavy** with a focused set of actions, closer to Linear or Jira than
to a form-filling admin panel.

Two things to know because they shape the visuals:

- **Work is measured in tokens** (units of AI computation), never in money or
  time. Token counts and their confidence are the primary metric shown
  throughout — treat them the way a finance tool treats currency, but there is
  **no currency anywhere** in this product.
- Much of the *creating* of structure happens in a separate chat interface, so
  the web app leans toward **browsing, status, and review**, with a small,
  clear set of actions on each thing.

## 2. What we're asking for

A **visual treatment and a consistent design system** for the web app. Today it
works but looks incoherent: inconsistent typography, no icon set, weak visual
separation between blocks of data, expanding controls with no affordance, and
forms that feel like a command line with boxes.

We want **clarity, consistency, and an easy-to-learn experience**. Specifically:

- A **standard icon set** used consistently for entity types, states, and
  actions — including the expand/collapse controls that currently have none.
- **Coherent typography** — a clear hierarchy that separates app chrome
  (navigation, labels, metadata) from long-form reading content (rendered
  documents).
- **Colour and small borders/dividers** to compartmentalise data visually, so
  dense pages are scannable.
- A view on **modal pop-ups** for forms and for viewing documents — where they
  help, and where inline is better. (Your call; we're asking you to consider
  it, not mandating it.)

## 3. What we are *not* asking for

- **Not a workflow or UX re-architecture.** The information architecture below
  is a starting point, not a constraint. If rearranging it produces a better
  experience, do so — but the *capabilities* (what each page shows and can do)
  are fixed; the *presentation and interaction* are yours.
- **Not bound by the current UI.** Ignore how it looks now. Custom components
  are welcome (see §6) — e.g. a bespoke roadmap or milestone component with
  colour and progress bars, if that reads better than a plain list.

You own the user experience. We own the capabilities and the data.

## 4. Inspiration

- https://linear.app
- https://monday.com
- https://www.atlassian.com/software/jira/features

Use these as reference for density, iconography, and how they compartmentalise
information — but **you are free to create your own visual language**. We are
not asking for a clone of any of them.

## 5. Pages to design (as one consistent set)

These share one shell (navigation + breadcrumbs) and should feel like one
product. Each is described by *what it must show*, not how it should look.

| Page | Purpose / must show |
|---|---|
| **Home** | The project at a glance, work-first: what's waiting on the human, what's running, what's unestimated. Entry point into browsing. |
| **Entity page** (initiative / feature) | The core page. A rendered design document as the main body; a summary description; a side rail of status, size-in-tokens, and actions; and sections for children, documents, milestones, and recent activity. **One layout serves all entity types.** |
| **Document page** | A rendered Markdown document (headings, tables, code, task lists), read-only, with its metadata and a comment thread. |
| **Inbox** | A queue of questions awaiting a human decision, each with context and a set of answer actions. Plus a single-question view. |
| **Documents index** | A filterable list of all documents by type and status. |
| **Work view** | Token roll-ups (estimated vs actual) across the project — the "how much work" view. |
| **Milestone page** | A milestone as a checklist of its members, with completion shown two ways (see §6). |
| **Roadmap page** | An ordered sequence of milestones with their progress. |
| **Task page** | A record of one agent run: its result, a code diff, and any findings. |

## 6. Components to design together (for consistency)

Group these into one system; they recur across pages.

**Shell & navigation**
- Left navigation rail (a handful of top-level views + a browsable tree of the
  project for navigation).
- Breadcrumbs (the primary "up" control through the hierarchy).

**Entity page furniture**
- The status/size/actions **side rail**.
- **Section blocks** (children, documents, milestones, activity) — the main
  candidates for borders/dividers to compartmentalise. These currently expand
  with no icon; give disclosure a consistent affordance.

**Status & metric language** (needs the most iconographic/colour thought)
- **Lifecycle state badges** — features move through idea → ready → active →
  review → done / abandoned; documents through draft → reviewing → approved →
  superseded; milestones are open or locked. Each needs a consistent badge.
- **Size & confidence** — every item shows a token estimate and a confidence
  tier (decomposed / considered / rough), plus an explicit "unestimated"
  state. This is currently three coloured circles and a "?"; it deserves a
  proper visual treatment. Token counts are the hero metric — make them read as
  such.
- **Progress** — milestones show completion **two ways at once**: a count
  (e.g. "3 of 4 items done") and a token bar (how much of the estimated work is
  done). Design both.

**Custom data components** (freedom encouraged here)
- **Milestone** — conceptually an unordered checklist of its members, each
  done/not-done. Colour, checkboxes, and progress bars are all fair game.
- **Roadmap** — conceptually an ordered, numbered sequence of milestones. A
  bespoke component (timeline, board, coloured sequence) is welcome if it reads
  better than a plain list.
- **Token/estimate readouts** — the recurring "size + tier" chip.

**Actions & input**
- **Action buttons**, including a **disabled-with-reason** state (an action the
  user can't take yet must show *why* in plain words, not just grey out).
- **Forms** — creating and editing things. Candidate for modals (§2).
- **Modal / pop-up** pattern for forms and possibly document viewing (your
  recommendation).

**Content & feedback**
- **Rendered document body** — Markdown → HTML at comfortable reading width
  (headings, tables, code blocks, task lists, images).
- **Comment thread** on documents.
- **Empty states** — e.g. "no design document yet — attach one."
- **Banners / notices** — success, warning, error.
- **Inbox item / checkpoint card** with its answer actions.
- **Activity / audit rows** — a compact event log.
- **Live-update affordance** — regions refresh in place when the server pushes
  changes; a subtle "just updated" signal may help (optional).

**Iconography set** (used across all of the above)
- Entity types: project, initiative, feature, document, milestone, roadmap,
  task, inbox question.
- Document types: design, spec, dev-plan, note (and a few more).
- Actions: start, estimate, attach, edit, archive, abandon, approve, request
  changes, add-to-milestone, lock, copy-path.
- Structural: expand/collapse, breadcrumb separator, external link,
  ownership/membership, live-updated.

## 7. Constraints the visuals must respect

- **Human-readable everywhere.** All labels, headings, and messages are written
  in plain, full-sentence prose for a designer/product reader — no cryptic
  abbreviations. The visual language should match that tone: clear over clever.
- **Tokens, not money.** No currency symbols, no cost framing. The unit of work
  is tokens.
- **Reading matters.** The design-document body is real prose people read at
  length; it needs proper reading typography, distinct from the dense chrome
  around it.
- **Desktop-first**, but shouldn't break at narrower widths. Light/dark theme
  is your call — flag it as a decision either way.
- **Accessibility**: legible contrast, visible keyboard focus, and state never
  conveyed by colour alone (pair colour with icon/text — important for the
  tier and lifecycle systems).

## 8. Technical delivery — please read, it affects your handoff

The app is built as **server-rendered Go HTML templates with HTMX** (a small
library that swaps fragments of HTML from the server). **There is no React, no
JavaScript framework, and no build step.** CSS and assets are compiled into the
program and served directly. This changes what a useful handoff looks like.

**Please deliver the design as a self-contained HTML + CSS component library**,
not as images or a Figma-only file. Concretely:

1. **A static HTML/CSS pattern library** — one CSS file plus HTML snippets
   showing every component and page in its representative states (default,
   hover, disabled-with-reason, empty, error, and mid-update). Because the app
   is HTML + CSS, developers can lift your markup structure and styles almost
   directly into the templates. This is by far the highest-fidelity, lowest-
   drift handoff.
2. **Design tokens as CSS custom properties** (`:root { --… }`) — colour, type
   scale, spacing, border widths, radii, shadows — as the single source of
   truth the templates reference.
3. **Icons as inline SVG** (or an SVG sprite). They must be embeddable in the
   app with **no external requests** — no icon-font CDN, no remote asset hosts.
4. If you use a custom webfont, it must be **self-hostable/embeddable** (again,
   no CDN dependency).

Two more technical notes so nothing surprises you:

- Components must look right when **injected as HTML fragments** (HTMX swaps
  pieces of the page in), so styling can't depend on a full-page JavaScript
  init.
- If you go with modals, the app can use the browser's native `<dialog>`
  element — style that rather than assuming a custom JS modal.

**Figma is welcome for exploration and concept sign-off, and static images are
fine for early direction** — but the *binding deliverable* should be the
HTML/CSS pattern library + tokens + SVG icons, so the design lands in the
product without being re-interpreted.

## 9. What "done" looks like

- One coherent visual language applied across every page and component in §5–6.
- A consistent icon set, including disclosure/expand affordances.
- A typographic system that separates chrome from reading content.
- Colour and borders that compartmentalise dense data and make it scannable.
- A recommendation on modals for forms and document viewing.
- Delivered as a self-contained HTML/CSS pattern library with design tokens and
  embeddable SVG icons.

We're optimising for **clarity, consistency, and learnability**. Where you see a
better UX than what's described here, take it — tell us what and why.
