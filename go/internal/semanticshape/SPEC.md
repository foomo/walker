# Semantic shapes – specification

Status: **implemented** in package `semanticshape` (formerly `htmlschema` v2), which replaces v1 (see
[Migration](#13-migration-from-v1)). The decisions from the feedback rounds are in the
[decision log](#15-decision-log). The worked examples in [section 14](#14-worked-examples)
are the golden tests in `examples_test.go`.

Semantic shapes make an SEO specification machine readable and executable. For every
*group* of pages (typically one Next.js route such as `/products/[slug]`) an SEO
expert writes a shape: an HTML file named `*.shape.html` that describes the
semantic structure a page of the group must have. This document still calls a
shape file a *schema* and its directory the *schema root*. Validating a page against it yields three independent
results:

- **findings**: rules the page breaks. Each finding has a **severity** (`error`,
  `warn`, `info`), which is the gate that later decides whether CI fails, and a
  **penalty**, which says how much the mistake weighs.
- **score**: points for present, well-formed content, with a known maximum. It
  measures how much good content a page, a component or a group has, as a percentage.
- **penalty**: the sum of the findings' penalties.

Score and penalty are **never combined**. "Lots of good content, but one heavy
mistake" (score 90 %, penalty 50) and "little content, no mistakes" (score 20 %,
penalty 0) are different situations and stay distinguishable.

Examples referenced throughout come from the demo shop: the schemas live in
[`demoshop/schema`](../demoshop/schema), the pages are rendered by
[`demoshop`](../demoshop) (`go run ./cmd/demo`). They are worked through in
[section 14](#14-worked-examples).

## 1. Concepts

| term | meaning |
|---|---|
| group | name of a page type, e.g. `content/landing`. Passed in by the caller. |
| schema | an HTML file describing the semantic outline a page of a group must have |
| component | a reusable schema for a block that can appear on many pages (teaser, gallery, …), recognized in the page by a marker attribute or a CSS selector |
| wildcard | a schema scope in which layout wrapper elements (`div`, `span`, …) between the described elements are skipped |
| finding | a broken rule: severity, penalty, rule, path, message |
| score | points earned by matched content, summed up to the page. The schema defines the maximum. |
| penalty | points lost through findings, summed up to the page. No maximum. |

**Input.** The library validates whatever HTML it is given. The intended input
is the server response, because SEO-relevant content must be server-rendered.
Rendered DOM snapshots (e.g. from Playwright in CI) work the same way.

## 2. Files and resolution

```
<root>/
  default.shape.html            fallback for groups without a schema
  groups/<group>.shape.html     e.g. groups/content/landing.shape.html for group "content/landing"
  …                             everything else is organized freely, e.g. components/, partials/
```

- Only files named `*.shape.html` belong to the root; every other file, a plain `.html` included, is ignored. A root that has plain `.html` files but no shape files at all is a lint error that names the rename (shape files before `*.shape.html`).
- A group resolves to `groups/<group>.shape.html`. Then `default.shape.html`. Neither existing is an error. These two are the only fixed locations.
- `default.shape.html` is only a fallback. It is **not** combined with group schemas.
- **Paths** in `val:include` and `val:comp` are written without the `.shape.html` suffix. A leading `/` starts at the schema root (`/partials/head`). Anything else is relative to the file that contains it (`../../partials/head`).
- Any file can be [included](#22-valinclude). A file with a single top level element can be [instantiated](#23-valcomp).
- At load time all files under the root are read. Every file whose root is `val:component` is a **recognized component**, identified by its root-relative path (`/components/teaser`), however it is referenced. Every recognized component can be found on every page, not only on pages whose schema instantiates it (needed for [closed contexts](#63-closed-and-open-contexts) and [ambiguity](#32-component-recognition)).

Library API sketch:

```go
root, err := semanticshape.LoadRoot("schema")
report, err := root.Validate("content/landing", htmlBytes)
```

### 2.1 Settings

Settings belong to each schema. They are written as a `<val:settings>` element,
a direct child of `<html>`. That makes them shareable through `val:include`.

```html
<val:settings exclude=".mobile-only" marker-attribute="data-component" penalty-error=10 penalty-warn=3 penalty-info=0/>
```

| setting | default | meaning |
|---|---|---|
| `exclude` | (none) | CSS selectors of elements to remove before matching, in addition to the [built-in exclusions](#31-exclusion) |
| `marker-attribute` | `data-component` | attribute that marks component instances |
| `penalty-error`, `penalty-warn`, `penalty-info` | 10, 3, 0 | penalty of a finding without explicit `val:penalty` |

When a page is validated, the settings of the group schema apply to everything,
including component recognition and the component schemas used on the page.

### 2.2 `val:include`

`<val:include path="/partials/head"/>` inserts the content of a file in place, as
if it were written there. The file may contain any number of top level elements
([partials/head.shape.html](../demoshop/schema/partials/head.shape.html) holds the `meta`s and the canonical `link` of every page).
Everything about the include site applies to the inserted elements, including the
wildcard scope. Includes may nest. Cycles are a lint error.

### 2.3 `val:comp`

`<val:comp path="/components/teaser" val:max=3/>` instantiates a file whose
content is **a single top level element**: that element is placed here, and the
attributes of `val:comp` (except `path`) **override or extend** the attributes of
the root element.

- Attributes with the same name replace the root's value, and new ones are added.
- Occurrence attributes (`val:min`, `val:max`, `val:count`, `val:optional`, `val:forbidden`) are replaced as a group. If the site sets any of them, none of the root's apply.
- `marker` and `selector` of a `val:component` root can't be overridden (lint error), because recognition runs page-wide before matching and must be the same everywhere.

The root decides what the instance does:

- a **`val:component`** root: a recognized component, see [section 5](#5-components)
- any **other element**: an element template, matched like the element written in place. [partials/site-header.shape.html](../demoshop/schema/partials/site-header.shape.html), a `<header>` with the logo link and the main navigation, is used as `<val:comp path="/partials/site-header"/>` in every group. A site could relax it for one group with `<val:comp path="../partials/site-header" val:severity="warn"/>`.

## 3. Matching

Validating a page runs in three steps: exclusion, component recognition, then
matching the schema tree against the page tree.

### 3.1 Exclusion

These elements, and their subtrees, are removed from the page before anything else:

- `script`, `style`, `template`, `noscript` (JSON-LD is read before this, see [section 7](#7-structured-data-valld))
- elements with the `hidden` attribute
- elements matching `exclude` from the settings. This is for responsive duplicates that are only hidden by CSS, e.g. `exclude=".mobile-only"`. `[aria-hidden=true]` is not excluded by default, because search engines index it. Add it if a project uses it for duplicates.

The validator never sees CSS, so content that is only hidden by a media query
must be excluded by selector. Otherwise it counts twice. A header that renders
its navigation twice, once for desktop and once as `<nav class="mobile-only">`,
contains two `nav`s, and `header > nav` (exactly one) fails without
`exclude=".mobile-only"`.

### 3.2 Component recognition

Every element of the (excluded) page is checked against all components:

1. An element whose marker attribute is set is an instance of the component with that `marker` value. If no component has that marker, the marker is ignored, as if it was not there: the element is matched like any other element, and its content is visible to the schema.
2. An element without a marker attribute is an instance of the component whose `selector` matches it. If **several** selectors match, the element gets a `component-ambiguous` finding (error) naming all of them. It is treated as an opaque unknown instance: not validated, earns nothing. This can only be detected on concrete pages, not when loading schemas. Example: `[class*='Teaser_root']` also matches `VideoTeaser_root__v7k`. The author fixes it with plain CSS (`[class*='Teaser_root']:not([class*='VideoTeaser_root'])`), or the frontend adds a marker.
3. A selector match inside an instance of the same component is part of that instance, not a nested one. Marker and CSS module class often sit on different wrappers: `<div data-component="text"><div class="RichText_root__r1">…` is one `text` instance.
4. Otherwise, recognition continues inside an instance. Nested instances belong to the content of the outer instance.

From the outside, an instance is **opaque**: one node named after the component
(`teaser`, `gallery`, …), whatever its root element is. Its content is only
visible to the component's own schema (and to [`val:selector`](#34-valselector)).

### 3.3 Elements and wildcards

A schema element matches the **children of its context node** (the page node
its parent schema element matched; the document for `<html>`) that have

- the same tag name, and
- every *literal* attribute of the schema element: `rel="canonical"` must be equal, `hreflang="*"` must be present and non-empty. Attributes starting with `val:` are not literals.

Page elements not mentioned in the schema are ignored. The schema describes what
must or may be there, not everything that is there.

Without a wildcard, matching is strict: only direct children. React layouts nest
semantic elements deep in wrapper `div`s, so schemas declare where wrappers are allowed:

```html
<body>
    <val:wildcard for="div, span">
        <header>…</header>
        <main>
            <h1/>
        </main>
    </val:wildcard>
</body>
```

- Inside `<val:wildcard for="…">`, a schema element matches elements reachable from the context node through **any number (0..n) of elements matching `for`**. `for` is a CSS selector list, matched against each element on its own: `for="div, span"`, `for="div, section:not([aria-label])"`. `for="*"` skips any element.
- The scope is **lexical and inherited**. In the example, `main` is found through the wrappers below `body`, and `h1` is found through the wrappers below `main`. A nested `val:wildcard` replaces the set for its content. `for=""` makes a part strict again.
- Component instances are never skipped and never searched through, even when their root is a `div`.
- An element matching `for` can still be matched itself, e.g. `<div class="x">` inside `for="div"`. The search continues below it either way.
- `val:wildcard` doesn't match a page node itself, and it doesn't appear in paths.

What the `article` of a [product page](../demoshop/templates/product.html) looks like through `<val:wildcard for="div">`:

```
article
├── figure                                 (in div.product-top)
├── h1                Clanky the Butlerbot (in div.product-top > div.product-info)
├── p.price           1299.00 €
├── p.description     A tireless butler …
├── feature-section   (data-component="feature-section", root is a <section>)
└── feature-section
```

### 3.4 `val:selector`

`<val:selector for="css">` matches all elements the CSS selector finds in
the subtree of the context node, **ignoring wildcards and component borders**.
At the top of `<body>` it searches the whole page, including inside components.
Typical use is page-wide rules, shared through a partial ([partials/page-baseline.shape.html](../demoshop/schema/partials/page-baseline.shape.html)): `<val:selector for="h1" count=1/>`.

The rules of the `val:selector` element itself apply to every selected node. Its
children match the children of each selected node (wildcards apply as usual).

## 4. Element rules

**Spelling.** On HTML elements, rule attributes carry the `val:` prefix to
separate them from literal attributes (`<meta property="og:title" val:score=2/>`).
`val:*` elements have no literal attributes, so they take the same rules
**without** prefix (`<val:selector for="h1" count=1 score=10/>`). The exception is
`val:comp`: it passes attributes through to its root, which may be an HTML
element, so it keeps the prefix (`<val:comp path="/partials/main-nav" aria-label="Main" val:score=20/>`).
At a `val:comp` site, `val:max=3` overrides `max` on a `val:*` root. This spec
names rules with prefix: `val:score` means `score` on a `val:*` element.

### 4.1 Occurrence

| syntax | min | max |
|---|---|---|
| (nothing) | 1 | 1 |
| `val:optional` | 0 | 1 |
| `val:min=n` | n | unbounded |
| `val:max=n` | 1 | n |
| `val:min=n val:max=m` | n | m |
| `val:count=n` | n | n |
| `val:forbidden` | 0 | 0 |
| `val:max=*` | | unbounded |

`val:optional` combines with `val:max`. Defaults differ for `val:selector`
(1..unbounded) and for `val:comp` inside `val:choice` (0..unbounded).

Finding rule: `occurrence`.

### 4.2 Text content

The text of a matched node is the text of all its descendants (after
exclusion), with whitespace runs collapsed to one space and trimmed. Length is
counted in characters (Unicode code points, so `–` and `ü` count as 1). Words are
whitespace separated tokens. All bounds are inclusive.

| syntax | finding rule |
|---|---|
| `val:min-length=n`, `val:max-length=n` | `min-length`, `max-length` |
| `val:min-words=n`, `val:max-words=n` | `min-words`, `max-words` |

### 4.3 Attribute rules

`val:attr-<name>="rule; rule; …"` validates the value of attribute `<name>` on
every matched node. Unlike literal attributes, a failing attribute rule does not
stop the node from matching. It produces a finding. An absent attribute has
the value `""`.

| rule | passes when |
|---|---|
| `required` | the attribute is present and not empty |
| `empty` | the attribute is absent or empty |
| `min-length:n`, `max-length:n`, `length:n` | value length in characters, inclusive |
| `min-words:n`, `max-words:n` | word count, inclusive |
| `one-of:a\|b\|c` | value equals one of the alternatives |
| `url` / `url:absolute` | value parses as a URL / as an absolute `http(s)` URL |
| `regex:<re>` | Go regexp matches. Must be the **last** rule: everything after `regex:` is the expression, unescaped, including `;` and `:` |
| `severity:<s>`, `penalty:<n>` | not checks: override severity / penalty for this attribute's findings |

Finding rule: `attr:<rule>`, path suffix `@<name>`, e.g. `html/head/meta[name=description]@content`.

`val:attr="name; rules"` (v1) is still accepted. It allows only one attribute
per element, because an element can't carry `val:attr` twice.

### 4.4 Severity and penalty

- `val:severity=error|warn|info` sets the severity of all findings produced by this element's own rules (occurrence, text, attributes). The default is `error`.
- `val:penalty=n` sets their penalty. The default comes from the [settings](#21-settings), based on the element's severity: error 10, warn 3, info 0.
- Neither is inherited by child elements.

Occurrence penalties scale with the deviation:

| deviation | findings |
|---|---|
| too many nodes | one finding **per surplus node**, each with that node's locator and the full penalty. Surplus = nodes after the first `max` in document order. |
| too few nodes | one finding at the context node, penalty × number of missing nodes |

`val:penalty-max=n` caps the total penalty of an element's occurrence findings
(surplus findings beyond the cap are still reported, with penalty 0).

| rule | page | findings |
|---|---|---|
| `<a val:min=3 val:max=12>` | 2 links | 1 finding, 1 × 10 |
| same | 0 links | 1 finding, 3 × 10 = 30 |
| `<h1 val:penalty=50>` | 4 h1s | 3 findings × 50 = 150 |
| `<i val:forbidden val:penalty=20 val:penalty-max=100>` | 30 `<i>` | 30 findings, penalty 100 in total |

## 5. Components

A component is a file whose single root is `val:component`:

```html
<!-- /components/teaser.shape.html -->
<val:component marker="teaser" selector="[class*='Teaser_root']">
    <val:wildcard for="div, span">
        <img val:optional val:score=2 val:penalty=5 val:attr-alt="min-length:5"/>
        <h3 val:score=3 val:min-length=10 val:max-length=80/>
        <p val:optional val:score=3 val:min-words=8/>
        <a val:score=2 val:attr-href="required"/>
    </val:wildcard>
</val:component>
```

- Its identity is its root-relative path, `/components/teaser`. Reports and `component-not-allowed` use it. Finding paths use the file name (`teaser[2]`), or the full path if two recognized components share a file name.
- `marker`: the value of the marker attribute (`data-component="teaser"`). This is the reliable way and needs the frontend team's cooperation. Markers must be unique among all recognized components (lint).
- `selector`: fallback for markup without a marker, e.g. CSS module classes such as `Teaser_root__t9q`. This is brittle across refactorings, so it's only a fallback.
- Children are matched against the children of the instance root. The root itself can be any element. Wildcards from the instantiating schema do **not** reach into a component, so a component declares its own.
- A component can instantiate other components (`val:comp`, `val:choice`, `val:sequence`), and it can contain `val:ld`, e.g. an FAQ component requiring an `FAQPage` JSON-LD.
- Including a component file with `val:include` inserts the `val:component` element itself, so it still finds instances. To reuse only its rules, put them in a separate file and include that.

## 6. Content models

### 6.1 Instantiating components

`<val:comp path="/components/gallery"/>` with a `val:component` root matches the
instances of that component among the children of the context node (wildcards
apply), and validates each against the component schema. Occurrence attributes
work as on elements. Outside of `val:choice` the default is exactly 1.

`val:severity` at the site is a **ceiling** for all findings inside the
instances. `val:severity=warn` turns their errors into warnings, e.g. to
tolerate legacy blocks without failing CI. Penalties are not affected. A default
penalty is derived from the severity *before* the ceiling applies. (On an element
template, `val:severity` only applies to the root element's own rules, as on any element.)

### 6.2 `val:choice` and `val:sequence`

For freely composed CMS areas:

```html
<val:choice min=2 max=12 score-max=100>
    <val:comp path="/components/text" val:max=4/>
    <val:comp path="/components/teaser" val:max=6/>
    <val:comp path="/components/gallery" val:max=1/>
    <val:comp path="/components/faq" val:max=1/>
</val:choice>
```

- `val:choice`: any number of instances of the listed components, **in any order**, mixed. `val:min/max` on the choice limit the total number of instances. `val:min/max` on each `val:comp` limit that type (default 0..unbounded).
- `val:sequence`: the listed components must appear **in this order**. Each `val:comp` defaults to exactly 1. An instance out of order produces a `component-order` finding.

A context may hold several choices and sequences. They contain only `val:comp`s of recognized components. A component may appear in only one of them per context (lint).

### 6.3 Closed and open contexts

A schema element containing any `val:comp` of a recognized component, `val:choice` or `val:sequence` is a
*component context*. By default it is **closed**:

| instance found in the context | finding |
|---|---|
| a component it uses | validated |
| a known component it does not use | `component-not-allowed`, error |

Markers without a component schema are not components (see [3.2](#32-component-recognition)), they produce no finding.

`val:open` on the context element turns `component-not-allowed` into info.
Instances outside component contexts are opaque and ignored. Only
`val:selector` can still look inside them.

## 7. Structured data (`val:ld`)

```html
<val:ld type="Product" score=20>
    <val:prop path="name" min-length=5/>
    <val:prop path="offers.priceCurrency" value="one-of:EUR|CHF"/>
    <val:prop path="offers.availability" optional value="regex:^https://schema\.org/(InStock|OutOfStock)$"/>
</val:ld>
```

- Matches JSON-LD objects with `@type` equal to (or containing) `type`, from all `script[type="application/ld+json"]` of the page, including `@graph` arrays. The position of `val:ld` in the schema doesn't matter (page-wide), except inside a component, where only scripts inside the instance count.
- Occurrence as for elements (default exactly 1). Finding rule: `ld-occurrence`.
- `val:prop path="a.b"`: dot path into the object. If a step hits an array, any element may satisfy it. With `every`, every element has to pass - and has to have the property, unless it is `optional` - and each failing element is a finding of its own, indexed in document order: `itemListElement.item.offers.priceCurrency[2]`. Use it for lists, where one good entry must not hide a bad one:

  ```html
  <val:ld type="ItemList">
      <val:prop path="itemListElement.item.offers.priceCurrency" every value="one-of:EUR|USD|CHF"/>
  </val:ld>
  ``` Default required. `val:min-length` etc. apply to string values. `value` takes the [attribute rule list](#43-attribute-rules). Finding rule: `ld-prop:<rule>`.
- Scoring as for elements: the object earns `val:score` if all its required props pass. Props may carry their own `val:score`.

## 8. Findings

```
error  10  attr:min-length   html/body/main/teaser[2]/img@alt   got 0, expected at least 5
```

- Locator: a CSS path to the page node, usable with `document.querySelector` in the browser's dev tools, e.g. `html > body > div > div > main > div:nth-of-type(2) > section`. `:nth-of-type(n)` is added where a node has siblings with the same tag. It is computed on the page as delivered, before exclusion. For missing nodes, the locator is the context node they are missing from.
- Path: schema path with page positions. Stable across layout changes, so findings can be compared over time. The locator is for finding the node. Components appear by name. `[n]` counts matched nodes of that schema element within one context node, starting at 1, and is omitted when there is only one. Literal attributes in head paths identify the element: `meta[name=description]`.
- Order: schema document order, then page order. Deterministic.
- A finding does not embed the schema element (v1 did, including its whole subtree).

## 9. Score and penalty

### 9.1 Earning points

- `val:score=n`: every matched node earns `n` if **its own rules pass** (text and attribute rules of this element). Children earn independently. A failing child rule does not cost the parent anything.
- Only the first `max` nodes (document order) earn. Surplus nodes produce an `occurrence` finding and nothing else.
- `val:score-per="words:k"` (or `chars:k`): the node earns `n × floor(units / k)` instead of `n`. This rewards amount: `val:score=2 val:score-per="words:20"` = 2 points per full 20 words.
- `val:score-max=m`: caps everything the element earns **including its subtree, summed over all its nodes**.
- `val:comp` of a component earns what its instances earn under the component schema. `val:choice` and `val:sequence` earn the sum of their `val:comp`s, capped by their own `val:score-max`.
- Component instances that are not allowed, or unknown, earn nothing.
- There are no negative points. Mistakes are measured by the penalty.

### 9.2 Maximum

The maximum is computed from the schema alone, so it is the same for every page of a group:

```
max(E)          = min( score-max(E),  maxCount(E) × (score(E) + Σ max(child)) )
max(val:comp)   = min( score-max,  maxCount × max(component) )
max(val:choice) = min( score-max,  best combination )
max(val:sequence) = min( score-max,  Σ max(val:comp) )
```

For a choice, the *best combination* is computed greedily: first the `val:min`
instances of every use, then the most valuable remaining instances (highest
`max(component)` first) up to each use's `val:max` and the choice's `val:max`.
So the maximum is always reachable in theory.

An element whose maximum would be unbounded (unbounded `max` or `score-per`
with points somewhere in its subtree) must have `val:score-max`. Otherwise
loading fails.

Optional content counts towards the maximum. A page that leaves out optional
content has less content, and its percentage shows that.

### 9.3 Penalty

The penalty of a page is the sum of its findings' penalties. It has no maximum
and is never subtracted from the score. Occurrence penalties scale with the
deviation, see [4.4](#44-severity-and-penalty).

### 9.4 Aggregation

- page: score `points / max` as a percentage, penalty as a sum
- component instance: `points / max(component)` and the penalty of the findings inside it. Component type on a page: sums over its instances.
- group (across pages, later in walker metrics): average score percentage, average and total penalty, and per component type the sum of points / sum of maxima. Score and penalty are separate metric series.

## 10. Report model

Names are tentative. Generated Swift code shares one module, so type names must
not clash with `vo` and `walker` types.

```go
type Severity string // "error" | "warn" | "info"

type Finding struct {
	Severity  Severity
	Penalty   int
	Rule      string // "occurrence", "min-length", "attr:url", "component-not-allowed", …
	Path      string // "html/body/main/teaser[2]/img@alt"
	Locator   string // "html > body > div > div > main > … > img"
	Message   string // "got 0, expected at least 5"
	Component string // innermost component instance, "/components/teaser", "" on page level
	Source    string // "components/teaser.shape.html:6"
}

type ScoreNode struct {
	Path     string
	Points   int
	Max      int
	Children []*ScoreNode // only scoring nodes, for drill down
}

type ComponentSummary struct {
	Instances int
	Points    int
	Max       int
	Penalty   int
}

type Report struct {
	Group      string
	Schema     string
	Findings   []*Finding
	Score      *ScoreNode
	Penalty    int
	Components map[string]*ComponentSummary
}
```

## 11. Schema lint

Loading fails with a list of all problems, each with file and line:

- unknown `val:` attribute or element, unknown attribute rule, unknown setting
- on HTML elements, attributes that look like a forgotten prefix: `score=…`, `optional`, `val=optional`, `min-length=…` (v1 examples contain both)
- `val:`-prefixed attributes on `val:*` elements other than `val:comp`
- duplicate attributes on an element
- unclosed or mismatched elements (see [section 12](#12-schema-syntax))
- `min > max`, non-numeric values, `val:optional` together with `val:min`
- `val:include` / `val:comp` to missing files, include cycles
- `val:comp` of a file without exactly one top level element; overriding `marker`/`selector`
- duplicate markers among recognized components
- `val:include`, `val:comp`, `val:prop`, `val:settings` with children. `val:settings` anywhere but directly in `<html>`.
- unbounded maximum without `val:score-max`
- a component listed in two choices/sequences of the same context

## 12. Schema syntax

Schemas look like HTML but are **not** parsed with the HTML5 parser. Its error
recovery would restructure them: a `val:*` element in `<head>` ends the head,
`<val:comp/>` swallows its following siblings, `<p><div>` is split. Instead,
schemas are read with the x/net/html *tokenizer* and a simple tree builder:

- The tree is exactly as written. Nothing is moved and nothing is inserted (a schema that describes `<head>` writes it).
- `<x/>` closes any element, including `val:*`. Elements without children are written self-closing: `<h2 val:score=3 val:min-length=3/>`. That's the convention in all examples; `<h2 …></h2>` is equivalent.
- Void elements (`img`, `meta`, `link`, `br`, `hr`, `input`, …) close themselves and can't have children.
- Every other element must be closed explicitly. Unclosed or mismatched end tags are lint errors with line numbers.
- HTML attribute syntax: boolean attributes (`val:optional`), unquoted values (`val:score=10`), entities.
- Attribute names are lowercased (`hrefLang` → `hreflang`), as in the parsed pages.
- Comments and text are ignored.

Pages are still parsed with the HTML5 parser, as browsers do.

## 13. Migration from v1

| v1 | v2 |
|---|---|
| HTML5 parser for schemas | own tree builder, tree exactly as written |
| strict direct-child matching everywhere | still the default; `val:wildcard` allows wrappers explicitly |
| `<root>/<group>/<page>.html`, root = the groups dir | `<root>/groups/<group>.shape.html`, everything else organized freely, unknown group → `default.shape.html`; only `*.shape.html` files count |
| `<ref>path</ref>`, file relative, exactly one top level element | `<val:include path=…/>` (any number of elements) and `<val:comp path=…/>` (one element, attribute overrides); `/` = root, otherwise relative |
| `val:attr="name;…"`, one per element | `val:attr-<name>="…"`; the old form still works for one attribute |
| attribute `min-length`/`max-length` exclusive (`max-length:0` never passes) | inclusive |
| regex URL-encoded | regex raw, last rule |
| text length = first text child, bytes | full normalized text, characters |
| `val:selector`: zero matches → no finding; children repeat the selected element | occurrence applies to the matches (default 1..unbounded); rules on the `val:selector` itself; children match the selected node's children |
| penalty = element score, subtracted from the score in `Print` | severity (gate) + penalty (weight) per finding; score separate, never combined |
| `val:optional` → 0..unbounded | 0..1 |
| unknown `val:*` silently ignored, typos become literal attributes | lint error |
| no fallback for unknown groups | `default.shape.html` |

## 14. Worked examples

Root: [`demoshop/schema`](../demoshop/schema). The pages are rendered by the demo shop
([`demoshop`](../demoshop)) for `https://shop.example`, and the group comes
from its `x-next-js-app` response header. These numbers are the expected
results, checked by `examples_test.go`.

### 14.1 `index`: the home page `/`

A lead paragraph, two category tiles and a section about the demo. Schema:
[groups/index.shape.html](../demoshop/schema/groups/index.shape.html).

Findings: none.

Result: **score 89 / 117 = 76.1 %, penalty 0**

| path | points | max | why |
|---|---|---|---|
| `head` (7 rules) | 34 | 34 | title, description, canonical and OpenGraph, mostly from the partial |
| `val:ld(WebSite)` | 5 | 5 | |
| `body/main/h1` | 20 | 20 | |
| `body/main/p[class=lead]` | 10 | 10 | |
| `body/main/section[aria-labelledby=categories]` | 16 | 42 | h2 2 + choice 14 |
| ↳ `val:choice` | 14 | 40 | 2 category tiles × 7. Best combination 10 × 7 = 70, capped by `score-max=40` |
| `body/main/section[aria-labelledby=about]` | 4 | 6 | 2 paragraphs × 2, `val:score-max=6` |

Components: category-tile 14/14 (100 %).

### 14.2 `category`: `/category/cars`

Ten product tiles and an `ItemList` in JSON-LD, a host carousel where every
item is a complete product. The station wagon has a name of 75 characters.
Schema: [groups/category.shape.html](../demoshop/schema/groups/category.shape.html).

Findings:

| severity | penalty | rule | path | message |
|---|---|---|---|---|
| warn | 3 | `ld-prop:max-length` | `html/body/val:ld(ItemList)/itemListElement.item.name[4]` | got 75, expected at most 60 |
| warn | 3 | `max-length` | `html/body/main/product-tile[4]/a/h2` | got 75, expected at most 60 |

Locators: `html > head > script:nth-of-type(2)` (the `ItemList` script) and
`html > body > main > div > article:nth-of-type(4) > a > h2`.

Result: **score 130 / 182 = 71.4 %, penalty 6**

| path | points | max | why |
|---|---|---|---|
| `head` (7 rules) | 34 | 34 | |
| `val:ld(BreadcrumbList)` | 5 | 5 | |
| `val:ld(ItemList)` | 0 | 10 | `every` checks all ten names: one failing prop, the list earns nothing |
| `body/main/nav[aria-label=Breadcrumb]` | 3 | 3 | 2 links in `ol/li`, `val:wildcard for="ol, li"` |
| `body/main/h1` | 20 | 20 | |
| `body/main/p[class=lead]` | 10 | 10 | |
| `body/main/val:choice` | 58 | 100 | 10 tiles × 6, the fourth h2 earns nothing. Best combination 40 × 6 = 240, capped by `score-max=100` |

Components: product-tile 58/60 (96.7 %, penalty 3).

The same schema on `/category/robots`: no findings, `val:ld(ItemList)` 10, **score 142 / 182, penalty 0**.

### 14.3 `product`: a clean product `/product/robot-01`

Clanky the Butlerbot: complete merchant listing data in the `Product` JSON-LD,
the product info in wrapper `div`s and two feature sections. Schema:
[groups/product.shape.html](../demoshop/schema/groups/product.shape.html).

Findings: none.

Result: **score 121 / 145 = 83.4 %, penalty 0**

| path | points | max | why |
|---|---|---|---|
| `head` (8 rules) | 36 | 36 | the partial + `product:price:amount` |
| `val:ld(Product)` | 31 | 31 | 20 for the object + 11 for the optional merchant listing props |
| `val:ld(BreadcrumbList)` | 5 | 5 | |
| `body/main/nav[aria-label=Breadcrumb]` | 3 | 3 | |
| `body/main/article` | 46 | 70 | |
| ↳ `figure` | 5 | 5 | figure 2 + img 3 |
| ↳ `h1` | 20 | 20 | 2 wrappers deep, `val:wildcard for="div"` |
| ↳ `p[class=price]` | 5 | 5 | |
| ↳ `p[class=description]` | 4 | 10 | 21 words → 2 × 2, `val:score-per="words:10"` |
| ↳ `val:choice` | 12 | 30 | 2 feature sections × 6 (h2 2 + 2 dt + 2 dd) of 10 |

Components: feature-section 12/20 (60 %).

### 14.4 `product`: four flawed products

Same group and schema as 14.3. Each product has one deliberate flaw, and it
shows up wherever the page repeats it.

**Tiny Tim** (`/product/robot-07`): a description of one word, "Beeps.".
**Score 85 / 145, penalty 40.**

| severity | penalty | rule | path | message |
|---|---|---|---|---|
| error | 10 | `attr:min-length` | `html/head/meta[name=description]@content` | got 6, expected at least 70 |
| error | 10 | `attr:min-length` | `html/head/meta[property=og:description]@content` | got 6, expected at least 70 |
| error | 10 | `ld-prop:min-length` | `html/body/val:ld(Product)/description` | got 6, expected at least 70 |
| error | 10 | `min-words` | `html/body/main/article/p[class=description]` | got 1, expected at least 12 |

A failing required prop costs the `Product` object its 20 points, while its
optional props still earn their own: `val:ld(Product)` 11 / 31.

**Lazybot Deluxe** (`/product/robot-10`): no feature sections at all.
**Score 109 / 145, penalty 10.**

| severity | penalty | rule | path | message |
|---|---|---|---|---|
| error | 10 | `occurrence` | `html/body/main/article/val:choice` | got 0, expected 1..6 |

Locator: `html > body > main > article`, the context they are missing from.

**The station wagon** (`/product/car-04`): a name of 75 characters, 94 in the title.
**Score 69 / 145, penalty 90.**

| severity | penalty | rule | path | message |
|---|---|---|---|---|
| error | 20 | `max-length` | `html/head/title` | got 94, expected at most 60 |
| error | 10 | `attr:max-length` | `html/head/meta[property=og:title]@content` | got 94, expected at most 90 |
| error | 10 | `ld-prop:max-length` | `html/body/val:ld(Product)/name` | got 75, expected at most 60 |
| error | 50 | `max-length` | `html/body/main/article/h1` | got 75, expected at most 60 |

The title and the h1 carry their own `val:penalty`, so one long name weighs 90.

**The Crypto Cruiser** (`/product/car-09`): priced in `DOGE`.
**Score 101 / 145, penalty 30.**

| severity | penalty | rule | path | message |
|---|---|---|---|---|
| error | 10 | `attr:one-of` | `html/head/meta[property=product:price:currency]@content` | "DOGE" is not one of EUR\|USD\|CHF |
| error | 10 | `ld-prop:one-of` | `html/body/val:ld(Product)/offers.priceCurrency` | "DOGE" is not one of EUR\|USD\|CHF |
| error | 10 | `ld-prop:one-of` | `html/body/val:ld(Product)/offers.shippingDetails.shippingRate.currency` | "DOGE" is not one of EUR\|USD\|CHF |

The category schema allows `DOGE` in its `ItemList`, so `/category/cars` has no currency finding.

### 14.5 Fallback: the 404 page

The 404 page sends no group header, so it is validated with
[default.shape.html](../demoshop/schema/default.shape.html). No findings, **score 41 / 44, penalty
0**. The optional `og:image` is missing: 0 of 3 points, no finding.

## 15. Decision log

First feedback round:

1. **Wrappers**: no global projection. `val:wildcard for="…"` instead, with lexical, inherited scope. It replaces the draft's `val:deep`, `val:strict` and `val:transparent`.
2. **Settings**: per schema (`val:settings`, shareable via `val:include`). `aria-hidden` is not excluded by default.
3. **Shared rules**: `default.shape.html` is a fallback only. Shared rules live in partials (see [14](#14-worked-examples)).
4. **Component contexts**: closed by default, error. `val:open` relaxes to info.
5. **Score vs penalty**: two separate measures, never combined. Nodes earn independently of their children. No negative points. Penalty per finding from `val:penalty`, otherwise a default per severity.
6. **Component overrides**: `val:severity` at the instantiation site is a ceiling. Penalties unchanged.
7. **Choice maximum**: exact, greedy best combination.
8. **Schema syntax**: own tree builder on the x/net/html tokenizer, no HTML5 error recovery, line numbers.
9. **Input**: any HTML. The server response is the intended input, and rendered snapshots work too.
10. **Marker attribute**: `data-component` by default.

Second feedback round:

11. **Overlapping selectors**: `component-ambiguous` finding, the instance is not validated. Authors disambiguate with CSS (`:not(…)`) or markers. No priorities.
12. **Occurrence penalties** scale with the deviation: one finding per surplus node, penalty × count for missing nodes, optional `val:penalty-max`.
13. **Locators**: every finding carries a CSS path to the page node (or to the context of missing nodes).

Third feedback round:

14. **Includes and instances**: `<ref>` and `val:use` are replaced by `val:include` (insert any file) and `val:comp` (instantiate a single-root file, attributes override the root's). Paths are `/`-rooted or relative, without `.shape.html`. File organization is free. Recognized components are the files with a `val:component` root, identified by path. `marker`/`selector` can't be overridden.

15. **Spelling**: no `val:` prefix on attributes of `val:*` elements, except `val:comp`. `for` is a CSS selector everywhere (`val:selector for="h1"`, `val:wildcard for="div, span"`). Page-wide rules live in `/partials/page-baseline`, kept lean to avoid double findings with component rules.

Fourth feedback round:

16. **Unknown markers**: a `data-component` value without a component schema is ignored entirely - no `component-unknown` finding, and the element is not opaque. Projects mark many UI components (`Icon`, `Picture`, `Link`) that the SEO schema does not care about.

Fifth feedback round:

17. **`every` on `val:prop`**: by default one array element satisfies a property path ("has at least one offer in EUR"). `every` checks all of them ("every product of the list is in a valid currency") with a finding per failing element.

## 16. Open questions

None at the moment.
