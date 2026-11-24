# Demo

appstoreanalytics reading an invented App Store Connect account, in Docker: six apps of one
small studio, with analytics reports back to 2024 and a new day each day. What it read, its
sign-ins and its webhooks are kept from one run to the next, so a browser signed in stays signed
in.

```sh
./scripts/run_dev.sh
```

from the top of the checkout. It builds and starts everything, says when the first sync has read
the account, which takes about half a minute the first time, and follows appstoreanalytics' log;
then open <http://localhost:8080> and sign in as **demo**, password **demodemo**. Ctrl-C stops
it. `./scripts/run_dev.sh --fresh` starts without what earlier runs kept.

Where it is opened is `DEV_URL` in `.env.dev`, which is yours and is not committed: copy
[`.env.dev.example`](../.env.dev.example), such as `DEV_URL=http://appstoreanalytics.localhost:3020`.
Its webhook addresses are shown under it, and it is served on its port, 80 or 443 when it names
none. With a proxy in front answering at the address, `DEV_PORT` is the port the proxy passes to.
Each is read from the environment first, so `DEV_URL=http://localhost:9090 ./scripts/run_dev.sh`
serves there whatever the file says.

Without the script: `docker compose -f docker-compose.dev.yml up --build`, and `down` to stop,
with `--volumes` to forget what runs kept. `docker compose -f docker-compose.dev.yml logs seed`
says when the account has been read.

## What is in it

| app | | |
| --- | --- | --- |
| Knotwork | a puzzle game | The most downloads; hints, coins and no ads to buy. Busier at weekends. |
| Tidepool | a habit tracker | The most proceeds, from subscriptions; every January's resolutions. |
| Snowline | snow forecasts | Winter only, and mostly where there are mountains. |
| Beatkeeper | a metronome | A Pro unlock, and a rise when schools go back. |
| Herbarium | a field guide to plants | A paid app, in spring and summer. |
| Fieldnote | a journal | Launched in June 2025, so a year compares with nothing before it. |

Each has a new version every few weeks, which brings a run of updates; one version in five
crashes until the fix after it. Every app is featured by the App Store for a week now and then,
when browsing brings more visitors than searching. Christmas brings new phones.

Tidepool and Knotwork each have an App Store Connect webhook to an address of their own, which
has been sent a build, its beta review, a tester's feedback and a version through App Review;
Knotwork's version was rejected once. Test on either address asks the stand-in, which sends a
`webhookPingCreated`. No mail relay is set, so nothing is mailed.

## How it is put together

- `appstoreconnect`, a stand-in for App Store Connect in [appstoreconnect/](appstoreconnect/):
  the apps, analytics report requests and their reports, and webhooks, answering as
  `api.appstoreconnect.apple.com` on the demo's network. It makes a certificate authority on its
  first start, which appstoreanalytics is told to trust, and the API key appstoreanalytics signs
  with, and refuses a token signed with any other. A snapshot is delivered the day it is asked
  for, rather than a day or two later, and an ongoing request delivers each day from then.
  Every figure is made from the app and the day, so a day reads the same whenever it is asked
  for.
- `appstoreanalytics`, built from this checkout, signed in to as demo / demodemo.
- `seed`, the stand-in's own binary, which waits for the first sync to read the account, then
  adds the two webhook addresses in appstoreanalytics and makes their webhooks in the stand-in,
  and stops.

Two named volumes keep what runs keep: `data`, appstoreanalytics' database, and
`appstoreconnect`, the stand-in's keys and what it was asked to make.

There are no sales reports, as no vendor number is set: the analytics reports say everything.
