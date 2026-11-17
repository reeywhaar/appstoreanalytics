import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getStatus, postSync } from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type { AppRequests, Part, Status as Synced } from "@app/api/types";
import { Button } from "@app/components/Button";
import { Dummy } from "@app/components/Dummy";
import { ago, moment } from "@app/format";
import { pending } from "@app/islands/app/pending";

/**
 * What has been read from Apple, when, and what went wrong: the page somebody opens when a
 * number looks missing.
 */
export function Status() {
  const client = useQueryClient();
  const status = useQuery({ queryKey: qk.status, queryFn: getStatus });
  const sync = useMutation({
    mutationFn: postSync,
    onSuccess: () => client.invalidateQueries({ queryKey: qk.status }),
  });
  const s = status.data;

  return (
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-4 px-2 pt-2 pb-6 sm:gap-6 sm:px-4 sm:pt-4 sm:pb-12">
      {/* Beside the title where there is room, and under it where there is not. */}
      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold">Status</h1>
          <span className="hidden flex-1 sm:block">
            {s ? <LastSync status={s} /> : null}
          </span>
          <span className="flex-1 sm:hidden" />
          <Button
            variant="solid"
            size="bar"
            disabled={!s || s.running || sync.isPending}
            onClick={() => sync.mutate()}
          >
            {s?.running ? "Syncing…" : "Sync now"}
          </Button>
        </div>
        <span className="sm:hidden">{s ? <LastSync status={s} /> : null}</span>
      </div>
      {sync.error ? (
        <p className="text-sm text-accent">{messageOf(sync.error)}</p>
      ) : null}
      <p className="text-sm text-muted">
        A sync runs every hour on its own. It reads the analytics reports Apple
        has made since the last one, and on the first asks Apple to make them,
        with history back to 2024.
      </p>
      {status.error ? (
        <p className="text-sm text-accent">{messageOf(status.error)}</p>
      ) : !s ? (
        <Dummy className="h-64" />
      ) : (
        <dl className="flex flex-col divide-y divide-line rounded-lg border border-line bg-bg">
          <Row title="Apps" part={s.apps.part} now={s.reading === "apps"}>
            {s.apps.count === 1 ? "1 app" : `${s.apps.count} apps`}
          </Row>
          <Row
            title="Analytics reports"
            part={s.analytics.part}
            now={s.reading === "analytics"}
          >
            {s.analytics.instances
              ? `${s.analytics.instances} ${s.analytics.instances === 1 ? "report" : "reports"} read, measuring ${s.analytics.first} to ${s.analytics.last}`
              : pending(s)}
          </Row>
          {s.sales.on ? (
            <>
              <Row
                title="Sales reports"
                part={s.sales.part}
                now={s.reading === "sales"}
              >
                {s.sales.days
                  ? `${s.sales.days} days read, ${s.sales.first} to ${s.sales.last}. They fill in downloads, purchases and proceeds for the days analytics reports have not reached yet.`
                  : "None read yet. They fill in downloads, purchases and proceeds for the days analytics reports have not reached yet."}
              </Row>
              <Row
                title="Exchange rates"
                part={s.rates.part}
                now={s.reading === "rates"}
              >
                {s.rates.at
                  ? `Read ${ago(s.rates.at)}, for sales reports' proceeds in other currencies`
                  : "None read yet"}
              </Row>
            </>
          ) : null}
        </dl>
      )}
      {s && s.requests.length > 0 ? <Requests requests={s.requests} /> : null}
    </main>
  );
}

/** What each part of a sync is called. */
const partNames: Record<Synced["reading"], string> = {
  "": "",
  apps: "apps",
  analytics: "analytics reports",
  sales: "sales reports",
  rates: "exchange rates",
};

/**
 * The last sync in a line: when it ran, and whether everything went through, or that one is
 * running now and which part of it.
 */
function LastSync({ status: s }: { status: Synced }) {
  if (s.running) {
    return (
      <span className="inline-flex items-center gap-1.5 text-sm text-muted">
        <span className="size-2 animate-pulse rounded-full bg-series" />
        Syncing now{s.reading ? `: ${partNames[s.reading]}` : ""}
      </span>
    );
  }
  const parts: [string, Part][] = [
    ["apps", s.apps.part],
    ["analytics", s.analytics.part],
    ...(s.sales.on
      ? ([
          ["sales", s.sales.part],
          ["rates", s.rates.part],
        ] as [string, Part][])
      : []),
  ];
  const last = Math.max(0, ...parts.map(([, p]) => p.ran_at ?? 0));
  if (last === 0) {
    return <span className="text-sm text-muted">Not synced yet</span>;
  }
  const failed = parts
    .filter(([, p]) => p.error)
    .map(([name]) => partNames[name as Synced["reading"]]);
  return (
    <span className="text-sm text-muted">
      Last sync {moment(last)}, {ago(last)} ·{" "}
      {failed.length === 0 ? (
        <span className="text-up">✓ everything went through</span>
      ) : (
        <span className="text-down">
          ! {failed.join(", ")} did not go through
        </span>
      )}
    </span>
  );
}

/**
 * Each app's analytics reports with Apple: what was asked for, since when, and what Apple has
 * delivered. Apple has no "pending": a request with nothing delivered is one Apple is preparing.
 */
function Requests({ requests }: { requests: AppRequests[] }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="font-medium">Analytics reports by app</h2>
      <div className="overflow-x-auto rounded-lg border border-line bg-bg">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-line text-left text-xs text-faint">
              <th className="px-4 py-2 font-normal">App</th>
              <th className="px-4 py-2 font-normal">Ongoing (from now on)</th>
              <th className="px-4 py-2 font-normal">Snapshot (back to 2024)</th>
              <th className="px-4 py-2 font-normal">Asked</th>
              <th className="px-4 py-2 font-normal">Reports delivered</th>
            </tr>
          </thead>
          <tbody>
            {requests.map((r) => (
              <tr key={r.app_id} className="border-b border-line last:border-0">
                <td className="px-4 py-2 font-medium">{r.name}</td>
                <td className="px-4 py-2 whitespace-nowrap">
                  {r.ongoing === "running" ? (
                    "Asked"
                  ) : r.ongoing === "stopped" ? (
                    <span className="text-down">Stopped by Apple</span>
                  ) : (
                    <span className="text-faint">Not asked</span>
                  )}
                </td>
                <td className="px-4 py-2 whitespace-nowrap">
                  {r.snapshot ? (
                    "Asked"
                  ) : (
                    <span className="text-faint">Not asked</span>
                  )}
                </td>
                <td className="px-4 py-2 whitespace-nowrap text-muted tabular-nums">
                  {moment(r.since)}
                </td>
                <td className="px-4 py-2">
                  {r.delivered > 0 ? (
                    <span className="tabular-nums">
                      {r.delivered}, latest {r.latest}
                    </span>
                  ) : (
                    <span className="text-faint">Not yet</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-faint">
        An ongoing request makes a report every day from when it is asked for; a
        snapshot makes one, once, of the history back to January 2024. Apple
        delivers the first within one to two days and says nothing of a request
        in between. It stops an ongoing request nobody downloads, and the next
        sync asks again.
      </p>
    </section>
  );
}

/**
 * One part of the sync: what it has read, and how it last went — or, while it is being read,
 * that it is, since how it went last time is no longer the news.
 */
function Row({
  title,
  part,
  now,
  children,
}: {
  title: string;
  part: Part;
  now: boolean;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1 px-4 py-3">
      <dt className="flex flex-wrap items-baseline gap-x-2">
        <span className="font-medium">{title}</span>
        <span className="inline-flex items-center gap-1.5 text-xs text-faint">
          {now ? (
            <>
              <span className="size-1.5 animate-pulse rounded-full bg-series" />
              reading now
            </>
          ) : part.ok_at ? (
            `went through ${ago(part.ok_at)}`
          ) : part.ran_at ? (
            `tried ${ago(part.ran_at)}`
          ) : (
            "not run yet"
          )}
        </span>
      </dt>
      <dd className="text-sm text-muted">{children}</dd>
      {part.error && !now ? (
        <dd className="text-sm text-down">
          <span aria-hidden="true">! </span>
          {part.error}
        </dd>
      ) : null}
    </div>
  );
}
