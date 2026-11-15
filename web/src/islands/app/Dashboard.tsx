import type { ReactNode } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { getAnalytics, getApps, getSales, type Ask } from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type { Analytics, App, Metric, Sales } from "@app/api/types";
import { BarList } from "@app/charts/BarList";
import { StatTile } from "@app/charts/StatTile";
import { TimeChart } from "@app/charts/TimeChart";
import { Dummy } from "@app/components/Dummy";
import {
  change,
  count,
  country,
  daysBetween,
  full,
  money,
  moneyAxis,
  moneyFull,
} from "@app/format";
import { Link } from "@app/islands/app/Link";
import { go, paths, ranges } from "@app/islands/app/route";

/**
 * Everything about a range of days, of every app or one: sales first, which are complete and
 * a day old, then what the analytics reports measured, which arrive later and leave out counts
 * too small for Apple to share. One row of filters above both, so every number agrees.
 */
export function Dashboard({ app, days }: { app: string | null; days: number }) {
  const ask: Ask = { days, app: app ?? undefined };
  const apps = useQuery({ queryKey: qk.apps, queryFn: getApps });
  const sales = useQuery({
    queryKey: qk.sales(ask),
    queryFn: ({ signal }) => getSales(ask, signal),
    placeholderData: keepPreviousData,
  });
  const analytics = useQuery({
    queryKey: qk.analytics(ask),
    queryFn: ({ signal }) => getAnalytics(ask, signal),
    placeholderData: keepPreviousData,
  });

  return (
    <main className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-4 pt-4 pb-12">
      <Filters
        app={app}
        days={days}
        apps={apps.data}
        through={sales.data?.to}
      />
      {sales.error ? (
        <p className="text-sm text-accent">{messageOf(sales.error)}</p>
      ) : !sales.data ? (
        <Waiting />
      ) : (
        <div
          className={
            sales.isPlaceholderData ? "opacity-60 transition-opacity" : ""
          }
        >
          <SalesPart sales={sales.data} everyApp={app === null} days={days} />
        </div>
      )}
      {analytics.error ? (
        <p className="text-sm text-accent">{messageOf(analytics.error)}</p>
      ) : !analytics.data ? null : (
        <div
          className={
            analytics.isPlaceholderData ? "opacity-60 transition-opacity" : ""
          }
        >
          <AnalyticsPart analytics={analytics.data} />
        </div>
      )}
      <p className="text-xs text-faint">
        Days are Apple&rsquo;s, in Pacific time. Proceeds are in US dollars,
        other currencies at the latest rates by{" "}
        <a
          href="https://www.exchangerate-api.com"
          className="underline"
          rel="noreferrer"
        >
          Exchange Rate API
        </a>
        , so they are near Apple&rsquo;s payments rather than equal to them.
      </p>
    </main>
  );
}

function Filters({
  app,
  days,
  apps,
  through,
}: {
  app: string | null;
  days: number;
  apps: App[] | undefined;
  through: string | undefined;
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <label className="relative flex">
        <span className="sr-only">App</span>
        <select
          className="h-8 max-w-64 min-w-0 appearance-none truncate rounded-md bg-control pr-8 pl-3 text-sm hover:bg-control-hover focus:ring-1 focus:ring-brand focus:outline-none pointer-coarse:h-10"
          value={app ?? ""}
          onChange={(e) => go(paths.dashboard(e.target.value || null, days))}
        >
          <option value="">All apps</option>
          {apps?.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name || a.id}
            </option>
          ))}
          {app && apps && !apps.some((a) => a.id === app) ? (
            <option value={app}>{app}</option>
          ) : null}
        </select>
        <svg
          viewBox="0 0 16 16"
          className="pointer-events-none absolute top-1/2 right-2.5 size-4 -translate-y-1/2 fill-none stroke-muted"
          strokeWidth={1.5}
          aria-hidden="true"
        >
          <path d="m4.5 6.5 3.5 3.5 3.5-3.5" />
        </svg>
      </label>
      <div
        className="flex rounded-md bg-control p-0.5"
        role="group"
        aria-label="Days"
      >
        {ranges.map((n) => (
          <Link
            key={n}
            href={paths.dashboard(app, n)}
            aria-current={n === days ? "true" : undefined}
            className="inline-flex min-h-7 items-center rounded px-2.5 text-sm text-muted hover:text-fg aria-[current=true]:bg-bg aria-[current=true]:text-fg aria-[current=true]:shadow-sm pointer-coarse:min-h-9"
          >
            {n === 365 ? "1 year" : `${n} days`}
          </Link>
        ))}
      </div>
      {through ? (
        <span className="text-sm text-faint">through {through}</span>
      ) : null}
    </div>
  );
}

function Waiting() {
  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
        {[0, 1, 2, 3, 4].map((i) => (
          <Dummy key={i} className="h-24" />
        ))}
      </div>
      <Dummy className="h-56" />
    </div>
  );
}

/** A change worth showing: only between two runs of days that were both reported in full. */
function against(
  now: number,
  before: number,
  covered: number,
  coveredBefore: number,
  length: number,
) {
  return covered === length && coveredBefore === length
    ? change(now, before)
    : null;
}

function SalesPart({
  sales,
  everyApp,
  days,
}: {
  sales: Sales;
  everyApp: boolean;
  days: number;
}) {
  const range = daysBetween(sales.from, sales.to);
  const byDay = new Map(sales.days.map((d) => [d.day, d]));
  const t = sales.totals;
  const p = sales.previous;
  const vs = (now: number, before: number) =>
    against(now, before, t.days, p.days, range.length);
  const note =
    t.days < range.length
      ? `${t.days} of ${range.length} days reported`
      : undefined;

  if (t.days === 0) {
    return (
      <Part title="Sales">
        <Empty>
          No sales report has been read for these days yet. The first sync reads
          the past year, a day at a time, and takes a few minutes;{" "}
          <Link href={paths.status} className="underline">
            Status
          </Link>{" "}
          says how it is going.
        </Empty>
      </Part>
    );
  }

  return (
    <Part
      title="Sales"
      about="From Apple's daily sales reports: complete, and a day behind."
    >
      <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
        <StatTile
          label="Downloads"
          value={count(t.downloads)}
          change={vs(t.downloads, p.downloads)}
          note={note}
        />
        <StatTile
          label="Proceeds"
          value={money(t.proceeds)}
          change={vs(t.proceeds, p.proceeds)}
        />
        <StatTile
          label="In-app purchases"
          value={count(t.in_app)}
          change={vs(t.in_app, p.in_app)}
        />
        <StatTile
          label="Redownloads"
          value={count(t.redownloads)}
          change={vs(t.redownloads, p.redownloads)}
        />
        <StatTile
          label="Updates"
          value={count(t.updates)}
          change={vs(t.updates, p.updates)}
        />
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <Card title="Downloads">
          <TimeChart
            label="Downloads per day"
            days={range}
            values={range.map((d) => byDay.get(d)?.downloads)}
            format={(n) => `${full(n)} downloads`}
            axis={count}
          />
        </Card>
        <Card title="Proceeds">
          <TimeChart
            label="Proceeds per day, in US dollars"
            days={range}
            values={range.map((d) => byDay.get(d)?.proceeds)}
            format={moneyFull}
            axis={moneyAxis}
            whole={false}
          />
        </Card>
      </div>
      <div className="grid items-start gap-3 md:grid-cols-2">
        <Card title="Countries">
          <BarList
            label="Downloads and proceeds by country"
            columns={[
              { label: "Downloads", format: full },
              { label: "Proceeds", format: moneyFull },
            ]}
            rows={sales.countries.map((c) => ({
              id: c.id,
              name: country(c.id),
              values: [c.downloads, c.proceeds],
            }))}
          />
        </Card>
        {everyApp ? (
          <Card title="Apps">
            <BarList
              label="Proceeds and downloads by app"
              columns={[
                { label: "Proceeds", format: moneyFull },
                { label: "Downloads", format: full },
              ]}
              rows={sales.apps.map((a) => ({
                id: a.id || "unknown",
                name: a.id ? (
                  <Link
                    href={paths.dashboard(a.id, days)}
                    className="hover:underline"
                  >
                    {a.name || a.id}
                  </Link>
                ) : (
                  "An app Apple did not name"
                ),
                values: [a.proceeds, a.downloads],
              }))}
            />
          </Card>
        ) : null}
        {sales.in_app.length > 0 ? (
          <Card title="In-app purchases">
            <BarList
              label="Proceeds and purchases by in-app purchase"
              columns={[
                { label: "Proceeds", format: moneyFull },
                { label: "Bought", format: full },
              ]}
              rows={sales.in_app.map((a) => ({
                id: a.id,
                // Titles like "Premium" repeat from app to app, so with every app shown each
                // says whose it is.
                name:
                  everyApp && a.app_name ? (
                    <>
                      {a.name || a.id}
                      <span className="ml-2 text-faint">{a.app_name}</span>
                    </>
                  ) : (
                    a.name || a.id
                  ),
                values: [a.proceeds, a.units],
              }))}
            />
          </Card>
        ) : null}
      </div>
      {sales.unconverted.length > 0 ? (
        <p className="text-xs text-faint">
          Proceeds in {sales.unconverted.join(", ")} have no exchange rate yet
          and are left out of the sums.
        </p>
      ) : null}
    </Part>
  );
}

const names: Record<Metric, string> = {
  impressions: "Impressions",
  page_views: "Product page views",
  first_downloads: "First-time downloads",
  redownloads: "Redownloads",
  installs: "Installs",
  deletions: "Deletions",
  sessions: "Sessions",
  crashes: "Crashes",
};

/** The measures shown, and whether more of one is good. */
const tiles: [Metric, boolean][] = [
  ["impressions", true],
  ["page_views", true],
  ["first_downloads", true],
  ["sessions", true],
  ["deletions", false],
  ["crashes", false],
];

const charted: Metric[] = ["impressions", "page_views", "sessions", "crashes"];

function AnalyticsPart({ analytics: a }: { analytics: Analytics }) {
  const range = daysBetween(a.from, a.to);
  const byDay = new Map(a.days.map((d) => [d.day, d]));
  const any = Object.keys(a.covered).length > 0;

  if (!any) {
    return (
      <Part title="App Store">
        <Empty>
          Nothing measured for these days yet. Apple makes the first analytics
          reports a day or two after they are asked for, which the first sync
          does;{" "}
          <Link href={paths.status} className="underline">
            Status
          </Link>{" "}
          says whether it could.
        </Empty>
      </Part>
    );
  }

  return (
    <Part
      title="App Store"
      about="From Apple's analytics reports: a day or two behind sales, and without counts too small for Apple to share."
    >
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
        {tiles.map(([m, upIsGood]) => {
          const covered = a.covered[m] ?? 0;
          return (
            <StatTile
              key={m}
              label={names[m]}
              value={covered ? count(a.totals[m] ?? 0) : "—"}
              change={against(
                a.totals[m] ?? 0,
                a.previous[m] ?? 0,
                covered,
                a.previous_covered[m] ?? 0,
                range.length,
              )}
              upIsGood={upIsGood}
              note={
                covered < range.length
                  ? `${covered} of ${range.length} days`
                  : undefined
              }
            />
          );
        })}
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        {charted.map((m) => (
          <Card key={m} title={names[m]}>
            <TimeChart
              label={`${names[m]} per day`}
              days={range}
              values={range.map((d) => byDay.get(d)?.[m])}
              format={(n) => `${full(n)} ${names[m].toLowerCase()}`}
              axis={count}
              height={140}
            />
          </Card>
        ))}
      </div>
      {a.sources.length > 0 ? (
        <Card title="Where visitors came from">
          <BarList
            label="Product page views, impressions and first-time downloads by source"
            columns={[
              { label: "Page views", format: full },
              { label: "Impressions", format: full },
              { label: "First-time downloads", format: full },
            ]}
            rows={a.sources.map((s) => ({
              id: s.source,
              name: s.source,
              values: [
                s.page_views ?? 0,
                s.impressions ?? 0,
                s.first_downloads ?? 0,
              ],
            }))}
          />
        </Card>
      ) : null}
    </Part>
  );
}

function Part({
  title,
  about,
  children,
}: {
  title: string;
  about?: string;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline gap-x-3">
        <h2 className="text-lg font-semibold">{title}</h2>
        {about ? <p className="text-sm text-faint">{about}</p> : null}
      </div>
      {children}
    </section>
  );
}

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex min-w-0 flex-col gap-3 rounded-lg border border-line bg-bg p-4">
      <h3 className="text-sm font-medium">{title}</h3>
      {children}
    </section>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-lg border border-dashed border-line px-4 py-6 text-sm text-muted">
      {children}
    </p>
  );
}
