import type { ReactNode } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { getApps, getReport, getStatus, type Ask } from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type { App, Metric, Report } from "@app/api/types";
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
import { pending } from "@app/islands/app/pending";
import { go, paths, ranges } from "@app/islands/app/route";

/**
 * Everything about a range of days, of every app or one: what was downloaded and earned, then
 * how the App Store pages did. One row of filters above both, so every number agrees.
 */
export function Dashboard({ app, days }: { app: string | null; days: number }) {
  const ask: Ask = { days, app: app ?? undefined };
  const apps = useQuery({ queryKey: qk.apps, queryFn: getApps });
  const report = useQuery({
    queryKey: qk.report(ask),
    queryFn: ({ signal }) => getReport(ask, signal),
    placeholderData: keepPreviousData,
  });
  const r = report.data;

  return (
    <main className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-4 pt-4 pb-12">
      <Filters app={app} days={days} apps={apps.data} through={r?.to} />
      {report.error ? (
        <p className="text-sm text-accent">{messageOf(report.error)}</p>
      ) : !r ? (
        <Waiting />
      ) : Object.keys(r.covered).length === 0 ? (
        <Nothing />
      ) : (
        <div
          className={`flex flex-col gap-8 ${report.isPlaceholderData ? "opacity-60 transition-opacity" : ""}`}
        >
          <Sales report={r} everyApp={app === null} days={days} />
          <Store report={r} />
        </div>
      )}
      <Sources />
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

const names: Record<Metric, string> = {
  first_downloads: "Downloads",
  proceeds: "Proceeds",
  purchases: "In-app purchases",
  redownloads: "Redownloads",
  updates: "Updates",
  impressions: "Impressions",
  page_views: "Product page views",
  installs: "Installs",
  deletions: "Deletions",
  sessions: "Sessions",
  crashes: "Crashes",
};

/** A headline number for each measure, compared only between two runs of days both measured in full. */
function Tiles({
  report: r,
  shown,
  className,
}: {
  report: Report;
  shown: [Metric, boolean][];
  className: string;
}) {
  const length = daysBetween(r.from, r.to).length;
  return (
    <div className={`grid gap-3 ${className}`}>
      {shown.map(([m, upIsGood]) => {
        const covered = r.covered[m] ?? 0;
        const total = r.totals[m] ?? 0;
        const whole =
          covered === length && (r.previous_covered[m] ?? 0) === length;
        return (
          <StatTile
            key={m}
            label={names[m]}
            value={
              covered ? (m === "proceeds" ? money(total) : count(total)) : "—"
            }
            change={whole ? change(total, r.previous[m] ?? 0) : null}
            upIsGood={upIsGood}
            note={covered < length ? `${covered} of ${length} days` : undefined}
          />
        );
      })}
    </div>
  );
}

function Sales({
  report: r,
  everyApp,
  days,
}: {
  report: Report;
  everyApp: boolean;
  days: number;
}) {
  const range = daysBetween(r.from, r.to);
  const byDay = new Map(r.days.map((d) => [d.day, d]));
  return (
    <Part title="Downloads and proceeds">
      <Tiles
        report={r}
        className="grid-cols-2 md:grid-cols-5"
        shown={[
          ["first_downloads", true],
          ["proceeds", true],
          ["purchases", true],
          ["redownloads", true],
          ["updates", true],
        ]}
      />
      <div className="grid gap-3 md:grid-cols-2">
        <Card title="Downloads">
          <TimeChart
            label="First-time downloads per day"
            days={range}
            values={range.map((d) => byDay.get(d)?.first_downloads)}
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
            rows={r.countries.map((c) => ({
              id: c.id,
              name: country(c.id),
              values: [c.first_downloads ?? 0, c.proceeds ?? 0],
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
              rows={r.apps.map((a) => ({
                id: a.id,
                name: (
                  <Link
                    href={paths.dashboard(a.id, days)}
                    className="hover:underline"
                  >
                    {a.name || a.id}
                  </Link>
                ),
                values: [a.proceeds ?? 0, a.first_downloads ?? 0],
              }))}
            />
          </Card>
        ) : null}
        {r.in_app.length > 0 ? (
          <Card title="In-app purchases">
            <BarList
              label="Proceeds and purchases by in-app purchase"
              columns={[
                { label: "Proceeds", format: moneyFull },
                { label: "Bought", format: full },
              ]}
              rows={r.in_app.map((a) => ({
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
                values: [a.proceeds ?? 0, a.purchases ?? 0],
              }))}
            />
          </Card>
        ) : null}
      </div>
    </Part>
  );
}

const charted: Metric[] = ["impressions", "page_views", "sessions", "crashes"];

function Store({ report: r }: { report: Report }) {
  const range = daysBetween(r.from, r.to);
  const byDay = new Map(r.days.map((d) => [d.day, d]));
  return (
    <Part title="App Store">
      <Tiles
        report={r}
        className="grid-cols-2 md:grid-cols-3 lg:grid-cols-6"
        shown={[
          ["impressions", true],
          ["page_views", true],
          ["installs", true],
          ["sessions", true],
          ["deletions", false],
          ["crashes", false],
        ]}
      />
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
      {r.sources.length > 0 ? (
        <Card title="Where visitors came from">
          <BarList
            label="Product page views, impressions and downloads by source"
            columns={[
              { label: "Page views", format: full },
              { label: "Impressions", format: full },
              { label: "Downloads", format: full },
            ]}
            rows={r.sources.map((s) => ({
              id: s.id,
              name: s.name,
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

function Part({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-lg font-semibold">{title}</h2>
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

/** Where the numbers come from, which depends on whether sales reports fill in. */
function Sources() {
  const status = useQuery({ queryKey: qk.status, queryFn: getStatus });
  return (
    <p className="text-xs text-faint">
      From Apple&rsquo;s analytics reports, a day or two behind, without counts
      too small for Apple to share
      {status.data?.sales.on ? (
        <>
          , and from its sales reports for downloads, purchases and proceeds on
          the days analytics reports have not reached yet, their proceeds
          converted at the latest rates by{" "}
          <a
            href="https://www.exchangerate-api.com"
            className="underline"
            rel="noreferrer"
          >
            Exchange Rate API
          </a>
        </>
      ) : null}
      . Days are Apple&rsquo;s, in Pacific time. Proceeds are in US dollars,
      after Apple&rsquo;s commission and taxes.
    </p>
  );
}

/** No numbers for the days asked: why, as the sync last found it. */
function Nothing() {
  const status = useQuery({ queryKey: qk.status, queryFn: getStatus });
  const why = pending(status.data);
  return (
    <Empty>
      {why ?? "Nothing measured for these days."}{" "}
      <Link href={paths.status} className="underline">
        Status
      </Link>
    </Empty>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-lg border border-dashed border-line px-4 py-6 text-sm text-muted">
      {children}
    </p>
  );
}
