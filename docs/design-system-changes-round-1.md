# Design system — what changed when we wired it up

**For:** the designer, ahead of the next iconography and colour-palette round
**From:** implementation, after building the system into Cromwell's Go templates
**Date:** 2026-07-27
**Baseline:** your delivered `cromwell.css` (1,718 lines, pattern-library
scaffolding removed) and `icons.svg`

---

## Why you're getting this

The system is now live in the real app across nine page types, driven by real
data rather than fixtures. That shook out a handful of changes — five in the CSS,
and a few in how the markup uses it.

We're sending them before you deliver the new palette and icon set for one
practical reason: **if the next hand-off is built on your original stylesheet, it
will quietly revert these fixes.** Folding them into your source first means the
next delivery drops straight in.

**The icon sprite is untouched** — all 71 symbols are byte-identical to what you
sent. Every change below is CSS, or how the templates use your classes.

---

## 1. Icons are now sized in `em`, not pixels

**The one change we'd most like you to adopt.** It's the biggest departure and it
fixes a whole class of problem rather than one instance.

Your sizes were fixed pixels:

```css
.icon    { width: 16px; height: 16px; }
.icon-xs { width: 13px; … }  .icon-sm { width: 14px; … }
.icon-md { width: 18px; … }  .icon-lg { width: 22px; … }  .icon-xl { width: 28px; … }
```

The trouble showed up wherever an icon sat beside large type. The hero token
readout is 30px, and its mark stayed at 14px — visibly a footnote next to the
number it belongs to. The same icon class also read inconsistently between two
panel headings of different sizes.

Now:

```css
.icon    { width: 1.15em; height: 1.15em; }
.icon-xs { width: 1.1em;  … }  .icon-sm { width: 1.05em; … }
.icon-md { width: 1.1em;  … }  .icon-lg { width: 1.3em;  … }
.icon-xl { width: 1.6em;  … }
```

The ratios were chosen to **reproduce your pixel sizes in each icon's usual
context**, so nothing shifted where it already looked right — a `.icon-sm` beside
13px text is still ~14px. But an icon beside bigger text now grows with it.
Measured across the app, icons now sit at a consistent 1.07–1.13× their context
font size; before, the same classes ranged from 0.9× to 1.4× depending on where
they landed.

`stroke-width` per size class is unchanged — your thinning at `-lg` and `-xl`
still applies.

**Please sanity-check this against the new icon set.** Em sizing means the
optical weight of a symbol now matters across a wider size range than you
designed for, and a mark that reads well at 14px may need a different stroke at
30px. That's a judgement call we'd rather you made.

## 2. The confidence meter is em-based too

Same reasoning. It was `height: 12px`, bars `3px` wide at `5 / 8.5 / 12px`. It is
now `0.86em` tall, bars `0.21em` at `0.36 / 0.61 / 0.86em` — which reproduces
your exact pixel sizes at the default 14px readout, and scales up beside a hero
number.

We also let it grow in the hero readout specifically (`0.62em` of a 30px
container, so ~16px rather than 12px), because the client reads the meter as part
of the number, not as a footnote to it.

## 3. The hero token readout scales as one unit

Previously the type size sat on `.tokens-num`, so the number scaled but its
siblings didn't. The container now carries it:

```css
.tokens--hero { font-size: var(--fs-3xl); }
.tokens--hero .tokens-num  { font-size: 1em; }
.tokens--hero .icon        { width: .85em; height: .85em; }
.tokens--hero .tier        { font-size: 0.62em; }
.tokens--hero .tier-label  { font-size: 0.42em; }
```

Everything in the readout now resizes together, and a `--lg` or a future `--xl`
variant only needs one font-size.

## 4. Two robustness fixes for real content

Both are cases where content longer than the pattern-library fixtures broke a
component. Added at the end of the file under `/* Project additions */`.

**A long modal title shoved the close button out of the panel.** `.modal-head` is
a flex row, but `.modal-title` had no flex basis, so it shrank to min-content and
its inline content broke between the icon and the first word. Fixed with
`flex: 1 1 auto; min-width: 0` on the title, plus `flex: none` on the close
button.

Worth knowing: **`.modal-close` is defined in your CSS but appears in none of the
example markup** — the demo close buttons are `btn btn--ghost btn--sm` only. We
missed the class for the same reason, and the button drifted left. Adding it to
the examples would prevent the next person repeating it.

**An icon opening a small `.t-meta` label broke onto its own line** when the row
got tight ("⚲ / Belongs to Planning surface" stacked). Fixed with a rule keyed on
the pattern rather than a hand-applied class:

```css
.t-meta:has(> svg.icon:first-child) { display: inline-flex; align-items: center; gap: var(--sp-2); }
```

One caution learned the hard way: we first applied that to `.rail-card-head` and
`.rail-field-label` as well, and it broke them. `.rail-card-head` already lays out
its own icon row **and carries the card's full-width `border-bottom`** —
`inline-flex` shrank it to fit its text, so the divider under "ACTIONS" only
spanned the word. If any new component pairs an icon with a bottom border, it
wants `flex`, never `inline-flex`.

## 5. Markup contracts worth documenting in the pattern library

Not CSS changes — places where a component needs a specific DOM shape that the
CSS depends on, and where we got it wrong from reading the class names alone.
Each one silently mangled the layout rather than failing loudly.

- **`.roadmap-item` is a two-column grid** (`44px minmax(0,1fr)`), and
  `.roadmap-num` must be nested **inside** `.roadmap-spine`. We emitted the
  number, the spine and the card as three siblings; the card landed in the 44px
  column and wrapped one word per line. A note on the pattern would have saved
  us — this was the worst-looking bug of the batch.
- **`.checkpoint-question` has no padding of its own.** It belongs inside
  `.checkpoint-head`; `.checkpoint-context` and `.checkpoint-answers` are the
  other two padded regions. We placed the question as a direct child of
  `.checkpoint` and it sat flush against the card edge.
- **`.section` should not nest.** We briefly wrapped a `<details class="section">`
  in a `<section class="section">`, which double-applied the styling.

## 6. Two content decisions that affect the visuals

Product decisions, but they change what your components display, so they matter
for the palette round.

- **Token counts are rounded to the nearest 1k** and always shown in `k`
  (`155,800` → `156k`; `1,240,000` → `1,240k`). Sub-500 values show `<1k` rather
  than `0k`. So `.tokens-num` now holds shorter strings than your fixtures did.
- **The word "tokens" is gone from the visible UI.** The hexagonal mark carries
  the unit, which is exactly what your treatment of it made possible. `.tokens-unit`
  is now used only for real words like "Not estimated yet"; the unit itself
  survives as a `visually-hidden` span so screen readers still announce
  "156k tokens".

  This raises the stakes on the token mark in the new set: **it is now the only
  thing identifying a number as tokens.** Please treat it as load-bearing rather
  than decorative.

---

## What we'd like from the next round

1. **Fold sections 1–4 into your source**, so the new palette and icon set arrive
   on top of them rather than reverting them.
2. **Re-check the icon set at size**, given em sizing now stretches marks from
   ~11px to ~26px in live use — particularly stroke weight at the large end.
3. **The token mark is load-bearing** (see §6) — it is the sole unit indicator.
4. **Add the markup contracts from §5 to the pattern library**, as a note on each
   affected component. They are the changes most likely to be got wrong again.
5. For the palette: the states carrying the most meaning in practice are the
   confidence tiers (rough / considered / decomposed, currently amber / blue /
   green) and the lifecycle badges. Those get read at a glance far more than
   anything else on the page.

Everything else in the system went in unchanged and is working well — the
disclosure pattern, the row and panel anatomy, the checklist, the numbered
roadmap spine, the answer cards with their consequence lines, the banner and
empty-state treatments, and both themes.
