# Conventions

Words, comments, commits, code, time. Settled once here so it is not re-argued in review.

## Words

One name per thing, in identifiers, the API, the docs and the interface.

| word | means | in code |
| --- | --- | --- |
| app | an app of the App Store Connect account | `App`, `apps`, `app_id` |
| product | anything a sales report counts: an app, or an in-app purchase | `Product`, `products`, `product_id` |
| sale | one line of a sales report, summed over what is not kept | `Sale`, `sales` |
| in-app purchase | what is bought inside an app, subscriptions included | `in_app` |
| measure | one number of an analytics report: impressions, sessions and the rest | `Measured`, `metric`, `metrics` |
| report day | the calendar day a report is about, in Apple's Pacific time | `day` |
| sync | reading App Store Connect into the database | `pull`, `Pass` |

**"Download" is a first-time download.** A redownload and an update are their own words and
are never counted as downloads.

## American spelling

`color`, `favorite`, `canceled`. It is the dialect everything this is written against already
spells in, CSS and the DOM included, and one word has one spelling everywhere.

## Comments

**Why, never what.** A comment restating the line under it is noise. A comment recording the
reason a line is written the way it is stops somebody "simplifying" it back into a bug.

**No prose.** A sentence or two. If it needs more, the argument belongs in `docs/` and the
comment points at it.

**No history.** Never "this used to be X". Git holds that. A comment describes the code as it
is, to somebody who has never seen any other version of it.

**No negations.** A comment explains the code that is there, not what is absent and why. A
decision not to do something belongs in `docs/`, where somebody asking the question finds it.

**Say it once.** The same reason in a function, its test and its caller is three copies to keep
true.

**Package doc comments are expected**, and are the right place for the argument a package
exists to make.

## Docs

`docs/` holds the decisions, one file per subject. A heading states the decision and the text
under it gives the reason:

```md
## A sales day is read again until it settles
```

Not `## Sales days`. A reader scanning the headings learns what was decided without opening a
section. A decision is written down in the change that makes it.

## Commits

**One or two lines.** A declarative sentence saying what the change accomplishes. Capitalized,
no trailing period, no prefix, no conventional-commit tag, no ticket number. No trailers, no
`Co-Authored-By`, no generated-with footer.

```
A refused key stops the sales part at once rather than after five days of the same refusal
```

## Go

- `gofmt` clean; CI fails on anything it would rewrite.
- Tests beside sources as `*_test.go`, named as sentences:
  `TestAMissingSalesReportIsToldApartByWhatAppleSays`.
- Errors wrap with `%w` and name what was being done. What a person is shown is written for
  them, in `store.Invalid`/`NotFound`/`Conflict` or in the sync's `why`.
- `context.Context` first on anything that can block. Everything that talks to Apple can.
- Injectable clocks (`Store.SetClock`, `Puller.now`), so a test drives time rather than
  sleeping through it.
- `internal/store` is the only package that writes SQL. `internal/asc` is the only one that
  speaks to App Store Connect, and knows nothing of the database.

## TypeScript

- Prettier, default settings; oxlint.
- Components are `PascalCase.tsx`; everything else is `camelCase.ts`.
- Imports use the `@app/*` alias, never `../../`.
- `strict`, `noUncheckedIndexedAccess`, `noUnusedLocals`, `noUnusedParameters`,
  `verbatimModuleSyntax`.
- Server state is TanStack Query's; where the app is, is the address bar's. Nothing else holds
  state that outlives a component.
- Colors are semantic tokens in `web/src/main.css`. A component names what a color is for
  (`series`, `up`, `down`), never a hex.

## Ids are Apple's

An app is its Apple ID, the number in its App Store link; a product its Apple Identifier; an
analytics report instance its own id. Nothing here mints ids, because nothing here exists that
Apple does not already name. An id is checked for its shape before it reaches a query, so a typo
is a `400` rather than an empty answer.

## Time

- `main.go` pins `time.Local = time.UTC`. Every moment stored and logged is UTC.
- **A moment is Unix seconds in an `INTEGER` column**: when a report was read, when a session
  lapses.
- **A report day is `TEXT`, `YYYY-MM-DD`, and is not a moment.** Apple's day runs midnight to
  midnight Pacific time; made into a moment it would shift into the day before or after in any
  other zone. It is stored, sent and shown as written. The interface handles it as noon UTC of
  that date, which no zone moves to another day.
- Dates are shown as ISO 8601 writes them, `2026-11-14`, whatever the locale.

## Money

Proceeds are stored in millionths of their currency, as integers, so sums are exact. They are
converted to US dollars only when read, at the rates of the day; see
[app-store-connect.md](app-store-connect.md).

## Migrations

Named `<timestamp>_<snake_case_name>.go`, the timestamp being the UTC moment the file was
written, `date -u +%Y%m%d%H%M%S`, which is their order. **Never edit a released migration**:
every database past it has recorded it as applied.

## Logging

`log/slog`, JSON, one line a record. Log what an operator needs: a sync part that did not go
through and why, a sign-in refused, every browser signed out by a changed password, how many
days or reports a pass read.

**Never** the password, the private key, a token or a cookie value. Sales and app names are the
account's own business and stay out of the log too; an app is named by its id.
