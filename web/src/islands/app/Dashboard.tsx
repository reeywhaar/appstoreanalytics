import { useState, type ReactNode } from "react";
import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
} from "@tanstack/react-query";

import {
  getApps,
  getHistory,
  getReport,
  getStatus,
  type Ask,
} from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type { App, ByApp, Day, History, Metric, Report } from "@app/api/types";
import { BarList } from "@app/charts/BarList";
import { Legend } from "@app/charts/Legend";
import { appColors, type ChartMode, type Series } from "@app/charts/series";
import { StatTile } from "@app/charts/StatTile";
import { TimeChart } from "@app/charts/TimeChart";
import { Dummy } from "@app/components/Dummy";
import {
  change,
  count,
  counted,
  country,
  daysBetween,
  full,
  money,
  moneyAxis,
  moneyFull,
} from "@app/format";
import { Deliveries } from "@app/islands/app/Deliveries";
import { Link } from "@app/islands/app/Link";
import { pending } from "@app/islands/app/pending";
import { go, paths, ranges } from "@app/islands/app/route";

/**
 * Everything about a range of days, of every app or one: what was downloaded and earned, then
 * how the App Store pages did. One row of filters above both, so every number agrees. Then what
 * Apple's webhooks have delivered, which no filter narrows.
 */
export function Dashboard({ app, days }: { app: string | null; days: number }) {
  const ask: Ask = { days, app: app ?? undefined };
  const apps = useQuery({ queryKey: qk.apps, queryFn: getApps });
  const report = useQuery({
    queryKey: qk.report(ask),
    queryFn: ({ signal }) => getReport(ask, signal),
    placeholderData: keepPreviousData,
  });
  // A year at a time, the latest first, and each year before it as a chart is scrolled back
  // to its start.
  const history = useInfiniteQuery({
    queryKey: qk.history(app ?? ""),
    queryFn: ({ pageParam, signal }) =>
      getHistory(app ?? undefined, pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: () => undefined,
    getPreviousPageParam: (earliest) =>
      earliest.from > earliest.first ? earliest.from : undefined,
    placeholderData: keepPreviousData,
  });
  const { hasPreviousPage, isFetchingPreviousPage, fetchPreviousPage } =
    history;
  const earlier = () => {
    if (hasPreviousPage && !isFetchingPreviousPage) void fetchPreviousPage();
  };
  const r = report.data;
  const [mode, setMode] = useChartMode();
  const colors = appColors((apps.data ?? []).map((a) => a.id));
  const line = r
    ? timeline(r, joined(history.data?.pages), {
        app,
        apps: apps.data ?? [],
        colors,
        mode,
        earlier,
      })
    : null;

  return (
    <main className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-4 pt-4 pb-12">
      <Filters
        app={app}
        days={days}
        apps={apps.data}
        through={r?.to}
        mode={mode}
        onMode={setMode}
      />
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
          <Sales report={r} everyApp={app === null} days={days} line={line!} />
          <Store report={r} days={days} line={line!} />
        </div>
      )}
      <Sources />
      <Part title="Webhooks">
        <Deliveries />
      </Part>
    </main>
  );
}

function Filters({
  app,
  days,
  apps,
  through,
  mode,
  onMode,
}: {
  app: string | null;
  days: number;
  apps: App[] | undefined;
  through: string | undefined;
  mode: ChartMode;
  onMode: (m: ChartMode) => void;
}) {
  // Stays at the top as the page scrolls, over what passes under it: the range and the app are
  // what every number below is of. The days end where it says, beside the days, and how the
  // charts are drawn is apart, at the end, so "through" is not read as the lines'.
  //
  // On a phone, two rows rather than three: the app and how the charts are drawn, then the days,
  // shortened, and where they end. Kept at the top, a third row would be a third of the screen.
  return (
    <div className="sticky top-0 z-20 -mx-4 flex flex-wrap items-center gap-x-3 gap-y-2 bg-page/90 px-4 py-2 backdrop-blur">
      <div className="flex w-full items-center gap-3 sm:w-auto">
        <label className="relative flex min-w-0 flex-1 sm:flex-none">
          <span className="sr-only">App</span>
          <select
            className="h-8 w-full min-w-0 appearance-none truncate rounded-md bg-control pr-8 pl-3 text-sm hover:bg-control-hover focus:ring-1 focus:ring-brand focus:outline-none sm:w-auto sm:max-w-64 pointer-coarse:h-10"
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
        <Modes mode={mode} onMode={onMode} compact className="sm:hidden" />
      </div>
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
            <span className="sm:hidden">{n === 365 ? "1y" : `${n}d`}</span>
            <span className="hidden sm:inline">
              {n === 365 ? "1 year" : `${n} days`}
            </span>
          </Link>
        ))}
      </div>
      {through ? (
        <span className="text-sm text-faint">through {through}</span>
      ) : null}
      <Modes mode={mode} onMode={onMode} className="ml-auto hidden sm:flex" />
    </div>
  );
}

/**
 * Columns or lines, as two words, or as two pictures where the row is a phone's: a word each
 * would push the app's name off its row.
 */
function Modes({
  mode,
  onMode,
  compact = false,
  className = "",
}: {
  mode: ChartMode;
  onMode: (m: ChartMode) => void;
  compact?: boolean;
  className?: string;
}) {
  return (
    <div
      className={`flex shrink-0 rounded-md bg-control p-0.5 ${className}`}
      role="group"
      aria-label="Charts as"
    >
      {(["bars", "lines"] as const).map((m) => (
        <button
          key={m}
          type="button"
          aria-pressed={m === mode}
          aria-label={compact ? (m === "bars" ? "Bars" : "Lines") : undefined}
          title={compact ? (m === "bars" ? "Bars" : "Lines") : undefined}
          onClick={() => onMode(m)}
          className="inline-flex min-h-7 items-center rounded px-2.5 text-sm text-muted hover:text-fg aria-pressed:bg-bg aria-pressed:text-fg aria-pressed:shadow-sm pointer-coarse:min-h-9"
        >
          {!compact ? (
            m === "bars" ? (
              "Bars"
            ) : (
              "Lines"
            )
          ) : m === "bars" ? (
            <svg
              viewBox="0 0 16 16"
              className="size-4 fill-current"
              aria-hidden="true"
            >
              <rect x="2" y="7" width="3" height="7" rx="1" />
              <rect x="6.5" y="3" width="3" height="11" rx="1" />
              <rect x="11" y="9" width="3" height="5" rx="1" />
            </svg>
          ) : (
            <svg
              viewBox="0 0 16 16"
              className="size-4 fill-none stroke-current"
              strokeWidth={1.75}
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <path d="M2 12 6 7l3 3 5-6" />
            </svg>
          )}
        </button>
      ))}
    </div>
  );
}

/**
 * Whether the charts are columns or lines: this browser's choice, kept for its next visit. Kept
 * where a private window or blocked storage may refuse it, and nothing depends on it surviving.
 */
function useChartMode(): [ChartMode, (m: ChartMode) => void] {
  const [mode, setMode] = useState<ChartMode>(() => {
    try {
      return localStorage.getItem("chart-mode") === "lines" ? "lines" : "bars";
    } catch {
      return "bars";
    }
  });
  return [
    mode,
    (m) => {
      setMode(m);
      try {
        localStorage.setItem("chart-mode", m);
      } catch {
        // Kept for this visit only.
      }
    },
  ];
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

/**
 * The days a chart scrolls through, what each measured, and how to draw it: each app apart in
 * its own colour when every app is shown, and the one app's colour when one is.
 */
type Line = {
  days: string[];
  byDay: Map<string, Day>;
  byApp: ByApp;
  app: string | null;
  apps: App[];
  colors: Map<string, string>;
  mode: ChartMode;
  /** Asks for the year before the earliest day there is so far. */
  earlier: () => void;
};

/** The years of the history read so far, as one: the earliest first, as they are kept. */
function joined(pages: History[] | undefined): History | undefined {
  if (!pages?.length) return undefined;
  const byApp: ByApp = {};
  for (const page of pages) {
    for (const [app, days] of Object.entries(page.by_app ?? {})) {
      byApp[app] = { ...byApp[app], ...days };
    }
  }
  return {
    ...pages[pages.length - 1]!,
    from: pages[0]!.from,
    days: pages.flatMap((p) => p.days),
    by_app: byApp,
  };
}

/**
 * Every day there is, once the history has come, and the range's own days until then. The
 * history ends where the range does, so the chart opens on the same latest day either way.
 */
function timeline(
  r: Report,
  h: History | undefined,
  how: Pick<Line, "app" | "apps" | "colors" | "mode" | "earlier">,
): Line {
  const source = h && h.to === r.to ? h : r;
  return {
    ...how,
    days: daysBetween(source === h ? h.from : r.from, r.to),
    byDay: new Map(source.days.map((d) => [d.day, d])),
    byApp: source.by_app ?? {},
  };
}

/**
 * Each app's own values of a measure, in the order their colours were given, which is the order
 * they were made: only those with any of it, and nothing on a day the totals do not know yet.
 */
function appSeries(
  line: Line,
  m: Metric,
  totals: (number | undefined)[],
): Series[] {
  const called = new Map(line.apps.map((a) => [a.id, a.name || a.id]));
  return [...line.colors.keys()].flatMap((id) => {
    const own = line.byApp[id];
    if (!own) return [];
    const values = line.days.map((d, i) =>
      totals[i] === undefined ? undefined : (own[d]?.[m] ?? 0),
    );
    if (!values.some((v) => v)) return [];
    return [
      { id, name: called.get(id) ?? id, color: line.colors.get(id)!, values },
    ];
  });
}

/** What a chart of one measure draws: its days and values, and each app apart or the one app. */
function plot(line: Line, m: Metric) {
  const values = line.days.map((d) => line.byDay.get(d)?.[m]);
  return {
    days: line.days,
    values,
    mode: line.mode,
    onEarlier: line.earlier,
    series: line.app === null ? appSeries(line, m, values) : undefined,
    color: line.app === null ? undefined : line.colors.get(line.app),
  };
}

/**
 * Who is who in a part's charts: every app any of them tells apart, once, in their order. None
 * for one app, or for every app when only one has anything to show.
 */
function Apps({ line, measures }: { line: Line; measures: Metric[] }) {
  if (line.app !== null) return null;
  const shown = new Map<string, Series>();
  for (const m of measures) {
    for (const s of plot(line, m).series ?? []) shown.set(s.id, s);
  }
  const entries = [...line.colors.keys()].flatMap((id) => {
    const s = shown.get(id);
    return s ? [{ id, name: s.name, color: s.color }] : [];
  });
  if (entries.length < 2) return null;
  return <Legend entries={entries} />;
}

function Sales({
  report: r,
  everyApp,
  days,
  line,
}: {
  report: Report;
  everyApp: boolean;
  days: number;
  line: Line;
}) {
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
      <Apps line={line} measures={["first_downloads", "proceeds", "updates"]} />
      <div className="grid gap-3 md:grid-cols-3">
        <Card title="Downloads">
          <TimeChart
            window={days}
            label="First-time downloads per day"
            {...plot(line, "first_downloads")}
            format={(n) => counted(n, "download", "downloads")}
            cell={full}
            axis={count}
          />
        </Card>
        <Card title="Proceeds">
          <TimeChart
            window={days}
            label="Proceeds per day, in US dollars"
            {...plot(line, "proceeds")}
            format={moneyFull}
            axis={moneyAxis}
            whole={false}
          />
        </Card>
        <Card title="Updates">
          <TimeChart
            window={days}
            label="Updates per day"
            {...plot(line, "updates")}
            format={(n) => counted(n, "update", "updates")}
            cell={full}
            axis={count}
          />
        </Card>
      </div>
      <div className="grid items-start gap-3 md:grid-cols-2">
        <Card title="Countries">
          <BarList
            label="Downloads and proceeds by country"
            named="Country"
            columns={[
              { label: "Downloads", format: full },
              { label: "Proceeds", format: moneyFull },
            ]}
            rows={r.countries.map((c) => ({
              id: c.id,
              name: country(c.id),
              text: country(c.id),
              values: [c.first_downloads ?? 0, c.proceeds ?? 0],
            }))}
          />
        </Card>
        {everyApp ? (
          <Card title="Apps">
            <BarList
              label="Proceeds and downloads by app"
              named="App"
              columns={[
                { label: "Proceeds", format: moneyFull },
                { label: "Downloads", format: full },
              ]}
              rows={r.apps.map((a) => ({
                id: a.id,
                text: a.name || a.id,
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
              named="In-app purchase"
              columns={[
                { label: "Proceeds", format: moneyFull },
                { label: "Bought", format: full },
              ]}
              rows={r.in_app.map((a) => ({
                id: a.id,
                text: a.name || a.id,
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

/** What one of each charted measure is called. */
const one: Partial<Record<Metric, string>> = {
  impressions: "impression",
  page_views: "product page view",
  sessions: "session",
  crashes: "crash",
};

function Store({
  report: r,
  days,
  line,
}: {
  report: Report;
  days: number;
  line: Line;
}) {
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
      <Apps line={line} measures={charted} />
      <div className="grid gap-3 md:grid-cols-2">
        {charted.map((m) => (
          <Card key={m} title={names[m]}>
            <TimeChart
              window={days}
              label={`${names[m]} per day`}
              {...plot(line, m)}
              format={(n) => counted(n, one[m] ?? "", names[m].toLowerCase())}
              cell={full}
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
            named="Source"
            columns={[
              { label: "Page views", format: full },
              { label: "Impressions", format: full },
              { label: "Downloads", format: full },
            ]}
            rows={r.sources.map((s) => ({
              id: s.id,
              name: s.name,
              text: s.name,
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
