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
| 502 | `relay_failed`: the mail relay refused, in its own words |

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
  "countries": [{"id": "US", "name": "US", "first_downloads": 2707, "proceeds": 1230.78, "page_views": 9815,
                 "by_app": {"1234567890": {"first_downloads": 2707, "proceeds": 1230.78, "page_views": 9815}}}],
  "sources": [{"id": "App Store search", "name": "App Store search", "page_views": 9815,
               "impressions": 54678, "first_downloads": 2917}],
  "apps": [{"id": "1234567890", "name": "Dots", "proceeds": 1875.25, "first_downloads": 5175}],
  "in_app": [{"id": "1234567891", "name": "100 Coins", "app_name": "Dots", "proceeds": 301,
              "purchases": 430}],
  "by_app": {"1234567890": {"2026-11-14": {"first_downloads": 150, "proceeds": 61.25}}}
}
```

The measures are `first_downloads`, `redownloads`, `updates`, `proceeds`, `purchases` (in-app),
`impressions`, `page_views`, `installs`, `deletions`, `sessions` and `crashes`. Proceeds are US
dollars after Apple's commission and taxes; everything else is a count.

A measure missing from a day is not known for it yet, which is not zero. A day its report has
covered and said nothing of comes at zero: see
[app-store-connect.md](app-store-connect.md#a-day-a-report-says-nothing-of-had-none-and-a-day-after-its-last-is-not-reported-yet).
`covered` counts the days each total is of. `previous` and `previous_covered` are the same of the as many days before
`from`, starting at `previous_from`. Each breakdown is largest first by its first measure.

`by_app` is each app's own days, by app id and then by day, for charts that tell the apps apart;
empty when one app is asked for. An app missing from a day, or a measure missing from an app's
day, had none of it on a day `days` knows.

A row of `countries`, `sources` or `in_app` has a `by_app` of its own when every app is asked for:
each app's part of the row, by app id, in the row's measures, for bars that show whose they are.
The rows of `apps` have none, being each one app's already.

## Every day, for scrolling back

`GET /api/history`, with `app` as for a report, is the days measured a year at a time:
`{"from", "to", "first", "app", "days", "by_app"}`, where `days` and `by_app` are as a report's.
Without `before` it is the year up to the day a report without `to` ends on; with
`before=YYYY-MM-DD`, the year before that day. `first` is the first day anything was measured,
so an answer whose `from` is `first` is the earliest there is. The charts start on the latest
year and ask for the one before as they are scrolled back to their start, while the totals and
breakdowns are of the range picked.

## Mail relay

Where messages are handed over. There is one or none.

| | |
| --- | --- |
| `GET /api/settings/relay` | `{"configured", "host", "port", "security", "username", "password_set", "from_address", "from_name"}`; the password never comes back, only whether there is one |
| `PUT /api/settings/relay` | the same fields, with `password` in place of `password_set`; answers as `GET` does |
| `DELETE /api/settings/relay` | forgets it, `204` |
| `POST /api/settings/relay/check` | the relay as the form has it, saved or not, `{"host", "port", "security", "username", "password"}`: reaches it, secures the connection and signs in, and sends nothing, `204`, or `502 relay_failed` with the relay's own words. An empty `password` with the saved `username` is the saved password |
| `POST /api/settings/relay/test` | `{"to": "…"}`: sends one message through the saved relay, `204`, or `502 relay_failed` with the relay's own words |

`security` is `starttls` or `implicit`; there is no plain connection. A `PUT` with an empty
`password` and the same `username` keeps the stored password. A username and a password come
together or not at all.

## Webhooks

`POST /hooks/{token}` takes whatever a webhook sends, in any content type, up to 1 MB, and
answers `200 {"ok": true}`. It needs no session: the token names one of the addresses, and one
that names none, or an address removed, is `404 not_found` like any path that is not there. The
newest 1,000 deliveries are kept, each with the address it came through and what checking its
signature found. Only a delivery proven to be Apple's is mailed; one that fails is kept and
marked. See [App Store Connect](app-store-connect.md#a-webhook-is-mailed-only-when-it-is-proven-to-be-apples).

| | |
| --- | --- |
| `GET /api/hooks` | `{"endpoints", "mails", "events", "catalog"}`: the addresses, everybody mailed as `[{"id", "to", "events", "endpoints"}]`, every kind of event kept with its `count` and `last_at`, and every kind Apple documents, by service and then by group, as `[{"name", "groups": [{"name", "kinds": [{"name", "title", "meaning"}]}]}]` |
| `POST /api/hooks/endpoints` | `{"label": "…", "secret": "…"}`: a new address, `201` with its `secret`, the one time it is shown; an empty `secret` is generated, 32 letters and digits |
| `POST /api/hooks/endpoints/{id}/test` | finds what in App Store Connect sends to the address and asks Apple to send each a test: `{"senders": [{"kind", "app", "name", "result", "message"}]}`. `kind` is `notifications`, an app's App Store Server Notifications, whose `name` is `Production` or `Sandbox`, or `webhook`, whose `name` is the webhook's. `result` is `asked`, `off` for a webhook turned off, `no_key` for notifications with no In-App Purchase key to ask with, or `failed`, with `message` saying why. `502 apple_failed` when App Store Connect cannot be asked at all |
| `DELETE /api/hooks/endpoints/{id}` | removes one; it answers `404` from then, and what came through it is kept. App Store Connect's webhooks to it are deleted: `204`, or `200 {"left": ["…"]}` saying what still sends to it, a webhook Apple would not delete or server notifications |
| `GET /api/hooks/apple` | where App Store Connect sends each app's three parts: `{"apps": [{"id", "name", "webhook", "production", "sandbox"}]}`, each part `{"state", "url", "endpoint", "name"}`. `state` is `ok`, sent to the address `endpoint`; `off` or `some`, a webhook to it turned off or sending only some events; `none`, nowhere; or `unknown`, to `url`, which is none of the addresses. `502 apple_failed` when App Store Connect cannot be asked. See [App Store Connect](app-store-connect.md#an-apps-webhook-and-notifications-are-connected-by-hand) |
| `POST /api/hooks/apple/{app}/connect` | `{"part": "webhook", …}` sends one of an app's parts, `webhook`, `production` or `sandbox`, somewhere else: `"auto": "main"` to the app's one automatic address for all three parts, or `"auto": "separate"` to the part's own, made the first time; `"endpoint": 1` to one of the addresses; `"url": "https://…"` to one typed in, with `"secret"` for a webhook; with none of these, nowhere. `{"result", "sender"}`: `result` is `made` for a webhook made, `changed`, or `already`; `sender` is the part as it now stands, as it was just set, since App Store Connect asked at once can still say how it was; with `"endpoint"`, the address, when one was made for it. Auto takes an address added by hand that the app sends to already, or that has the very label it would give, rather than making one. `502 apple_failed` with Apple's words when it refuses |
| `GET /api/hooks/deliveries` | `{"deliveries": [...]}`, the newest 50, newest first |
| `GET /api/hooks/{id}` | one delivery with its `content_type`, its `body` as it came, and `decoded`, the body with every signed part opened, or `null` when it is not JSON |
| `POST /api/hooks/mails` | `{"to": "…", "events": ["…"], "endpoints": [1]}`: somebody to mail, through the relay, of those kinds of event that came through those addresses, `201`. A type mails its subtypes too. An email address and at least one kind are needed; `endpoints` empty or left out is every address, those added later too, and one there is not is refused. An address removed stays in the list and matches nothing |
| `PUT /api/hooks/mails/{id}` | the same, changing one |
| `DELETE /api/hooks/mails/{id}` | stops mailing one, `204` |

An address is `{"id", "label", "url", "created_at", "count", "last_at", "secret_set", "app",
"part"}`: `count` and `last_at` are of the deliveries kept that came through it, `secret_set` is
false only for the address carried over from when there was one, and `app` and `part` are the
app's id and the part an address was made for by Auto, `main` for all three, `""` for one added by
hand.

A delivery is `{"id", "received_at", "via", "event", "summary", "size", "proven_by",
"proof_error", "mailed_at", "mailed_to", "mail_error"}`. `via` is the label of the address it
came through, as it was then. `event` is an App Store Server Notification's type and subtype,
like `SUBSCRIBED/INITIAL_BUY`, an App Store Connect webhook's `data.type`, a top-level `type` or
`event`, or `unknown`. `proven_by` is `apple` for a notification whose signatures lead to
Apple's root, `secret` for a webhook signed with its address's secret, and `""` for one not
proven; `proof_error` says why a signature failed, and is `""` for one with nothing to check.
`mailed_at` is when it was handed to the relay, `null` for one not to be mailed, `mailed_to`
whom it was mailed to, each once however many of their entries chose it and each in a message of
their own, and `mail_error` why the relay did not take it, to which of them.

## Sessions

| | |
| --- | --- |
| `GET /api/sessions` | `{"sessions": [{"id", "current", "created_at", "last_seen_at", "expires_at", "device", "user_agent"}]}`, the most recently used first |
| `DELETE /api/sessions/{id}` | signs that one out, `204`; the current one too, which clears its cookie in the same answer |
| `DELETE /api/sessions` | signs out every session but the current one, `204` |

An `id` is `s_` and 12 hex characters, derived from the session and never from the cookie
value. `device` is a guess at the browser and platform, and `user_agent` what it was guessed
from.

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
