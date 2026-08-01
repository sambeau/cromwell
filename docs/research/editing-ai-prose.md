# Editing machine-generated prose

A practical reference for turning AI-generated text into prose that sounds like a person
wrote it. Use it as an editing pass over a draft, alongside
[writing-guide.md](writing-guide.md) rather than instead of it.

Work through the lists below as a checklist. If a passage triggers several items at once, it
probably needs rewriting from scratch rather than fixing word by word.

> **Sources.** Wikipedia's *Signs of AI writing*, published research on lexical
> overrepresentation in language models, and practical observation from editing generated
> technical content.

---

## 1. Vocabulary

Models regress towards the statistical mean of their training data. The result is a narrow,
recognisable vocabulary that sounds authoritative and says very little.

### 1.1 Words to remove

These appear at greatly elevated rates in post-2022 text. There's always a simpler, more
precise alternative.

| Remove | Use instead |
|---------------------|----------------------------------|
| delve / dive into   | examine, explain, look at        |
| leverage            | use                              |
| utilise             | use                              |
| facilitate          | help, support, enable            |
| optimise            | improve                          |
| spearhead           | lead                             |
| amplify             | increase, strengthen             |
| bolster             | support, strengthen              |
| foster              | encourage, support               |
| garner              | get, earn, attract               |
| harness             | use                              |
| empower             | let, allow, enable               |
| streamline          | simplify                         |
| elevate             | raise, improve                   |
| underscore          | show, stress, emphasise          |
| showcase            | show, demonstrate                |
| navigate (abstract) | deal with, handle, work through  |
| embark              | start, begin                     |
| unveil              | announce, release, show          |
| unlock              | enable, allow, improve           |
| unleash             | release, enable                  |

### 1.2 Inflated adjectives and adverbs

These promise more than they deliver.

**Cut without replacement — almost always filler:** pivotal, vibrant, meticulous, seamless,
effortless, cutting-edge, groundbreaking, transformative, revolutionary, game-changing,
comprehensive, holistic, innovative, dynamic, and *robust* when it isn't describing fault
tolerance.

**Replace with specifics.** Instead of "a comprehensive solution", describe what the tool
does. Instead of "robust performance", give a number or a comparison.

### 1.3 Abstract nouns used as metaphors

A handful of metaphors are used so heavily they've stopped meaning anything.

| Avoid | Problem |
|-------------------|--------------------------------------------------------------|
| landscape         | Vague. Say what you mean: the market, the available tools.   |
| tapestry          | Almost never appropriate in technical writing.               |
| testament         | Inflated. "X shows Y" beats "X is a testament to Y".         |
| journey           | Overused to the point of parody. Say process, or effort.     |
| paradigm shift    | Rarely accurate. Describe the actual change.                 |
| ecosystem         | Fine in biology. Check whether you mean system, or tools.    |
| synergy           | Say what the combined effect actually is.                    |

### 1.4 Empty hedging

Models hedge to avoid being wrong. In technical writing, hedging sounds uncertain and wastes
the reader's time.

Cut these and make the claim directly:

- "Generally speaking" → just state it
- "It could be argued that" → argue it
- "It is worth considering" → consider it
- "To some extent" → quantify it, or cut
- "It's important to note that" → state the note
- "It should be noted that" → state it
- "It bears mentioning" → mention it
- "At its core" → cut

If you're genuinely uncertain, say so plainly. "We haven't measured this yet" is honest. "It
could potentially perhaps be the case that" is noise.

---

## 2. Phrases and sentence patterns

If any of these appear, the sentence needs rewriting. Swapping words won't fix the
underlying problem.

### 2.1 Faux-insider openers

These perform knowledge rather than stating it. They're borrowed from listicles and
marketing copy.

- "Here's what most people get wrong…"
- "Here's the thing…" / "Here's why…"
- "Here's the secret…" / "The trick is…"
- "What nobody tells you…"
- "The truth about…"
- "Let's be honest…"

**Fix.** State the fact. If there's a genuine misconception worth correcting, describe the
misconception and the correction as plain exposition.

### 2.2 Staccato rhetoric

Short fragments arranged for drama. Copywriting technique, not technical prose.

| Pattern | Example |
|-----------------------------|-------------------------------------------|
| Parallel fragments          | "No config. No setup. No hassle."         |
| Setup and reversal          | "We thought X. We were wrong."            |
| Fragment as punchline       | Ending a paragraph with a three-word punch. |
| "And" for false drama       | "It parses YAML. And it validates it."    |
| "Not X. But Y."             | "It's not a framework. It's a mindset."   |

**Fix.** Combine into a flowing sentence that explains why. "We expected X, but testing
showed Y because Z" is more useful than the dramatic pause.

### 2.3 "Not just X, but Y"

Negative parallelisms that correct a misconception the reader never held.

- "It's not just a tool — it's a philosophy."
- "Not only does it parse YAML, but it also validates schemas."
- "This isn't merely a refactor. It's a rethinking of the entire approach."

**Fix.** Drop the contrast and say what the thing does: "It parses YAML and validates
schemas." If the contrast genuinely matters, check that the reader actually holds the belief
you're correcting.

### 2.4 Opening clichés

Dead giveaways. Replace with your actual first point.

- "In today's digital landscape…"
- "In the ever-evolving world of…"
- "In an era where…"
- "When it comes to…"
- "At its core…"
- "Let's dive in."

### 2.5 Hollow conclusions

Vague, upbeat summaries that add nothing.

- "In summary, X represents a powerful approach to…"
- "By leveraging X, teams can unlock…"
- "Overall, X stands as a testament to…"
- "Despite challenges, the future looks promising."

**Fix.** End on the last substantive point. If the piece needs a conclusion, summarise the
specific takeaways, not the mood.

---

## 3. Sentence and paragraph structure

### 3.1 The tricolon problem

Models love triplets: three adjectives, three bullets, three parallel clauses. One or two in
a document is fine — it's a legitimate device. When every list has exactly three items and
every noun has exactly two adjectives, the rhythm turns robotic.

Symptoms:

- "enthusiasm, experience, and expertise"
- "fast, flexible, and reliable"
- "designed, developed, and deployed"
- Every bulleted list has three items, or five.

**Fix.** Vary list lengths. Use two. Use four. Use a sentence instead of a list. If you
genuinely have three things to say, say them — then break the pattern somewhere else.

### 3.2 The claim-support-restate formula

Generated paragraphs often follow a rigid shape: topic sentence stating a claim, supporting
sentence with a generic example, closing sentence restating the claim in different words.
Every paragraph the same, producing a numbing rhythm.

**Fix.** Vary paragraph length. Some should be one sentence, some five. Lead with an example
sometimes. Occasionally let the evidence stand without a summary at the end.

### 3.3 Robotic transitions

| Overused | Simpler |
|------------------------|-------------------------------|
| Furthermore            | Also, and                     |
| Moreover               | Also, and, plus               |
| Additionally           | Also                          |
| Subsequently           | Then, later, after that       |
| Consequently           | So                            |
| Nevertheless           | But, still, even so           |
| It is worth noting     | Cut entirely                  |
| Notably                | Cut, or fold into the sentence |

Not every sentence needs a signpost. If the logic flows, the reader doesn't need a word
telling them it does.

### 3.4 Thesaurus syndrome

Models avoid repeating words, sometimes to absurd effect. A server becomes "the machine",
then "the instance", then "the compute resource". This is confusing, not elegant.

**Fix.** In technical writing, consistency is a virtue. Call a server a server every time.
The reader is here for clarity, not literary variety.

### 3.5 Copula avoidance

Generated text systematically avoids *is* and *are*, replacing them with inflated
alternatives.

| Generated | Human |
|-------------------------------------|--------------------------------|
| "serves as the primary entry point" | "is the primary entry point"   |
| "stands as a reminder"              | "is a reminder", or just cut   |
| "boasts a wide range of features"   | "has many features"            |
| "features four separate spaces"     | "has four spaces"              |
| "offers a diverse array of options" | "has several options"          |

*Is* and *has* are fine words. Use them.

---

## 4. Punctuation

### 4.1 Em-dash overload

Models use the em dash as a universal connector — joining clauses, inserting asides,
replacing commas, colons, and full stops — until it becomes a visual fingerprint.

- One em-dash pair per paragraph at most. Convert the rest to commas, parentheses, colons,
  or full stops.
- Never use em dashes in headings or titles.
- Prefer a full stop and a new sentence over an em-dash-connected thought. Shorter sentences
  are almost always clearer.

### 4.2 Colon-heavy headings

Generated headings often take a two-part shape: "Deployment: A Practical Guide", "Error
Handling: Best Practices and Patterns". One or two is fine. When every heading does it, the
document reads like a slide deck.

### 4.3 Excessive bolding

Models bold key terms as though writing study notes. In technical prose, bold should be
rare: introducing a term for the first time, or naming an interface element. Don't bold every
occurrence of a concept, and don't bold phrases for emphasis in running text.

### 4.4 Curly quotes

Some models emit typographic quotation marks and apostrophes instead of straight ones. Where
the project uses straight quotes in code and prose, search and replace the curly variants.

---

## 5. Content and substance

### 5.1 Significance inflation

Generated text constantly tells you how important something is instead of showing you.

- "X plays a crucial / vital / pivotal role in…"
- "X marks a significant shift in…"
- "X underscores the importance of…"
- "X is a testament to…"
- "This highlights the enduring legacy of…"
- "Contributing to the broader…"

**Fix.** Delete the significance claim. If the importance isn't obvious from the facts, add
concrete evidence — a number, a comparison, a consequence — rather than an adjective.

### 5.2 Vague claims

Generated writing is often low signal: many words conveying little.

| Vague | Specific |
|--------------------------------------|-----------------------------------------------------|
| "a comprehensive solution"           | "it automates monthly payroll billing"              |
| "significantly improves performance" | "reduces p99 latency from 200ms to 45ms"            |
| "a wide range of use cases"          | "batch processing, streaming, and ad-hoc queries"   |
| "designed with scalability in mind"  | "tested to 10,000 concurrent connections"           |
| "leverages cutting-edge technology"  | "uses gRPC for transport and Raft for consensus"    |

If you can't replace a vague claim with a specific one, the claim probably shouldn't be in
the document.

### 5.3 Superficial analysis

Models append shallow commentary to facts, usually as a present-participle phrase.

- "The library was released in 2019, **marking a significant milestone** in the project's
  evolution."
- "The API supports pagination, **ensuring that clients can efficiently retrieve** large
  datasets."
- "It was written in Go, **reflecting the team's commitment to** performance."

**Fix.** Delete the participle phrase. The fact stands on its own. If the analysis genuinely
matters, give it its own sentence with evidence behind it.

### 5.4 Promotional language

Generated text drifts towards advertising copy, even about mundane components.

**Signals:** boasts, showcases, enhances, exemplifies, commitment to, nestled, in the heart
of, renowned, featuring, diverse array.

**Fix.** Neutral, descriptive language. "The library provides three serialisation formats",
not "The library boasts a diverse array of powerful serialisation options."

### 5.5 "Challenges and future outlook" boilerplate

Models love ending with a section about challenges faced and prospects ahead. The formula:
"Despite [positive words], X faces challenges including [generic list]. Despite these
challenges, [optimistic speculation]."

If there are genuine challenges worth documenting, describe them concretely. Otherwise, cut
the section.

---

## 6. Structural tells

### 6.1 Inline-header lists

Generated bulleted lists take a bold header, a colon, and a description on the same line:

- **Parsing:** The system parses incoming YAML files and validates their structure.
- **Routing:** Requests are routed to the appropriate handler based on the URL path.
- **Logging:** All events are logged to stdout in JSON format.

Occasionally useful, but models reach for it constantly. When each description is a single
sentence, prose is usually better: "The system parses incoming YAML, routes requests by URL
path, and logs events as JSON to stdout."

### 6.2 Unnecessary tables

Models create small tables that would read better as a sentence. If a table has two columns
and fewer than four rows, consider prose.

### 6.3 Title case in headings

Models default to Title Case for All Headings. Use sentence case — first word and proper
nouns only.

### 6.4 Emoji

No emoji in headings, bullets, or running text. They're fine in chat and unsuitable in
technical documentation.

---

## 7. How to edit

### Read the whole thing first

Don't start fixing word by word. Read the draft and ask:

- Does this say anything substantive, or is it waving its arms?
- Could I swap the subject for a completely different project and have the text still make
  sense? If so, it's too generic.
- What's the one thing the reader should take away? Is that thing actually stated?

### Delete before rewriting

Cut every sentence that fails this test: does it contain information the reader didn't
already have? Significance claims, restated conclusions, and vague analyses almost always
fail.

### Look for clusters

The tells rarely appear alone. Find one — an em dash, a "delve", a tricolon — and search for
others. Three or more distinct tells in one passage means it was generated wholesale and
needs rewriting, not patching.

### Read it aloud

Generated prose has a distinctive cadence: smooth, even, relentlessly upbeat. Human prose has
texture — short sentences beside long ones, blunt statements beside nuanced ones, occasional
roughness. If it sounds like a keynote, it needs more variation.

### Add your actual opinion

Models are trained towards neutrality. Technical writing benefits from a point of view. "We
chose X over Y because Z" is more useful than "Both X and Y offer compelling advantages for
modern development workflows." If you have a recommendation, state it. If you have a caveat,
state it. The reader is here for your judgement, not a diplomatic survey of all positions.

---

## 8. Checklist

Use this when reviewing a draft. More than three boxes ticked means rewrite the passage
rather than edit it.

- [ ] Contains words from the removal list (§1.1)
- [ ] Uses inflated adjectives with no specifics (§1.2)
- [ ] Leans on abstract metaphors (§1.3)
- [ ] Hedges instead of claiming (§1.4)
- [ ] Contains faux-insider phrasing (§2.1)
- [ ] Uses staccato rhetoric or dramatic fragments (§2.2)
- [ ] Uses "not just X, but Y" constructions (§2.3)
- [ ] Opens with a cliché (§2.4)
- [ ] Ends with a hollow conclusion (§2.5)
- [ ] Every list has exactly three items (§3.1)
- [ ] Every paragraph follows claim-support-restate (§3.2)
- [ ] Formal transition words start every other sentence (§3.3)
- [ ] Avoids "is" and "are" (§3.5)
- [ ] More than one em-dash pair per paragraph (§4.1)
- [ ] Bold used for emphasis in running prose (§4.3)
- [ ] Claims significance without evidence (§5.1)
- [ ] Makes vague claims that could apply to anything (§5.2)
- [ ] Appends participle phrases with shallow analysis (§5.3)
- [ ] Uses promotional adjectives (§5.4)
- [ ] Ends with a "despite challenges" section (§5.5)
- [ ] Formats every list as bold-header-colon (§6.1)

---

## 9. What not to "fix"

Not everything that looks generated is a problem, and over-correcting causes its own damage.

**Correct grammar and spelling.** Good grammar isn't a tell. Don't introduce errors for
authenticity.

**Formal register.** Technical writing is often formal, and that's fine. The problem is
*formulaic* writing, not formal writing.

**Transition words in moderation.** *However* and *also* are useful. The problem is using
them mechanically at the start of every other sentence.

**The word "is".** Models avoid it; you shouldn't. But don't insert it where a more precise
verb genuinely works better.

**Lists.** Bulleted lists are a legitimate tool. The problems are reaching for one where
prose would work, and formatting every one as bold-header-colon.

---

## 10. The principles behind all of it

1. **Specifics beat adjectives.** A number, a name, or a concrete example is worth more than
   any superlative.
2. **Short words beat long words.** *Use* beats *utilise*. *Help* beats *facilitate*. *Show*
   beats *showcase*.
3. **Varied rhythm beats even rhythm.** Mix sentence lengths. Mix paragraph lengths. Break
   patterns.
4. **Stating a fact beats announcing its importance.** Let the reader judge significance.
5. **One good sentence beats three that say the same thing.** Delete the restatements.
6. **An honest opinion beats diplomatic neutrality.** Readers want your recommendation, not a
   survey of every possible position.
7. **Silence beats noise.** If a sentence adds no information, remove it. A shorter document
   that says something beats a longer one that doesn't.
