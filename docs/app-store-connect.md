# App Store Connect

What is read from Apple, how, and what it means.

## The Analytics Reports API is the source

Apple is retiring Sales and Trends, its dashboards and reports, through 2026 and 2027, and
points programmatic exports at the Analytics Reports API. So that API is the source: its
downloads report for downloads, redownloads and updates, its purchases report for proceeds and
in-app purchases, and the rest for how the App Store pages did. Sales reports only fill in
until it has spoken, below.

What that costs: the numbers arrive a day or two after the day they are about rather than the
next morning, and Apple leaves out counts too small to share. What it gives: proceeds in US
dollars, as Apple converted them, and history back to 2024 rather than a rolling year.

## Sales reports fill in what analytics reports have not said yet

Given a vendor number, the daily sales summaries are read too, for the year Apple keeps them,
and stand in for downloads, redownloads, updates, in-app purchases and proceeds wherever
analytics reports have not said an app's day yet: before the first analytics reports arrive, and
for the day or two they run behind.

**Analytics wins, per app, day and measure.** A sales report's numbers are kept as placeholders.
An analytics report of the same app's day replaces them, as a later instance replaces an earlier
one, and a sales report read after that leaves what analytics said alone. So the numbers of a
day can change once, from the sales report's to Apple's analytics, which leave out small counts.

**Purchases are the purchases report's on every day it has processed.** It counts every
purchase, however few: a day between the first and the last it said anything of, and says
nothing of, had none. And it dates a purchase the day after the sales report does, which is in
Pacific time, so a sales report's purchase on such a day is the report's purchase of the next,
and kept would be counted twice. The other reports leave small counts out, so a day they say
nothing of keeps what the sales report counted: there it is the only count there is.

**A sales report is read until it settles.** `GET /v1/salesReports`, `DAILY`, `SALES`,
`SUMMARY`, version `1_0`. Its territories arrive at different hours, so a day read within 36
hours of its end is read again each pass. A `404` "There were no sales for the date specified."
is a day of nothing; "Report is not available yet" is asked again, and after five days taken as
nothing; a `410` is older than Apple keeps.

**What a line counts is its product type.** `1…`/`F1…` a download, `3…` a redownload, `7…` an
update, `IA…`/`FI…` an in-app purchase. An in-app purchase names its app by the app's SKU.

**Its proceeds become dollars at the latest rates**, from
[Exchange Rate API](https://www.exchangerate-api.com)'s free `open.er-api.com/v6/latest/USD`,
read once a day; nothing about the account is sent. A currency with no rate is left out. Today's
rate for an older sale is near what Apple paid, which is one more reason analytics replaces it.

## One team key, with the Admin role

**Users and Access → Integrations → App Store Connect API → Team Keys.** The page shows the
issuer id above the list; a key's id is in its row, and its `.p8` downloads once.

The key goes into the environment as base64, so it fits on one line:

```sh
base64 -i AuthKey_XXXXXXXXXX.p8 | tr -d '\n'
```

Asking Apple to make an app's reports takes the **Admin** role, once per app. Reading them takes
Admin, Finance, or Sales and Reports, so a key with one of those works once an Admin key has
asked.

## Only report requests are written to Apple

They are made, and a snapshot with nothing left is deleted to be made again; nothing else is
changed. Each app gets two, asked for on the first sync:

- an **ongoing** request, which makes reports every day from the day it is asked for;
- a **snapshot**, which makes one report of the history there is, back to 2024.

The first reports come a day or two later. Apple ends an ongoing request nobody reads, and a
sync asks again. Apple allows a snapshot about once a month, and refuses another with a
conflict, which is taken to mean there is one.

Requests live at Apple, not here, so a new database finds the ones already made and reads what
they still hold.

## A snapshot that delivers nothing for a week is asked for again

Apple deletes report instances after 35 days, so a database made more than a month after the
snapshot finds the request and none of its history. A snapshot that has listed nothing since it
was first seen here, a week on, is taken as one of those: it is deleted and history is asked for
again. A week, because Apple delivers the first within a day or two.

Apple allows a snapshot about once a month, deleted ones included. When it refuses the new one,
there is no snapshot until it allows one, and every pass asks.

## A sync runs every hour, and on demand

A pass lists the apps, then reads every report instance not read yet, and keeps how each part
went for the status page. It goes on from where the last stopped, so a pass cut short loses
nothing but time, and records nothing: the server stopping is not a part failing.

**A refused key stops the pass at once**, since every other request would be refused the same
way. **Apple asking to slow down (`429`) stops it too**, and the next pass carries on.

## Six Standard reports are read

| report | measures |
| --- | --- |
| App Downloads Standard | first-time downloads, redownloads, updates (manual and automatic) |
| App Store Purchases Standard | proceeds in US dollars, in-app purchases |
| App Store Discovery and Engagement Standard | impressions, product page views |
| App Store Installation and Deletion Standard | installs, deletions |
| App Sessions Standard | sessions |
| App Crashes | crashes |

Standard rather than Detailed: the same numbers, split less finely, so fewer lines fall under the
threshold below which Apple leaves a count out. Apple drops counts of fewer than five users or
devices and adds a little noise to detailed ones, so small numbers here are near Apple's own
rather than equal to them.

Only measures that can be summed are kept. Unique counts and paying users cannot be, across days
or across any split, and are not.

Names and values are compared without regard to case, as Apple says they should be, and columns
are found by name, so a report that adds or moves one reads the same.

## Proceeds are Apple's, in dollars

The purchases report's "Proceeds in USD": the customer's price less taxes and Apple's
commission, converted by Apple. Refunds are negative purchases with negative proceeds, and sums
take them back. Stored in millionths of a dollar, so a sum is exact.

## An in-app purchase is what the purchases report calls content

A purchases line says what was bought, by its Content Apple Identifier and Content Name, and
whether it was the app itself ("App purchase") or something inside it ("In-app purchases"). Only
the latter count as in-app purchases; a paid app's proceeds are its app's.

## A later instance replaces the days an earlier one said

An instance is a report as Apple produced it on one processing day. One processed later corrects
earlier ones, and a snapshot repeats days the ongoing request reports too. So an app's instances
are read oldest first, and each replaces the days it covers, for the measures its report yields,
rather than adding to them.

Apple deletes instances after 35 days, and an hourly sync reads each long before then.

## A measure missing from a day is not zero

The reports of a day arrive separately. A day without a measure is drawn as nothing, a line
breaks across it, and a total says how many days it covers. A range asked for without an end
ends on the last day downloads were measured, so it is not a run of whole days ending in one
Apple has said only part of.

## A webhook is mailed only when it is proven to be Apple's

The webhook address is a secret, but one that leaks is enough to send anything to it, so a
delivery is checked before it is mailed. Every delivery is kept either way, with what the
check found, and shown on the dashboard.

An [App Store Server Notification](https://developer.apple.com/documentation/appstoreservernotifications)
proves itself. Its `signedPayload` is a JWS whose header carries three certificates; it is
proven when, as Apple's own libraries check without going online:

- it is signed with ES256, by the first certificate's key;
- the last certificate is Apple Root CA - G3, by its SHA-256,
  `63343abfb89a6a03ebb57e9b3f5fa7be7c4f5c756f3017b3a8c488c3653e9179`, the fingerprint of
  <https://www.apple.com/certificateauthority/AppleRootCA-G3.cer>;
- the leaf carries Apple's mark `1.2.840.113635.100.6.11.1`, and the intermediate
  `1.2.840.113635.100.6.2.1`;
- the chain verifies at the `signedDate` it states, so a notification sent again after a
  certificate lapsed still checks out;
- the `signedTransactionInfo` and `signedRenewalInfo` inside pass the same checks.

An [App Store Connect webhook](https://developer.apple.com/documentation/appstoreconnectapi/webhook-notifications)
is signed with the secret typed in when it was made: its `x-apple-signature` header is
`hmacsha256=` and the hex of an HMAC-SHA256 of the body keyed with that secret. Each address made
under Settings, Webhooks, has a secret of its own, shown once when it is added, and a delivery
signed with its address's secret is proven; one unsigned, or signed otherwise, has failed.

A webhook covers one app, and an app can have ten, so there are as many addresses as there are
places in App Store Connect that ask for one, each labelled to say which. Removing one stops it
answering without touching the others. App Store Server Notifications need only an address; the
production and the sandbox one can share it, the environment being in each notification.

The kinds of event offered for mail are those Apple documents, as of November 2026: the
[notification types](https://developer.apple.com/documentation/appstoreservernotifications/notificationtype)
and the [webhook event types](https://developer.apple.com/documentation/appstoreconnectapi/webhookeventtype),
with `webhookPingCreated` as the Test button delivers it. A notification's subtypes arrive as
`TYPE/SUBTYPE`, and choosing a type mails them all. The groups they are offered in, such as
Subscriptions or Asset packs, are this program's, to choose a handful at a time.

## A mailed delivery says what happened before what Apple sent

A delivery is mailed as a letter, not as its payload. The subject is the event as a person names
it, with where a state got to and which app: `Subscribed: initial buy · SeeIt (Sandbox)`,
`App version state changed: Ready for sale · Dots: App Store Connect`. The app's name is the one
the last sync listed for the notification's bundle id. An App Store Connect webhook does not say
which app it is about, so its address's label stands in.

The body opens with labelled lines: the event, the app and environment, then what the payload
says worth reading first. For a notification that is its transaction's product, price (Apple's
thousandths as units), dates, status and auto-renew; for a webhook, each `old…`/`new…` pair as
one before → after line, and when it happened. Then what Apple says the event means, and the
whole payload, its signed parts opened up.

## Testing an address asks Apple, not this program

An address's Test sends nothing itself: it asks Apple to, so what arrives has come the way
everything else will, from Apple's servers, signed. It finds what sends to the address in App
Store Connect, comparing scheme and host in any case and a trailing slash or none:

- each app whose `subscriptionStatusUrl` or `subscriptionStatusUrlForSandbox` is the address
  is asked, through the App Store Server API's
  [test notification](https://developer.apple.com/documentation/appstoreserverapi/request-a-test-notification),
  to send a `TEST` to it;
- each webhook of each app, from `GET /v1/apps/{id}/webhooks`, whose `url` is the address is
  pinged with `POST /v1/webhookPings`, as its Test button in App Store Connect does, and sends
  a `webhookPingCreated`. One turned off is left alone.

The team key reads the apps and the webhooks and asks for the pings. The App Store Server API
takes none of the team's keys, only an In-App Purchase key, signed for the app's bundle id; it is
optional, and without it the notifications are found and said to need one.
