# The writing guide

How we plan, structure, and write documentation. This covers everything from deciding what
a document is for through to where the commas go.

For removing the characteristic tics of machine-generated prose, see
[editing-ai-prose.md](editing-ai-prose.md). Use that as an editing pass over a draft written
to this guide, not as a replacement for it.

**Spelling and style.** British spelling throughout — *-ise*, *-our*, *-re*. Keep the serial
comma. For anything not covered here, follow the New Oxford Style Manual.

---

## Table of contents

**Before you write**

1. [Who you're writing for](#1-who-youre-writing-for)
2. [Planning a document](#2-planning-a-document)

**Structure**

3. [The inverted pyramid](#3-the-inverted-pyramid)
4. [Document structure](#4-document-structure)
5. [Sections and paragraphs](#5-sections-and-paragraphs)
6. [Writing to be scanned](#6-writing-to-be-scanned)
7. [Tables, figures, and examples](#7-tables-figures-and-examples)

**Prose**

8. [Voice and tone](#8-voice-and-tone)
9. [Sentences](#9-sentences)
10. [Verbs](#10-verbs)
11. [Word choice](#11-word-choice)
12. [Abbreviations](#12-abbreviations)
13. [Punctuation](#13-punctuation)
14. [Capitalisation](#14-capitalisation)
15. [Inclusive and accessible writing](#15-inclusive-and-accessible-writing)

**Kinds of document**

16. [Procedures and instructions](#16-procedures-and-instructions)
17. [Document types](#17-document-types)
18. [Across documents](#18-across-documents)

**Checking**

19. [Checklist](#19-checklist)

---

## 1. Who you're writing for

Before you write, answer four questions:

- **Who reads this?** What's their role, and what are they trying to do?
- **What do they already know?** Don't explain what they understand. Don't assume what they
  don't.
- **How will they read it?** Straight through, like a guide, or dipping in, like a reference?
- **What must they leave with?** Focus on that, and cut the rest.

### Split your writing by its reader

This is the rule that matters most here, and it isn't optional.

**Human-facing writing** — interface copy, headings and titles, initiative and feature
descriptions, notices, design documents, decision records — must be clear descriptive prose
written for a designer or product reader. Full sentences. Technical terms and acronyms
spelled out on first use. Titles that say what a thing *is*, in plain words.

**Agent-facing writing** — specifications and development plans — may be as terse, dense,
and jargon-heavy as it needs to be, because agents read it.

Terse condensed prose in a human-facing title or description is a defect, not a style
preference. Gloss internal shorthand — gate names, tool names, abbreviations — on first use
in anything a human reads, including design documents.

This applies to anything that *generates* human-facing text too. A feature that writes a
description for a person must write designer-friendly prose, not summarise a specification
in specification language.

### The two human audiences

**Technical designers, managers, and hobbyists.** They understand technology conceptually
and make product and architectural decisions. They use tools without necessarily building
them. They want to know what something does, why it exists, and when to use it. They read
openings and summaries and rarely go deeper.

**Skilled developers.** They build. They want to know how to use something, how it works
inside, and how to extend it. They scan for code, commands, and configuration, and read deep
when they need specifics.

Both are assumed to use Git, work at a command line, and read structured technical text
comfortably. State any assumption beyond that.

You don't need separate documents for each. The inverted pyramid handles it — see §3.

---

## 2. Planning a document

Don't start with a blank page and write from the top.

**Define the job in one sentence.** If you can't write it, you're not ready to write the
document.

> This document explains how a feature moves from idea to done, and what has to be true at
> each gate.

**Decide what kind of document this is.** Different types need different structures. See
§17.

**List the three to five things the reader must understand** after reading. These are the
spine. Everything in the document should support one of them; anything that doesn't, cut.

**Outline in sentences, not labels.** Sentences need verbs, and verbs need meaning.

- *Configuration* — tells you nothing.
- *The user creates a configuration file that controls which features are enabled* — tells
  you what the section says, and shows you where you don't yet know.

**Draft the examples and figures first.** Code examples, diagrams, tables, and transcripts
are often the most valuable part of a document. The prose exists to connect and explain
them.

**Write the summary last.** Summarise what you actually wrote, not what you planned to.

**Verify facts against the implementation.** The code is the source of truth for what a
system does. Design documents give you concepts and intentions; check the behaviour against
what's built. Documentation that contradicts the implementation is simply wrong.

---

## 3. The inverted pyramid

The most important information comes first. Supporting detail follows in descending order of
importance. A reader who stops at any point has already absorbed the most valuable content
up to there.

This operates at three levels at once:

**Content.** Broad concepts before specific details. A section opens with its key point,
then elaborates. A document opens with what it is and why it matters, then goes deeper.

**Tone.** More conversational at the top, more precise as detail increases. An introduction
can be relaxed; a configuration reference must be exact.

**Technical depth.** Concepts without jargon at the top. Commands, schemas, and code deeper
down, where the reader has chosen to go looking.

The payoff is that one document serves both audiences. A designer reads the opening of each
section and gets the concepts and the reasoning. A developer reads further into the same
sections and gets the specifics. Neither needs a separate version.

When a section is purely technical, say so at the top: *"This section covers the
command-line options for the server."* The designer skips it, the developer dives in, and
neither wastes time.

---

## 4. Document structure

### The opening

The first thing a reader sees answers: what is this, and why should I care? Three to five
sentences is usually enough.

1. **What this is** — one sentence identifying the document and its subject.
2. **Who it's for** — who should read it, and who can skip it.
3. **What's in it** — the payoff for reading.

### Navigation

For anything longer than a few screens:

- Include a table of contents with links.
- Arrange sections in the order the reader needs them, not the order you wrote them.
- Follow the reader's path: context before action, concepts before specifics, common cases
  before edge cases.

### Introductions to longer documents

Design documents, specifications, and guides need a proper introduction. Follow this arc:

1. **Context** — what the reader needs to know to understand the problem.
2. **Gap** — what's missing, broken, or unsolved.
3. **Goal** — what this document addresses.
4. **Approach** — how the document, or the work it describes, tackles it.

Keep it tight. Many readers skip to the final paragraph of an introduction, so make sure
that paragraph states the goal and approach well enough to stand alone.

Summarise the problem in one short paragraph. If you can't, you may not understand it well
enough yet.

### The closing

End with one of:

- **A summary**, for longer documents — one short paragraph restating the key points.
- **Next steps**, for guides and tutorials.
- **Related documents**, when there's a clear follow-on.

Don't introduce new information in a closing section.

---

## 5. Sections and paragraphs

Every section is a miniature inverted pyramid.

**Open with the point.** The first sentence states what the section is about and why it
matters. A reader who reads only opening sentences should understand the document's
structure and key messages.

**Follow the pyramid.** The concept first, then the common and important details, then edge
cases and advanced material.

**One idea per section.** If a section covers two topics, split it. If you can't write a
heading that accurately describes the section, the section lacks focus.

**Paragraphs are units of thought, not visual breaks.** A good paragraph has a topic
sentence, related ideas each in their own sentence, and a logical order — general to
specific, cause to effect, problem to solution, or chronological.

Keep paragraphs to three to seven lines. Single-sentence paragraphs are fine for emphasis.
No walls of text.

**Transitions.** Use linking words to show how ideas relate: *however, therefore, for
example, in contrast, as a result, instead*. Don't overdo it — if the connection between two
sentences is obvious, a signpost is clutter. Between sections, a clean heading break is
often clearer than a forced transitional sentence.

**Parallel structure.** When listing, comparing, or contrasting, keep the grammar
consistent.

Inconsistent:

> The tool supports:
> - Scanning for vulnerabilities
> - To generate reports
> - Audit trail management

Consistent:

> The tool supports:
> - Scanning for vulnerabilities
> - Generating reports
> - Managing audit trails

---

## 6. Writing to be scanned

Readers don't read; they scan. Design every page so that someone who never reads a full
paragraph can still find what they need.

### Headings

Headings are the skeleton. If a reader reads only the headings, they should understand the
structure and scope.

- Think of them as an outline. Each heading introduces its topic specifically.
- Keep them short, and front-load the important words.
- Use sentence case. No full stop at the end.
- Use parallel structure across headings at the same level. If one is a verb phrase, they
  all should be.
- Use two or three levels at most. If you need more, rethink the structure.
- Never put two headings in a row with no text between them.
- For how-to content, consider task-oriented headings: *Create a configuration file*, *Run
  the test suite*.

### Lists

Lists turn dense text into something absorbed at a glance.

- **Bulleted** for items sharing a category with no required order.
- **Numbered** for sequential steps or ranked items.
- **Definition lists** for terms with explanations. Bold the term, then a full stop, then the
  definition.
- At least two items. Aim for no more than seven.
- Parallel grammatical structure throughout.
- Capitalise the first word of every item.
- Full stops only when items are complete sentences — or when any single item is.
- No semicolons, commas, or conjunctions at the ends of items.

### White space

White space is a readability tool, not wasted room.

- Extra space above a heading signals a new topic.
- Short paragraphs with space between them are more inviting than dense blocks.
- Don't add blank lines to force spacing, especially in anything rendered responsively.

### Front-loading

Lead with keywords in headings, table entries, list items, and paragraph openings. The
scanner's eye hits the first few words of each line, so make those words count.

---

## 7. Tables, figures, and examples

### Tables

- Don't use a table where a list would do. Tables are for structured comparisons and
  multi-attribute data.
- Put the identifying information — names, commands, terms — in the leftmost column.
- Make entries parallel in structure.
- Use sentence case in headers and cells.
- Give every table a caption or an introducing sentence, so its purpose stands alone.
- Never leave a cell blank. Use *None* or *Not applicable*.
- Keep tables narrow: few columns, short cell text.

### Figures and diagrams

- Use a diagram where it communicates more efficiently than prose — architecture, data flow,
  state machines.
- Give every figure a caption that can be understood without the surrounding text.
- Introduce each figure with a complete sentence, not one ending in a colon.

### Code examples

- Every reference entry and every concept involving code gets at least one example.
- Keep examples minimal. Show the thing being documented and nothing else.
- Use realistic values rather than `foo` and `bar` where realism aids understanding.
- If an example needs context — a specific file, a running server — state the prerequisites.

### Emoji

Don't use emoji in technical documentation: not in headings, not in bullets, not in running
prose. In particular, avoid ✅ and ❌ in tables — write *Yes* and *No*, which are
unambiguous, translatable, and readable by a screen reader.

---

## 8. Voice and tone

### Write like you speak

Good technical writing sounds like a knowledgeable colleague explaining something clearly.
Read it aloud. If it sounds stiff, rewrite it.

- Use everyday words: *use* over *utilise*, *remove* over *eliminate*, *tell* over *inform*.
- Use contractions — *it's, you'll, you're, we're, don't, can't, let's*. They signal a human
  voice.
- Don't mix a contraction and its spelled-out form in the same document. Pick one.
- Never contract a noun and a verb. *The system's running* → *The system is running*.
- Avoid awkward contractions: *there'd, it'll, they'd*.

### Address the reader directly

Second person — *you, your* — is the default. It keeps the focus on the reader and naturally
produces active voice.

- Drop *you can* when the sentence works without it. *You can store files online* → *Store
  files online*.
- Avoid *we* as a corporate voice. If *we recommend* is unavoidable, use it sparingly, and
  only where it avoids an awkward passive.
- Use first person only in interface elements where the user is asserting control: *Remember
  my password*, *I agree to the terms*.

### Be friendly, not flippant

Warmth doesn't cost credibility. Technical content can be human and still precise. Go easy
on exclamation marks — save them for something genuinely worth one, if that ever happens.

---

## 9. Sentences

**One idea per sentence,** or at most two closely related ones. If you're joining clauses
with *and*, *but*, and *so* more than once, split the sentence.

**Lead with what matters.** Readers scan the beginnings of sentences, so the opening words
carry the most weight.

Before:

> The Recommended Charts command on the Insert tab recommends charts that are likely to
> represent your data well.

After:

> Create the right chart for your data with Recommended Charts on the Insert tab.

**Keep subject and verb together.** Long modifiers between them force the reader to hold too
much in memory.

Weak:

> The file, which was created during yesterday's build and subsequently modified by the CI
> pipeline, failed validation.

Better:

> The file failed validation. It was created during yesterday's build and then modified by
> the CI pipeline.

**Watch the length.** Sentences over 25–30 words deserve a second look. Long sentences aren't
always wrong, but each one should earn its length. More than two embedded clauses means
split it.

---

## 10. Verbs

### Prefer active voice

In active voice the subject performs the action. It's direct, clear, and usually shorter.

| Active | Passive |
|--------|---------|
| The installer copies the files. | The files are copied by the installer. |
| Divide your document into sections. | Your document can be divided into sections. |

### Use passive voice deliberately

Passive is the right choice when:

- You want to avoid blaming the reader, especially in error messages. *That site can't be
  found* rather than *You entered an invalid URL*.
- The receiver matters more than the actor. *The transaction is committed when the user
  selects OK.*
- The actor is unknown or irrelevant.

If you can't name a reason for the passive, switch to active.

### Unbury your verbs

Nominalisations make sentences sluggish. Turn the noun back into a verb.

| Smothered | Direct |
|-----------|--------|
| perform an installation | install |
| make a determination | determine |
| carry out an evaluation of | evaluate |
| establish connectivity | connect |

Never combine a smothered verb with the passive. *An installation was performed by the team*
→ *The team installed the software*.

### Choose the right tense

- **Present** for how things work now, established facts, and current behaviour. This is the
  default.
- **Past** for what happened: steps taken, test results, changes in a previous release.
- **Imperative** for instructions: *Enter a file name, then save the file.*

Don't switch tense within a paragraph unless the context genuinely shifts.

---

## 11. Word choice

### Prefer short, familiar words

| Use | Not |
|-----|-----|
| use | utilise, make use of |
| start | commence, initiate |
| end | terminate, cease |
| to | in order to, as a means to |
| also | in addition |
| because | due to the fact that |
| about | approximately, with regard to |
| if | in the event that |
| can | is able to |

### One word, one meaning

Use each term consistently for one concept. If you call it a dashboard in one section, don't
switch to control panel in the next. And don't use one word for two different things.

### Don't repurpose common words

Use words in their most familiar sense. Don't coin terms from existing words (*bucketise*),
and don't give ordinary words new meanings (*graveyard* for *archive*). Don't verb nouns or
nominalise verbs: *affect performance*, not *impact performance*; *download the paper*, not
*get the download*.

### Cut the filler

- Remove unnecessary adverbs: *very, quite, really, easily, simply, basically, just*.
- Replace vague quantifiers with specifics: *several* → *three*, *some* → *15%*, *many* →
  *most*.
- Strip adjectives that add nothing. If every feature is *powerful* and every integration is
  *seamless*, neither word means anything.

### Use technical terms with care

- Don't reach for a technical term where a common word works.
- Where a technical term is clearest, define it in context on first use.
- Use one term consistently for one concept. Don't alternate synonyms.
- Know your audience. Industry terminology is fine for professional readers, but check
  standard usage against an authoritative source.

### Avoid jargon

Jargon is shorthand among specialists and a wall to everyone else. Prefer the more familiar
term where one exists. Business and marketing jargon is never acceptable: *leverage* meaning
*use*, *synergise*, *paradigm shift*, *circle back*.

A quick test: if a reviewer questions a term, it's probably jargon. If it isn't in a standard
dictionary, spell it out or replace it.

---

## 12. Abbreviations

- Keep them to a minimum. Each one adds cognitive load.
- Define at first use: *the command-line interface (CLI) supports the following flags*. After
  that, use the abbreviation alone.
- Don't use abbreviations in headings unless they're universally understood — API, URL, HTTP,
  HTML.
- Don't introduce an abbreviation you'll use once. Spell it out.
- Latin abbreviations (*e.g., i.e., etc.*) are fine in parentheses. In running prose, prefer
  *for example*, *that is*, and *and so on*.

Internal shorthand — gate names, tool names, project abbreviations — must be glossed on
first use in anything a human reads.

---

## 13. Punctuation

### The serial comma

Always use it before the conjunction in a list of three or more.

*Android, iOS, and Windows* — not *Android, iOS and Windows*.

### Commas

- After an introductory phrase: *With the new CLI, you can deploy in seconds.*
- To join independent clauses with a conjunction: *Select Options, and then select Enable.*
- Between adjectives modifying the same noun, where you could reverse them or join them with
  *and*: *a fast, reliable connection.*
- Not to join independent clauses without a conjunction. Use a semicolon, or two sentences.
- Not between verbs in a compound predicate: *The program evaluates your system and copies
  the files.*

### Full stops

- One space after. Never two.
- No full stops in headings, subheadings, or interface labels.
- In lists: use them if any item is a complete sentence; skip them if every item is a short
  phrase of three words or fewer.

### Semicolons

Semicolons signal complexity. Before using one, try splitting the sentence or converting it
to a list.

Use them between two independent clauses not joined by a conjunction, and to separate items
in a series that already contains commas.

### Colons

- Don't end a heading with a colon.
- A colon within a heading can separate a title and subtitle.
- Lowercase the word after a colon unless it's a proper noun.
- When introducing a list, use a colon if the introducing text refers to the items directly
  — "the following", "these", a stated number. Otherwise, use a full stop.

### Dashes and hyphens

Not interchangeable.

- **Em dash (—)** sets off a break in thought or a parenthetical remark. No spaces around it.
  One per sentence at most, and one pair per paragraph at most — beyond that, convert them
  to commas, parentheses, colons, or full stops. Never in headings.
- **En dash (–)** for ranges of numbers and dates: *2020–2024*, *pages 15–32*. No spaces.
- **Hyphen (-)** joins compound modifiers before a noun: *real-time updates*.

### Apostrophes

- Possessives: *the server's configuration*, *users' passwords*.
- Contractions: *don't, it's, you're*.
- Not for possessive *its*: *The system restarted its services.*
- Not for plurals: *APIs*, never *API's*.

### Quotation marks

Use straight quotes, not curly ones, to match the code. Search for and replace curly
variants — some tools introduce them silently.

### Bold

Bold should be rare. Use it to introduce a term for the first time, or for interface element
names. Don't bold every occurrence of a concept, and don't bold phrases for emphasis in
running prose.

---

## 14. Capitalisation

### Default to sentence case

Capitalise only the first word and any proper nouns. This is the rule for headings, titles,
subheadings, interface labels, and list items.

Correct:

> Set up your development environment

Wrong:

> Set Up Your Development Environment

### When to capitalise

- The first word of a sentence or heading.
- Proper nouns: product, service, and brand names, people, places.
- The first word after a colon in a title.

### When not to

- Common technology terms: *cloud computing, open source, machine learning*.
- The spelled-out form of an acronym, unless it's a proper noun.
- Never ALL CAPS for emphasis. Use *italic*, sparingly.

### Title case

Reserve title case for what requires it: book titles, article titles in citations, and
product names. When using it, capitalise everything except articles (*a, an, the*),
prepositions of four letters or fewer (*on, to, in, of*), and conjunctions (*and, but, or,
nor, yet, so*) — unless they're the first or last word.

---

## 15. Inclusive and accessible writing

- Use gender-neutral language. Rewrite to use *you* or *they* rather than *he/she*.
- Use *they* as a singular pronoun for a person whose gender is unknown or non-binary.
- Avoid ableist language where a neutral alternative exists: *degraded* not *crippled*,
  *oversight* not *blind spot*, *validation* not *sanity check*.
- Write for screen readers. Heading hierarchy matters. Link text should describe the
  destination — *read the configuration guide*, not *click here*. Table headers should be
  specific.

---

## 16. Procedures and instructions

### Steps

- Use a **numbered list** for multi-step procedures, and a bulleted list or a single
  paragraph for one-step procedures.
- One action per step. Combining two short actions that happen in the same place is fine.
- Write each step as a complete sentence: capital letter, full stop.
- Start most steps with a verb: *Select Settings*, *Enter your password*, *Open the file*.
- Say where before you say what: *On the Design tab, select Header Row.*
- Include the final action that completes the procedure.
- If a procedure runs past one screen, consider splitting it.

### Describing interface interactions

Use verbs that work regardless of input method — keyboard, mouse, touch, or voice. Avoid
*click*, *tap*, and *swipe*.

| Verb | Use for |
|------|---------|
| **Select** | Buttons, checkboxes, options, list items, links, menu items, keys |
| **Open** | Apps, files, folders, panes |
| **Close** | Apps, dialogs, panes, tabs, notifications |
| **Go to** | Menus, tabs, pages, websites |
| **Enter** | Typing or inserting values in text fields |
| **Turn on / Turn off** | Toggle switches |
| **Clear** | Deselecting a checkbox |
| **Choose** | Where the action depends on preference, or where *Select* appears in the label |
| **Move / Drag** | Repositioning elements |

Use the angle-bracket shorthand for simple sequential paths: *Select Accounts > Other
accounts > Add an account.* Reserve it for paths where every step uses the same interaction.

### Keep procedures scannable

Use headings so readers can jump to the procedure they need, with parallel structure across
them: *Create a profile*, *Add an account*, *Delete a record*. Don't repeat the heading in
the opening sentence — if the heading says *Create a profile*, the intro shouldn't begin *To
create a profile…*

---

## 17. Document types

Different documents do different jobs and need different shapes.

### Design document

**Purpose.** Explain what a system does, why it's designed that way, and what was rejected.
Human-facing: written in plain descriptive prose.

1. **Problem and motivation** — in one short paragraph. If you can't write it, you don't
   understand the problem well enough yet.
2. **Context** — what the reader needs to know to follow the problem.
3. **The design** — the proposed approach, in enough detail to write a specification from.
4. **Alternatives considered** — what was rejected and why. Often the most valuable section
   for a future reader.
5. **Decisions** — the specific choices made, each with its reasoning.
6. **Implications** — what this means for the rest of the system.
7. **Open questions** — what isn't settled, and who settles it.

A busy reader gets the design from sections 1 to 3. Someone implementing it reads to the
end.

### Decision record

**Purpose.** Capture one decision, its reasoning, and what it costs. Human-facing.

State the decision, the context that forced it, the alternatives, the reasoning, and the
consequences — including the ones you don't like. A decision recorded without its reason
can't be revisited when circumstances change; it can only be re-argued from scratch, usually
badly.

Record supersession explicitly. A decision that's been partly overtaken by a later one is
more dangerous than one that's been fully replaced, because it still reads as current.

### Specification

**Purpose.** Define what must be built, precisely enough to verify. Agent-facing: may be
terse and dense.

1. **Problem statement** — what this solves, referencing the design.
2. **Requirements** — functional and non-functional, each with a unique identifier.
3. **Constraints** — what must not change, what's out of scope.
4. **Acceptance criteria** — testable conditions for done. Each one verifiable.
5. **Verification plan** — how each criterion will be checked: test, inspection, or
   demonstration.

Every requirement needs an identifier, because everything downstream cites it. An acceptance
criterion that can't be checked isn't an acceptance criterion.

### Development plan

**Purpose.** Break a specification into buildable tasks. Agent-facing.

1. **Scope** — what this covers, referencing the specification.
2. **Task breakdown** — each task with a description, dependencies, and a size.
3. **Dependency graph** — what depends on what, and what can run in parallel.
4. **Risks** — what could go wrong, and the mitigations.
5. **Verification approach** — how the plan confirms the specification is met.

Each task must be small and self-contained enough for one agent to build alone. Decomposition
quality predicts the outcome more strongly than anything else in the chain.

### Research note

**Purpose.** Record findings, with their sources and how much to trust them.

State the question, the sources, the findings, and — explicitly — the strength of the
evidence for each. Separate what a source found from what you conclude from it. A finding
without a source is an opinion; label it as one.

### Walkthrough

**Purpose.** Record what actually happened during a run: what worked, what broke, what was
learned.

Be honest, including about the bugs. A walkthrough that records only successes is worthless
as evidence and misleading as history.

### README

**Purpose.** First contact. What this is, why you'd use it, how to start.

1. Title and one-line description.
2. Why this exists — the problem it solves, in two or three sentences.
3. Quick start — the fastest path to a working result.
4. Key concepts — brief definitions of terms the reader needs. Link out for detail.
5. Usage examples — two or three concrete common tasks.
6. Configuration — the most important options only; link to a full reference.
7. Links — to the manual, reference, and contributing guide.

### Guide

**Purpose.** Walk someone from nothing to a working result. A guided journey with a specific
outcome.

1. What you'll achieve — state the goal so the reader knows what success looks like.
2. Prerequisites — tools, accounts, knowledge. Be specific.
3. Steps — numbered, sequential, one action each.
4. Verification — how to confirm each major step worked. Don't make the reader proceed on
   faith.
5. Next steps — where to go from here.

Explain *why* a step is needed, not just what to do. It builds a mental model, and developers
appreciate it as much as anyone.

### Reference

**Purpose.** Exhaustive and optimised for lookup. The reader knows what they want.

Organise by the thing being referenced — commands, functions, configuration keys — not by
workflow. Use a consistent format for every entry: name, description, syntax, parameters,
examples, notes. Keep prose minimal; accuracy and completeness matter more than readability.
Every entry gets at least one example.

---

## 18. Across documents

### Link, don't repeat

Each concept has a home document. Others reference it briefly and link there for the full
account. Duplicate only what each document needs to stand on its own.

If you find yourself copying a paragraph from one document into another, stop. Write a
one-sentence summary and link to the source.

### Show, don't explain

Where you can, demonstrate with a concrete example rather than describing in prose. Three
lines of code or a short terminal transcript beats a paragraph.

### Be honest about what things cost

Documentation must be factual about what something does well, what it costs in time and
complexity, and when it isn't the right choice. Trust is worth more than persuasion.

### Consistent terminology

One term for one concept, across every document. Define it in its home document and use it
everywhere else. Don't invent synonyms for variety.

---

## 19. Checklist

Use this when reviewing a document before it ships.

### Planning

- [ ] The document's purpose fits in one sentence.
- [ ] The document type is identified and its structure followed.
- [ ] The audience is identified, and any assumption beyond the shared baseline is stated.
- [ ] Three to five key messages are clear.
- [ ] Examples, figures, and tables were drafted before the prose.
- [ ] Facts are verified against the implementation, not only against design documents.

### Register

- [ ] Human-facing text is plain descriptive prose in full sentences.
- [ ] Internal shorthand is glossed on first use in anything a human reads.
- [ ] Titles say what a thing is, in plain words.

### Structure

- [ ] The opening answers: what is this, who is it for, what's in it.
- [ ] The introduction follows context → gap → goal → approach.
- [ ] Sections are in the order the reader needs them.
- [ ] A table of contents is present for anything longer than a few screens.
- [ ] The most important information comes first, in the document and in every section.
- [ ] Technical depth increases as the reader goes deeper.
- [ ] Any summary was written last and reflects the final content.
- [ ] The document ends with a summary, next steps, or related links — and no new material.

### Sections and paragraphs

- [ ] Each section opens with its key point and covers one topic.
- [ ] Each paragraph opens with a topic sentence.
- [ ] Paragraphs are three to seven lines. No walls of text.
- [ ] Sentences within a paragraph follow a logical order.
- [ ] Lists and comparisons use parallel construction.

### Sentences and verbs

- [ ] Each sentence carries one idea, or two closely related ones.
- [ ] The most important information comes first in the sentence.
- [ ] Subject and verb are close together.
- [ ] No more than two embedded clauses per sentence.
- [ ] Sentences over 25–30 words have been reconsidered.
- [ ] Active voice by default; every passive has a reason.
- [ ] Nominalisations have been turned back into verbs.
- [ ] Instructions use the imperative.
- [ ] Present tense for current behaviour, past for what happened.

### Words

- [ ] Short, familiar words preferred over long ones.
- [ ] Wordy phrases replaced with single words.
- [ ] Vague quantifiers replaced with specific numbers.
- [ ] Adjectives and adverbs present only where they add meaning.
- [ ] Each term used consistently for one concept.
- [ ] Technical terms defined on first use.
- [ ] No business or marketing jargon.
- [ ] Contractions used consistently, not mixed with spelled-out forms.
- [ ] The reader is addressed as *you*.

### Headings and lists

- [ ] Headings form a readable outline on their own.
- [ ] Headings use sentence case, are short, and front-load important words.
- [ ] Headings at the same level are parallel; none ends with a full stop.
- [ ] Every list has at least two items, all parallel in structure.
- [ ] List punctuation is consistent.
- [ ] No semicolons or conjunctions at the ends of list items.

### Punctuation and capitalisation

- [ ] The serial comma is used throughout.
- [ ] One space after a full stop.
- [ ] Em dashes have no spaces and appear at most once per sentence, once per paragraph.
- [ ] En dashes for ranges; hyphens for compound modifiers.
- [ ] Straight quotes, not curly.
- [ ] Bold is rare and purposeful.
- [ ] Sentence case is the default; no ALL CAPS for emphasis.

### Tables, figures, and examples

- [ ] Tables are used only where a list won't do.
- [ ] Every table and figure has a caption or introducing sentence.
- [ ] No blank cells.
- [ ] Every concept involving code has at least one example.
- [ ] Examples are minimal and use realistic values.
- [ ] No emoji.

### Procedures

- [ ] Multi-step procedures use numbered lists.
- [ ] Each step is one action, starting with a verb.
- [ ] Location comes before the action.
- [ ] Input-neutral verbs are used — select, enter, open; not click, tap, swipe.

### Across documents

- [ ] Each concept has one home document; others link to it.
- [ ] No paragraph-length duplication between documents.
- [ ] Terminology is consistent everywhere.
- [ ] Claims about capability are factual, including about costs and limits.
