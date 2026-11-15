import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getStatus, postSync } from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type { Part } from "@app/api/types";
import { Button } from "@app/components/Button";
import { Dummy } from "@app/components/Dummy";
import { ago } from "@app/format";

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
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 pt-4 pb-12">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-lg font-semibold">Status</h1>
        <span className="flex-1" />
        <Button
          variant="solid"
          size="bar"
          disabled={!s || s.running || sync.isPending}
          onClick={() => sync.mutate()}
        >
          {s?.running ? "Syncing…" : "Sync now"}
        </Button>
      </div>
      {sync.error ? (
        <p className="text-sm text-accent">{messageOf(sync.error)}</p>
      ) : null}
      <p className="text-sm text-muted">
        A sync runs every hour on its own. It reads whatever days of sales are
        missing, and the analytics reports Apple has made since the last one.
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
            title="Sales reports"
            part={s.sales.part}
            now={s.reading === "sales"}
          >
            {s.sales.days
              ? `${s.sales.days} days read, ${s.sales.first} to ${s.sales.last}`
              : "None read yet"}
          </Row>
          <Row
            title="Analytics reports"
            part={s.analytics.part}
            now={s.reading === "analytics"}
          >
            {s.analytics.instances
              ? `${s.analytics.instances} ${s.analytics.instances === 1 ? "report" : "reports"} read, measuring ${s.analytics.first} to ${s.analytics.last}`
              : "None read yet. Apple makes the first a day or two after they are asked for."}
          </Row>
          <Row
            title="Exchange rates"
            part={s.rates.part}
            now={s.reading === "rates"}
          >
            {s.rates.at ? `Read ${ago(s.rates.at)}` : "None read yet"}
          </Row>
        </dl>
      )}
    </main>
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
