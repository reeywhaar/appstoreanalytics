# API

The interface does everything through these endpoints, and a program can too. JSON in, JSON
out; paths are relative to the instance's address.

## Signing in

```sh
curl -c jar -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}' https://stats.example.com/api/auth/login
curl -b jar 'https://stats.example.com/api/report?days=7'
```

`POST /api/auth/login` takes the username and password of the environment (the username without
regard to case) and answers `204` with the cookie `appstoreanalytics_auth`: `HttpOnly`,
`SameSite=Lax`, `Secure` over https. It lasts 30 days from when it was last used. Attempts are
limited: 30 at once for the instance and then one every 2 seconds, and 5 for a username and then
one every 20 seconds, past which the answer is `429 rate_limited`.

Everything else needs the cookie, and answers `401 unauthenticated` without it.

| | |
| --- | --- |
| `POST /api/auth/logout` | ends the session, `204` |
| `GET /api/auth/me` | `{"username": "…"}` |

## Refusals

Always `{"ok": false, "code": "…", "message": "…"}`. The code is the contract; the message is
written for a person and may change.

| status | code |
| --- | --- |
| 400 | `invalid` |
| 401 | `unauthenticated` |
| 403 | `forbidden`: a write from another site |
| 404 | `not_found` |
| 413 | `body_too_large` |
| 415 | `unsupported_media_type`: a write whose body is not JSON |
| 429 | `rate_limited` |
| 500 | `internal` |

## Apps

`GET /api/apps` lists every app, by name: `[{"id", "name", "bundle_id"}]`. An id is the app's
Apple ID.

## A report of a range of days

`GET /api/report`

| parameter | |
| --- | --- |
| `days` | how many days, 1 to 731; 30 when left out |
| `to` | the last day, `YYYY-MM-DD`; the last day downloads were measured when left out |
| `from` | the first day, in place of `days` |
| `app` | an app's Apple ID; every app when left out |

```json
{
  "from": "2026-10-16", "to": "2026-11-14", "previous_from": "2026-09-16", "app": "",
  "days": [{"day": "2026-11-14", "first_downloads": 212, "proceeds": 61.25, "impressions": 3010}],
  "totals": {"first_downloads": 6708, "proceeds": 1875.25, "impressions": 90700},
  "covered": {"first_downloads": 30, "proceeds": 30, "impressions": 29},
  "previous": {"first_downloads": 6390, "proceeds": 1630.1},
  "previous_covered": {"first_downloads": 30, "proceeds": 30},
  "countries": [{"id": "US", "name": "US", "first_downloads": 2707, "proceeds": 1230.78, "page_views": 9815}],
  "sources": [{"id": "App Store search", "name": "App Store search", "page_views": 9815,
               "impressions": 54678, "first_downloads": 2917}],
  "apps": [{"id": "1234567890", "name": "Dots", "proceeds": 1875.25, "first_downloads": 5175}],
  "in_app": [{"id": "1234567891", "name": "100 Coins", "app_name": "Dots", "proceeds": 301,
              "purchases": 430}]
}
```

The measures are `first_downloads`, `redownloads`, `updates`, `proceeds`, `purchases` (in-app),
`impressions`, `page_views`, `installs`, `deletions`, `sessions` and `crashes`. Proceeds are US
dollars after Apple's commission and taxes; everything else is a count.

A measure missing from a day is not known for it yet, which is not zero. `covered` counts the
days each total is of. `previous` and `previous_covered` are the same of the as many days before
`from`, starting at `previous_from`. Each breakdown is largest first by its first measure.

## Every day, for scrolling back

`GET /api/history`, with `app` as for a report, is every day measured, at most three years of
them, up to the day a report without `to` ends on: `{"from", "to", "app", "days"}`, where `days`
are as a report's. The charts scroll back through it, while the totals and breakdowns are of the
range picked.

## The sync

`GET /api/status` says how each part of the sync last went, with times in Unix seconds:

```json
{
  "running": true,
  "reading": "analytics",
  "apps": {"count": 2, "part": {"ran_at": 1763251200, "ok_at": 1763251200, "error": ""}},
  "analytics": {"first": "2024-01-01", "last": "2026-11-14", "instances": 412,
                "requested_apps": 8, "requested_since": 1763244000,
                "part": {"ran_at": 1763251200, "ok_at": null, "error": "…"}},
  "sales": {"on": true, "first": "2025-11-16", "last": "2026-11-15", "days": 365, "part": {"…": "…"}},
  "rates": {"at": 1763251200, "part": {"…": "…"}},
  "requests": [{"app_id": "1234567890", "name": "Dots", "ongoing": "running", "snapshot": true,
                "since": 1763244000, "delivered": 0, "latest": "", "read": 0}]
}
```

`reading` is the part under way, and `""` between passes. `error` is why the part last did not
go through, in words for a person, and `""` when it did.

`sales` is the sales reports read, which fill in what analytics reports have not said yet; `on` is
whether a vendor number is set, and without one there are none. `rates` is when exchange rates
for their proceeds were last read.

`requests` is where each app's analytics reports stand with Apple. `ongoing` is `"running"`,
`"stopped"` when Apple stopped the daily reports, or `""` when none were asked for; `snapshot` is
whether history was asked for. `since` is when they were first asked for, or first seen when
something else asked. `delivered` is how many report instances Apple lists, `latest` the newest
one's processing date, and `read` how many have been read. Apple has no "pending": nothing
delivered means Apple is still preparing them, usually for a day or two.

`POST /api/sync` starts a pass now and answers `202` with `{"started": true}`, or
`{"started": false}` when one is running already.

`GET /healthz` answers `{"ok": true, "version": "…"}` to anybody.
