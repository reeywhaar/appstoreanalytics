# App Store Connect

What is read from Apple, how, and what it means.

## One team key, made in App Store Connect

**Users and Access → Integrations → App Store Connect API → Team Keys.** The page shows the
issuer id above the list; a key's id is in its row, and its `.p8` downloads once.

The key goes into the environment as base64, so it fits on one line:

```sh
base64 -i AuthKey_XXXXXXXXXX.p8 | tr -d '\n'
```

A team key, because sales reports take one; an individual key is refused by them.

| role | sales | analytics |
| --- | --- | --- |
| Admin | read | asked for and read |
| Finance, or Sales and Reports | read | read, once an Admin key has asked for them |

The vendor number is shown at the top of **Payments and Financial Reports**. Sales reports are
asked for by it.

## The only thing written to Apple is a request for analytics reports

Everything else is read. Requests are made per app, once each, and are listed in App Store
Connect's own analytics settings.

## A sync runs every hour, and on demand

A pass reads the apps, then sales, then analytics, and keeps how each part went for the status
page. Each part goes on from where the last stopped, so a pass cut short loses nothing but time.

**A refusal of the key stops the part at once**, since every other request would be refused the
same way. **Apple asking to slow down (`429`) stops it too**, and the next pass carries on.

## Sales

### A day's report is the daily sales summary

`GET /v1/salesReports`, `DAILY`, `SALES`, `SUMMARY`, version `1_0`: a gzipped, tab-separated
file with a line per product, country, device and price. Columns are read by name, so a version
that adds or moves one reads the same. Lines are summed over device and price, which are not
kept.

### A year is read, newest day first

Apple keeps daily reports for 365 days. The first sync reads all of them, one request a day,
newest first, so the days somebody looks at first are there first. That takes a few minutes.

### A missing report is one of three things, and Apple's words tell which

- `404` "There were no sales for the date specified." — a day with none, stored as known and
  empty.
- `404` "Report is not available yet…" — asked again next pass. After five days it is taken as
  having had none, so it is not asked for every hour forever.
- `410` — older than Apple keeps; nothing before it is asked for.

### A day is read again until it settles

The territories' reports arrive at different hours of the next morning. A day read within 36
hours of its end is read again each pass, until a read comes after that.

### What a line counts is its product type

| product type | counted as |
| --- | --- |
| `1`, `1F`, `1T`, `F1`, `1E`, `1EP`, `1EU`, `1-B`, `F1-B` | download |
| `3`, `3F`, `3T`, `F3` | redownload |
| `7`, `7F`, `7T`, `F7` | update |
| `IA…`, `FI…` | in-app purchase, subscriptions included |

Units of a refund are negative, and so are its proceeds; sums take them back.

### An in-app purchase belongs to the app whose SKU it names

Its line's Parent Identifier is the app's SKU, resolved through the apps list and through the
app's own lines.

### Proceeds are summed in US dollars at the latest rates

A report has proceeds in a dozen currencies and no rates. Rates come from
[Exchange Rate API](https://www.exchangerate-api.com)'s free `open.er-api.com/v6/latest/USD`,
read once a day: it covers every currency Apple pays in and asks for no key. Nothing about the
account is sent with the request.

Today's rate for a sale a year old is near what Apple paid, not equal to it; the interface says
so. Proceeds are stored in their own currency, so a better rate later changes every sum.

## Analytics

### Each app gets an ongoing request and a snapshot

An **ongoing** request makes reports every day from the day it is asked for. A **snapshot**
makes one report of the history there is, back to 2024. Both are asked for on the first sync;
the first reports come a day or two later. Apple ends an ongoing request nobody reads, and a
sync asks again.

### Five Standard reports are read

| report | measures |
| --- | --- |
| App Store Discovery and Engagement Standard | impressions, product page views |
| App Downloads Standard | first-time downloads, redownloads |
| App Store Installation and Deletion Standard | installs, deletions |
| App Sessions Standard | sessions |
| App Crashes | crashes |

Standard rather than Detailed: the same numbers, split less finely, so fewer lines fall under
the threshold below which Apple leaves a count out. Counts that small are missing from these
totals, which is why analytics downloads are a little below the sales report's.

Only measures that can be summed are kept. Unique counts cannot be, across days or across any
split, and are not.

### A later instance replaces the days an earlier one said

An instance is a report as Apple produced it on one processing day. One processed later corrects
earlier ones, and a snapshot repeats days the ongoing request also reports. So instances are read
oldest first, and each replaces the days it covers rather than adding to them.

Apple deletes instances after 35 days, and an hourly sync reads each long before then.

### A measure missing from a day is not zero

The reports of a day arrive separately. A day without a measure is drawn as nothing, a line
breaks across it, and a total says how many days it covers.
