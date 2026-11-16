import { useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";

import { getHooksById, getHooksDeliveries } from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type { Delivery } from "@app/api/types";
import { copy } from "@app/clipboard";
import { Button } from "@app/components/Button";
import { Dialog } from "@app/components/Dialog";
import { Dummy, DummyRows } from "@app/components/Dummy";
import { ago, moment } from "@app/format";
import { Link } from "@app/islands/app/Link";
import { paths } from "@app/islands/app/route";

/** How many rows show before somebody asks for the rest of what was read. */
const SHOWN = 10;

/**
 * What Apple's webhooks have delivered, newest first: a line each, and everything else about one
 * behind its Info, so the list stays a list.
 */
export function Deliveries() {
  const list = useQuery({
    queryKey: qk.deliveries,
    queryFn: getHooksDeliveries,
    refetchInterval: 30_000,
  });
  const [all, setAll] = useState(false);
  const [open, setOpen] = useState<Delivery | null>(null);

  if (list.error) {
    return <p className="text-sm text-accent">{messageOf(list.error)}</p>;
  }
  if (!list.data) return <DummyRows />;
  const deliveries = list.data.deliveries;
  if (deliveries.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-line px-4 py-6 text-sm text-muted">
        Nothing delivered yet. The addresses to give Apple are made in{" "}
        <Link href={paths.settings} className="underline">
          Settings
        </Link>
        .
      </p>
    );
  }
  const shown = all ? deliveries : deliveries.slice(0, SHOWN);

  return (
    <>
      <ul className="flex flex-col divide-y divide-line rounded-lg border border-line bg-bg">
        {shown.map((d) => (
          <li key={d.id} className="flex items-center gap-3 px-4 py-2.5">
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm">{d.summary || d.event}</p>
              <p className="flex flex-wrap gap-x-2 text-xs text-faint">
                <span className="tabular-nums">{moment(d.received_at)}</span>
                {d.via ? <span>{d.via}</span> : null}
                <Proof delivery={d} />
                <Mailed delivery={d} />
              </p>
            </div>
            <Button size="bar" onClick={() => setOpen(d)}>
              Info
            </Button>
          </li>
        ))}
      </ul>
      {deliveries.length > shown.length ? (
        <div>
          <Button variant="link" onClick={() => setAll(true)}>
            Show {deliveries.length - shown.length} more
          </Button>
        </div>
      ) : null}
      <DeliveryDialog delivery={open} onClose={() => setOpen(null)} />
    </>
  );
}

/** Whether a delivery is proven to be Apple's, in a word or two. */
function Proof({ delivery: d }: { delivery: Delivery }) {
  if (d.proof_error) {
    return <span className="text-down">signature failed</span>;
  }
  switch (d.proven_by) {
    case "apple":
      return <span className="text-up">signed by Apple</span>;
    case "secret":
      return <span className="text-up">signed</span>;
    default:
      return <span>unsigned</span>;
  }
}

/** Whether a delivery was mailed: nothing for one that was not to be. */
function Mailed({ delivery: d }: { delivery: Delivery }) {
  if (d.mailed_at === null) return null;
  return d.mail_error ? (
    <span className="text-down">mail failed</span>
  ) : (
    <span className="text-up">mailed</span>
  );
}

/** Everything about one delivery: where it is proven to come from, its mail, and its body. */
function DeliveryDialog({
  delivery,
  onClose,
}: {
  delivery: Delivery | null;
  onClose: () => void;
}) {
  // Kept through the closing, so the dialog does not empty while it shuts.
  const [last, setLast] = useState(delivery);
  if (delivery && delivery !== last) setLast(delivery);
  const d = delivery ?? last;

  return (
    <Dialog
      open={delivery !== null}
      onClose={onClose}
      title={d && d.event !== "unknown" ? d.event : "Webhook"}
      wide
    >
      {d ? <Details key={d.id} delivery={d} /> : null}
    </Dialog>
  );
}

function Details({ delivery: d }: { delivery: Delivery }) {
  const one = useQuery({
    queryKey: qk.hook(d.id),
    queryFn: () => getHooksById(d.id),
  });
  const [copied, setCopied] = useState(false);
  const text = one.data
    ? one.data.decoded !== null
      ? JSON.stringify(one.data.decoded, null, 2)
      : one.data.body
    : "";

  return (
    <div className="flex flex-col gap-4 pb-4">
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
        <Row term="Received">
          {moment(d.received_at)}{" "}
          <span className="text-faint">({ago(d.received_at)})</span>
        </Row>
        {d.summary ? <Row term="Summary">{d.summary}</Row> : null}
        {d.via ? <Row term="Address">{d.via}</Row> : null}
        <Row term="Origin">
          <Origin delivery={d} />
        </Row>
        <Row term="Mail">
          <Mail delivery={d} />
        </Row>
        <Row term="Content">
          {d.size.toLocaleString()} bytes
          {one.data ? `, ${one.data.content_type || "no content type"}` : null}
        </Row>
      </dl>
      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-2">
          <h3 className="caps text-sm text-muted">Body</h3>
          {one.data ? (
            <Button
              size="bar"
              onClick={async () => {
                if (!(await copy(text))) return;
                setCopied(true);
                window.setTimeout(() => setCopied(false), 1200);
              }}
            >
              {copied ? "Copied" : "Copy"}
            </Button>
          ) : null}
        </div>
        {one.error ? (
          <p className="text-sm text-accent">{messageOf(one.error)}</p>
        ) : !one.data ? (
          <Dummy className="h-40" />
        ) : (
          <pre className="overflow-x-auto rounded-md bg-fill p-3 font-mono text-xs whitespace-pre-wrap break-all">
            {text}
          </pre>
        )}
        {one.data?.body.includes("signedPayload") ? (
          <p className="text-xs text-faint">
            The strings Apple signed are shown opened up; the delivery is kept
            exactly as it came.
          </p>
        ) : null}
      </div>
    </div>
  );
}

function Row({ term, children }: { term: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-muted">{term}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </>
  );
}

function Origin({ delivery: d }: { delivery: Delivery }) {
  if (d.proof_error) {
    return <span className="text-down">Not proven. {d.proof_error}</span>;
  }
  switch (d.proven_by) {
    case "apple":
      return <>Apple: its signatures lead to Apple&rsquo;s root certificate.</>;
    case "secret":
      return <>App Store Connect: signed with its address&rsquo;s secret.</>;
    default:
      return (
        <span className="text-muted">
          Not proven: it carries no signature, and its address has no secret to
          expect one.
        </span>
      );
  }
}

function Mail({ delivery: d }: { delivery: Delivery }) {
  if (d.mailed_at === null) {
    return (
      <span className="text-muted">
        {d.proven_by
          ? "Not mailed: nobody under Webhook mail chose its kind."
          : "Not mailed: it is not proven to be Apple’s."}
      </span>
    );
  }
  // Each recipient was tried, so some may have it and some not: the relay's words name which.
  const to = d.mailed_to.length > 0 ? ` to ${d.mailed_to.join(", ")}` : "";
  return (
    <>
      {d.mail_error ? "Tried mailing" : "Mailed"}
      {to}, {moment(d.mailed_at)}.
      {d.mail_error ? (
        <span className="block text-down">Not taken: {d.mail_error}</span>
      ) : null}
    </>
  );
}
