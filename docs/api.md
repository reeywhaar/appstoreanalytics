# API

The interface does everything through these endpoints, and a program can too. JSON in, JSON
out; paths are relative to the instance's address.

## Signing in

```sh
curl -c jar -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}' https://stats.example.com/api/auth/login
curl -b jar https://stats.example.com/api/sales?days=7
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

`GET /api/apps` lists every app known, by name: `[{"id", "name", "bundle_id", "sku"}]`. An id
is the app's Apple ID.

## A range of days

`/api/sales` and `/api/analytics` take the same query:

| parameter | |
| --- | --- |
| `days` | how many days, 1 to 731; 30 when left out |
| `to` | the last day, `YYYY-MM-DD`; the last day whose sales report has been read when left out |
| `from` | the first day, in place of `days` |
| `app` | an app's Apple ID; every app when left out |

Each answers with the range it read, `from`, `to`, `app`, and `previous_from`, the first of as
many days before `from`, which `previous` totals are of.

## Sales

`GET /api/sales`. Proceeds are in US dollars.

```json
{
  "from": "2026-10-16", "to": "2026-11-14", "previous_from": "2026-09-16", "app": "",
  "days": [{"day": "2026-11-14", "downloads": 212, "redownloads": 14, "updates": 530,
            "in_app": 18, "proceeds": 61.25}],
  "totals": {"downloads": 6708, "redownloads": 457, "updates": 8311, "in_app": 505,
             "proceeds": 1875.25, "days": 30},
  "previous": {"…": "the same, of the days before"},
  "countries": [{"id": "US", "name": "US", "downloads": 2707, "units": 9000, "proceeds": 1230.78}],
  "apps": [{"id": "1234567890", "name": "Dots", "downloads": 5175, "units": 0, "proceeds": 1875.25}],
  "in_app": [{"id": "1234567891", "name": "100 Coins", "downloads": 0, "units": 430, "proceeds": 301}],
  "unconverted": []
}
```

`days` holds only days whose report has been read; a day missing from it is not known yet.
`totals.days` and `previous.days` count them. `unconverted` names currencies with proceeds and no
rate, which are left out of every sum of proceeds.

## Analytics

`GET /api/analytics`.

```json
{
  "from": "…", "to": "…", "previous_from": "…", "app": "",
  "days": [{"day": "2026-11-13", "impressions": 3010, "page_views": 540, "sessions": 1100}],
  "totals": {"impressions": 90700, "page_views": 16200},
  "covered": {"impressions": 29, "page_views": 29},
  "previous": {}, "previous_covered": {},
  "sources": [{"source": "App Store search", "impressions": 54678, "page_views": 9815,
               "first_downloads": 2917}]
}
```

The measures are `impressions`, `page_views`, `first_downloads`, `redownloads`, `installs`,
`deletions`, `sessions` and `crashes`. A measure missing from a day is not known for it yet;
`covered` counts the days each total is of.

## The sync

`GET /api/status` says how each part of the sync last went, with times in Unix seconds:

```json
{
  "running": false,
  "apps": {"count": 2, "part": {"ran_at": 1763251200, "ok_at": 1763251200, "error": ""}},
  "sales": {"first": "2025-11-15", "last": "2026-11-14", "days": 365, "part": {"…": "…"}},
  "analytics": {"first": "2026-07-19", "last": "2026-11-13", "instances": 412, "part": {"…": "…"}},
  "rates": {"at": 1763251200, "part": {"…": "…"}}
}
```

`error` is why the part last did not go through, in words for a person, and `""` when it did.

`POST /api/sync` starts a pass now and answers `202` with `{"started": true}`, or
`{"started": false}` when one is running already.

`GET /healthz` answers `{"ok": true, "version": "…"}` to anybody.
