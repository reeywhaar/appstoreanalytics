# Screenshots

The pictures in the project README, captured from the demo: a real appstoreanalytics reading an
invented App Store Connect account.

```sh
npm --prefix web run screenshots
```

That starts [docker-compose.dev.yml](../../docker-compose.dev.yml) as a Compose project of its own
on a free port, with the stand-in for App Store Connect making the account's days from a fixed
moment, and waits for the seed to give two apps their webhooks. It then adds a mail relay and two
recipients, sets every moment the server keeps from the same one, drives headless Chromium over
the DevTools protocol, and overwrites the PNGs here. Everything it starts is removed on the way
out, its volumes too: each run reads the account afresh, which takes about a minute, and since the
account is made from seeded draws it is the same account every time. Each PNG says its density,
72 dpi times `SCALE`, as a macOS screenshot does.

Needs `node`, `docker` and Chromium or Chrome.

**A run with nothing changed changes nothing.** The account is made from 2026-11-20 18:00 UTC
rather than from when it runs, the webhooks' events are dated and numbered from it, and the
server's own times are set from it once the seed is done: the last sync and the reports read a few
minutes before, each delivery at the time its event says. The webhook addresses' random tokens
are swapped for stand-ins. The page's clock starts at the same moment, in UTC and `en-US`, so how
long ago a time was reads the same; the caret is hidden and animations finished. A new shot leaves
the file already there alone when every 8 by 8 block of CSS pixels has the same average colour in
both, within 24 levels: a Chromium update redraws the edges of letters all over a picture, and
that alone is not a new picture.

| knob | |
| --- | --- |
| `SCALE=2` | device pixels per CSS pixel |
| `THEME=dark` | the other theme; `OUT=/tmp/shots` keeps it off the committed set |
| `ONLY=status` | just those shots |
| `NOTES=0` | without the notes |
| `TIMEOUT=60` | seconds a shot may take; one that runs over says what it was waiting for |

**The data** is the demo's, [dev/appstoreconnect/account.go](../../dev/appstoreconnect/account.go):
six apps, what they sell, and how each one's days go. See [dev/README.md](../../dev/README.md).

**The shots** are the list at the bottom of
[`capture.mjs`](../../web/scripts/screenshots/capture.mjs): where to go, what to do there, and
optionally an element or the sections to crop to, or a phone to take it as. Each can carry notes:
small popovers drawn over the page, pointing at what they explain.
