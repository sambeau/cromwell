# Walkthrough: milestones and roadmaps you can edit (SPEC-010)

**Date:** 2026-09-28
**Spec:** [SPEC-010](specs/SPEC-010-milestones-and-roadmaps-editing.md), draft
for Sam's approval
**Status:** Definition of done items 2 to 5 run in this session. The browser
half was rerun after Sam decided that locking becomes a reversible "Mark as
shipped", and the MCP half again after the chat agent gained the two shipping
tools. Sam still has to approve the spec and the build.
**Set-up:** a fresh build of `./cmd/cromwell`, a throwaway project made with
`cromwell init` in `/var/tmp/m4demo`, served on `127.0.0.1:8810` against
Postgres 16. There was **no AI provider**: `ANTHROPIC_API_KEY` was a dummy, and
nothing in this walkthrough dispatches an agent.
**Actors:** the web UI acted as `sam` (`server.ui_actor`), and the MCP facet as
`chat-agent` (`server.mcp_actor`).

The claim SPEC-010 exists to prove:

> A person can build a two-milestone roadmap for an initiative entirely in the
> browser, and the chat agent can do the same over MCP.

Both halves held.

## Part 1: in the browser

Driven by Playwright with the pre-installed Chromium
([`walk.js`](walkthrough-spec-010/walk.js)). The same script created the tree
it plans over, through the UI too: two initiatives, Authentication and Billing,
with Login form and Passkeys under Authentication and Invoices under Billing.

### 1. An initiative's plan, before anything is in it

The "own plan" section now always appears on the project and on an initiative,
with **New milestone** and **New roadmap** (FR-2.1). Before this spec it only
appeared once there was something to show, and nothing could put anything
there.

![The Authentication page's plan section, empty, with New milestone and New roadmap](walkthrough-spec-010/01-empty-plan.png)

### 2. Two milestones and a roadmap, created on the initiative's page

**New milestone** asks for a name, an optional target date and an optional
description, and says where the milestone will be planned.

![The New milestone dialog, filled in for Auth beta](walkthrough-spec-010/02-new-milestone-dialog.png)

*Auth beta*, *Auth GA* and a roadmap, *Auth plan*, were created this way. All
three belong to Authentication (D-9): they appear in its plan section and not
in the project's.

![The plan section with the roadmap and both milestones, each with an Edit button](walkthrough-spec-010/03-plan-created.png)

### 3. Adding from the member's end

On the Login form's own page, **Add to a milestone…** in the "more" menu lists
the open milestones it could join, grouped by where each is planned (FR-3.1).

![Milestones for Login form: a select with Auth beta chosen](walkthrough-spec-010/04-add-from-member.png)

The page came back with the notice "Login form was added to Auth beta.", and
the milestone now shows in the feature's rail.

### 4. Adding from the milestone's end

**Edit** beside *Auth GA* loaded the milestone editor into a `<dialog>` (FR-6).
With the search box empty, the picker lists Authentication and everything in
it: the owner's subtree (FR-3.2). Invoices, under Billing, isn't there.

![The Auth GA editor: an empty checklist and the picker showing Authentication, Login form and Passkeys](walkthrough-spec-010/06-editor-subtree.png)

**Add** on Passkeys added it without closing the dialog. Typing "invoi" into
the search box widened the list to the whole project and found Invoices, which
lives in another tree (D-12).

![The picker after searching for "invoi", showing Invoices from Billing](walkthrough-spec-010/07-editor-search.png)

A search for "beta" found the *Auth beta* milestone, and adding it made *Auth
GA* deliver everything *Auth beta* does. The checklist now held three members.

![The Auth GA editor with Passkeys, Invoices and Auth beta in its checklist](walkthrough-spec-010/08-editor-composed.png)

Invoices was then taken back out from the checklist, with the reason "Billing
ships in its own release." recorded on the audit trail. **Done** closed the
dialog, and because something had changed, the page underneath reloaded
(FR-6.4). The script checked that the dialog was gone afterwards.

### 5. Ordering the roadmap

**Edit** beside *Auth plan* opened the roadmap editor. *Auth GA* was placed
first, then *Auth beta* at the end.

![The Auth plan editor: 1. Auth GA, 2. Auth beta](walkthrough-spec-010/09-roadmap-placed.png)

**Up** on *Auth beta* swapped them. The roadmap is still a numbered list
(D-10).

![The Auth plan editor after moving Auth beta up: 1. Auth beta, 2. Auth GA](walkthrough-spec-010/10-roadmap-reordered.png)

### 6. Marking it as shipped: refused, done, reopened, done again

In *Auth beta*'s editor, **Mark as shipped** is disabled while its one feature
isn't done, and G4's reason sits beside it in a full sentence (FR-5.1). There
is no way round it.

![The Auth beta editor: Mark as shipped disabled, with the reason that its one feature isn't done](walkthrough-spec-010/11-ship-refused.png)

No AI provider was available to build Login form, so the script **marked it
done directly in the database**. That is the only step here not taken through
the UI. With Login form done, the section says what marking it as shipped
does, and that it can be reopened (FR-5.2).

![Mark as shipped, explained, with the button enabled](walkthrough-spec-010/12-ship-ready.png)

**Mark as shipped** did it. The editor now says the milestone is shipped,
offers nothing to add or take out (FR-3.5), and offers to reopen it.

![The shipped Auth beta editor](walkthrough-spec-010/13-shipped.png)

To show the undo, the script then reopened it with a reason (FR-5.4):

![Reopen this milestone, with the reason filled in](walkthrough-spec-010/14-reopen.png)

The milestone was open and editable again:

![The reopened Auth beta editor](walkthrough-spec-010/15-reopened.png)

The audit row for the reopening keeps what the shipped record held, so nothing
is lost:

```json
{"reason": "Passkeys should go out with the beta after all.",
 "snapshot": ["01a0e80d-3c89-7e44-9f1f-e41a2a5c4e14"],
 "locked_at": "2026-09-28T12:46:26.963075Z"}
```

The script then marked it as shipped again.

### 7. The result

Authentication's plan: the roadmap in its new order, *Auth beta* shipped and
*Auth GA* open.

![The plan section after the walkthrough](walkthrough-spec-010/16-plan-final.png)

The roadmap's own page shows the same order, and now has its own **Edit this
roadmap** button (SD-6).

![The Auth plan roadmap page](walkthrough-spec-010/17-roadmap-page.png)

## Part 2: over MCP

The same server, the same database, and a small JSON-RPC client
([`mcp_walk.py`](walkthrough-spec-010/mcp_walk.py)) posting to `/mcp`. This time
the plan was for the Billing initiative. The output below is trimmed only of
ids.

The handshake's instructions now mention planning, including marking a
milestone as shipped:

```
instructions: Cromwell's planning surface. You can create and shape initiatives,
features, titles, descriptions and document attachments, and plan with
milestones and roadmaps: create them, fill them, order them, and mark a
milestone as shipped when the person says it has gone out. You cannot start
work, change a gate, or run an agent — a person does that from the command
centre.

21 tools: add_milestone_member, attach_document, create_feature,
create_initiative, create_milestone, create_roadmap, get_feature,
get_initiative, get_milestone, get_roadmap, get_tree, list_documents,
list_milestones, list_roadmaps, mark_milestone_shipped, place_roadmap_entry,
remove_milestone_member, remove_roadmap_entry, reopen_milestone,
update_feature, update_initiative
```

Creating, with an owner, and one refusal:

```
→ create_feature initiative_path="billing" slug="refunds" name="Refunds" ...
   billing/refunds idea
→ create_milestone name="Billing beta" owner_path="billing" target_date="2027-01-31" ...
   owner: {'name': 'Billing', 'path': 'billing', 'type': 'initiative'}
→ create_milestone name="Billing GA" owner_path="billing"
→ create_milestone name="Oops" owner_path="payments"
  refused: there is no initiative at "payments" to plan in; call get_tree to see
  what exists, or leave owner_path out to plan at the project level
→ create_roadmap name="Billing plan" owner_path="billing"
```

Filling the milestones by name, across trees, and the loop it refuses:

```
→ add_milestone_member milestone="Billing beta" member_type="feature" member="billing/invoices"
   feature Invoices was added. → 0 of 1 items done
→ add_milestone_member milestone="Billing beta" member_type="feature" member="billing/refunds"
   feature Refunds was added. → 0 of 2 items done
→ add_milestone_member milestone="Billing GA" member_type="milestone" member="Billing beta"
   milestone Billing beta was added. → 0 of 2 items done
→ add_milestone_member milestone="Billing GA" member_type="initiative" member="auth"
   initiative Authentication was added. → 1 of 4 items done
→ add_milestone_member milestone="Billing beta" member_type="milestone" member="Billing GA"
  refused: a milestone can't contain itself, directly or through another milestone inside it
→ remove_milestone_member milestone="Billing GA" member_type="initiative" member="auth"
                          reason="Sign-in has its own release plan."
   initiative Authentication was taken out.
```

Ordering the roadmap. `place_roadmap_entry` both places and moves:

```
→ place_roadmap_entry roadmap="Billing plan" milestone="Billing GA"
   order: ['1. Billing GA']
→ place_roadmap_entry roadmap="Billing plan" milestone="Billing beta"
   order: ['1. Billing GA', '2. Billing beta']
→ place_roadmap_entry roadmap="Billing plan" milestone="Billing beta" position=1
   order: ['1. Billing beta', '2. Billing GA']
```

Reading it back. `get_milestone` tells the agent whether the milestone could
be marked as shipped now, and why not, in G4's own sentence:

```
→ list_milestones owner_type="initiative" owner_path="billing"
   billing's milestones: ['Billing GA', 'Billing beta']
→ list_milestones
   every milestone: [('Billing GA', 'billing', 'open'), ('Billing beta', 'billing', 'open'),
                     ('Auth GA', 'auth', 'open'), ('Auth beta', 'auth', 'shipped')]
→ get_milestone milestone="Billing beta"
   members: [('feature', 'billing/invoices', False), ('feature', 'billing/refunds', False)]
   shipping: {"could_mark_shipped_now": false,
              "how": "Mark it as shipped with mark_milestone_shipped when the person says
                      the release has gone out. It can be undone with reopen_milestone.",
              "why_not": "This milestone can't be marked as shipped yet, because none of its
                          2 features is done. Marking it as shipped records what actually
                          went out, so at least one has to be finished first."}
→ get_roadmap roadmap="Billing plan"
   roadmap: ['1. Billing beta (open)', '2. Billing GA (open)']
```

Marking it as shipped and reopening it (FR-7.10, DEC-004 Amendment 1). G4
refuses the agent exactly as it refuses a person, in the same sentence. No AI
provider could finish a feature, so the script marked `billing/invoices` done
directly in the database, as the browser half did for Login form:

```
→ mark_milestone_shipped milestone="Billing beta"
  refused: This milestone can't be marked as shipped yet, because none of its 2
  features is done. Marking it as shipped records what actually went out, so at
  least one has to be finished first.

(billing/invoices marked done directly in the database)

→ mark_milestone_shipped milestone="Billing beta"
   shipped at 2026-09-28T14:46:36Z - The milestone is marked as shipped. What it
   contains is now fixed; reopen_milestone undoes this if it was a mistake.
→ add_milestone_member milestone="Billing beta" member_type="feature" member="auth/passkeys"
  refused: that milestone is marked as shipped, so what it contains can't change;
  if the person wants to change it, reopen it first with reopen_milestone
→ reopen_milestone milestone="Billing beta" reason="Passkeys go out with billing after all."
   open - The milestone is open again, and what it contains is live. The history
   still shows when it was marked as shipped.
→ add_milestone_member milestone="Billing beta" member_type="feature" member="auth/passkeys"
   feature Passkeys was added. → 1 of 3 items done
→ mark_milestone_shipped milestone="Billing beta"
   shipped - The milestone is marked as shipped. ...
→ reopen_milestone milestone="Billing GA"
  refused: that milestone isn't marked as shipped, so there is nothing to reopen
```

## The audit trail

Every act from both halves, by actor. The browser's acts are `sam`'s and the
chat agent's are `chat-agent`'s. `milestone.locked` is "marked as shipped" and
`milestone.unlocked` is "reopened". *Auth beta* was shipped, reopened and
shipped again by `sam` in the browser; *Billing beta* the same by
`chat-agent` over MCP.

```
   actor    |           kind           | count
------------+--------------------------+-------
 chat-agent | milestone.created        |     2
 chat-agent | milestone.locked         |     2
 chat-agent | milestone.member_added   |     5
 chat-agent | milestone.member_removed |     1
 chat-agent | milestone.unlocked       |     1
 chat-agent | roadmap.created          |     1
 chat-agent | roadmap.entry_set        |     3
 sam        | milestone.created        |     2
 sam        | milestone.locked         |     2
 sam        | milestone.member_added   |     4
 sam        | milestone.member_removed |     1
 sam        | milestone.unlocked       |     1
 sam        | roadmap.created          |     1
 sam        | roadmap.entry_set        |     3
```

## The automated suite

`go vet ./...` is clean, and `go test -race -count=1 ./...` passes against
Postgres 16, including `TestUIBrowseAndDrive` and the render tests. The new
tests are:

| Test | What it covers |
|---|---|
| `TestOwnerScopedCreation` | FR-1.1: owners stored and audited; bad owners and blank names refused |
| `TestPlaceAndRemoveRoadmapEntries` | FR-1.3, FR-1.4: dense order, tied positions, take off |
| `TestMilestoneCannotContainItself` | FR-1.5: direct and nested loops refused |
| `TestMemberCandidates` | FR-3.3: subtree default, project-wide search, literal `%_`, the limit |
| `TestUnlockMilestone` | FR-5.4: reopening drops the snapshot, keeps it in the audit row, and allows shipping again |
| `TestG4` (extended) | FR-5.1: every refusal is a sentence about features |
| `TestPlanEditorsRender` | Every new template and its shapes; no tables; no typed paths |
| `TestUIPlanEditing` | FR-2 to FR-6 through the `/ui/*` handlers, both post styles, including ship and reopen |
| `TestMCPPlanTools` | FR-7 over `POST /mcp`, the audit actor, the `shipped` state, and shipping and reopening over MCP, including G4's refusal |
| `TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet` (updated) | The twenty-one tools by name |

## Small things noticed on the way

- The milestone progress line says "1 of 1 items". That wording comes from the
  existing `milestone-progress` partial, which this spec didn't change.
- With nothing estimated, the token bar reads "0 of 0 estimated tokens done
  (0%)". That is also existing behaviour.
- The project's unix socket path must be short. The first attempt to serve from
  a deeply nested scratch directory failed with `bind: invalid argument`,
  because the path to `.cromwell/run/cromwell.sock` was over the 108-byte limit.
  It has nothing to do with this spec, but it will catch anyone who serves a
  project from a long path.
