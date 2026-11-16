import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryResult,
} from "@tanstack/react-query";

import {
  deleteHooksEndpointsById,
  getHooks,
  deleteHooksMailsById,
  getSettingsRelay,
  postHooksEndpoints,
  postHooksEndpointsByIdTest,
  postHooksMails,
  putHooksMailsById,
} from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type {
  HookEndpoint,
  HookGroup,
  HookKind,
  HookMail,
  Hooks,
  HookSender,
  HookService,
  NewHookEndpoint,
} from "@app/api/types";
import { copy } from "@app/clipboard";
import { Button } from "@app/components/Button";
import { useConfirm } from "@app/components/Confirm";
import { CopyBlock } from "@app/components/CopyBlock";
import { Dialog } from "@app/components/Dialog";
import { DummyRows } from "@app/components/Dummy";
import { Field } from "@app/components/Field";
import { Panel } from "@app/components/Panel";
import { TextField } from "@app/components/TextField";
import { ago } from "@app/format";

type HooksQuery = UseQueryResult<Hooks>;

/**
 * Where Apple's webhooks are sent and which kinds are mailed: a section each, showing what is set
 * and changing it only in a dialog of its own. What has been delivered is on the dashboard.
 */
export function HooksPanels() {
  const hooks = useQuery({ queryKey: qk.hooks, queryFn: getHooks });
  return (
    <>
      <Endpoints hooks={hooks} />
      <MailPanel hooks={hooks} />
    </>
  );
}

/**
 * The addresses Apple delivers to: one for each place in App Store Connect that asks for one,
 * each with its own secret, so one can be removed without touching the others.
 */
function Endpoints({ hooks }: { hooks: HooksQuery }) {
  const [adding, setAdding] = useState(false);
  const list = hooks.data?.endpoints ?? [];

  return (
    <Panel title="Webhooks" state={hooks} dummy={<DummyRows />}>
      <p className="mb-3 text-sm text-muted">
        An address for each place Apple asks for one: App Store Server
        Notifications in an app&rsquo;s App Information, which need only the
        address, and each webhook in Users and Access, Integrations, which needs
        its secret too. Everything delivered is kept, the newest 1,000, and
        shown on the dashboard.
      </p>
      {list.length > 0 ? (
        <ul className="mb-3 flex flex-col divide-y divide-line rounded-md border border-line bg-bg">
          {list.map((e) => (
            <EndpointRow key={e.id} endpoint={e} />
          ))}
        </ul>
      ) : (
        <p className="mb-3 text-sm text-muted">No addresses yet.</p>
      )}
      <Button onClick={() => setAdding(true)}>Add an address…</Button>
      <AddEndpointDialog open={adding} onClose={() => setAdding(false)} />
    </Panel>
  );
}

/**
 * One address: what it is called, the address itself, whether anything has come through it, and
 * its commands, on a line of their own on a phone.
 */
function EndpointRow({ endpoint: e }: { endpoint: HookEndpoint }) {
  const client = useQueryClient();
  const confirm = useConfirm();
  const [copied, setCopied] = useState(false);
  const [testing, setTesting] = useState(false);
  const remove = useMutation({
    mutationFn: () => deleteHooksEndpointsById(e.id),
    onSuccess: () => client.invalidateQueries({ queryKey: qk.hooks }),
  });

  return (
    <li className="flex flex-col gap-2 px-3 py-2.5 text-sm sm:flex-row sm:items-center sm:gap-3">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-medium">{e.label}</span>
          <span className="text-xs text-faint">
            {e.last_at === null
              ? "nothing delivered yet"
              : `last delivery ${ago(e.last_at)}`}
          </span>
        </span>
        <code className="font-mono text-xs break-all text-muted">{e.url}</code>
        {!e.secret_set ? (
          <span className="text-xs text-warn">
            No secret: App Store Connect webhooks to it are kept unproven, and
            never mailed.
          </span>
        ) : null}
        {remove.error ? (
          <span className="text-xs text-accent">{messageOf(remove.error)}</span>
        ) : null}
      </div>
      <span className="flex shrink-0 items-center gap-2">
        <Button
          size="bar"
          onClick={async () => {
            if (!(await copy(e.url))) return;
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1200);
          }}
        >
          {copied ? "Copied" : "Copy address"}
        </Button>
        <Button size="bar" onClick={() => setTesting(true)}>
          Test
        </Button>
        <Button
          size="bar"
          disabled={remove.isPending}
          onClick={async () => {
            const yes = await confirm({
              title: `Remove ${e.label}?`,
              message:
                "The address stops answering at once: whatever sends to it gets nothing from then on. What it delivered stays.",
              confirm: "Remove",
              danger: true,
            });
            if (yes) remove.mutate();
          }}
        >
          Remove…
        </Button>
      </span>
      <TestDialog
        endpoint={e}
        open={testing}
        onClose={() => {
          setTesting(false);
          // What was asked for may have arrived by now, and the row says when the last did.
          void client.invalidateQueries({ queryKey: qk.hooks });
        }}
      />
    </li>
  );
}

/**
 * Asking Apple to send an address a test, as soon as it opens: what in App Store Connect sends
 * to it, and what asking each came to. What arrives is on the dashboard, as any delivery is.
 */
function TestDialog({
  endpoint: e,
  open,
  onClose,
}: {
  endpoint: HookEndpoint;
  open: boolean;
  onClose: () => void;
}) {
  const test = useMutation({
    mutationFn: () => postHooksEndpointsByIdTest(e.id),
  });
  const { mutate, reset } = test;
  // Once an opening: StrictMode runs an effect twice, and each run would ask Apple again.
  const asked = useRef(false);
  useEffect(() => {
    if (!open) {
      asked.current = false;
      reset();
      return;
    }
    if (asked.current) return;
    asked.current = true;
    mutate();
  }, [open, mutate, reset]);

  const senders = test.data?.senders;
  const anyAsked = senders?.some((x) => x.result === "asked") ?? false;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={`Test ${e.label}`}
      footer={
        <>
          <Button disabled={test.isPending} onClick={() => test.mutate()}>
            Test again
          </Button>
          <Button variant="solid" onClick={onClose}>
            Done
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3 pb-4 text-sm">
        {test.isPending || test.isIdle ? (
          <>
            <p className="text-muted">
              Asking App Store Connect what sends to this address…
            </p>
            <DummyRows />
          </>
        ) : test.error ? (
          <p className="text-accent">{messageOf(test.error)}</p>
        ) : senders && senders.length === 0 ? (
          <p className="text-muted">
            Nothing in App Store Connect sends here: no app&rsquo;s App Store
            Server Notifications URL and no webhook is this address.
            {e.url.startsWith("https://")
              ? null
              : " Apple sends only to an https address, and this one is not."}
          </p>
        ) : senders ? (
          <>
            <ul className="flex flex-col divide-y divide-line rounded-md border border-line">
              {senders.map((x, i) => (
                <li key={i} className="flex flex-col gap-0.5 px-3 py-2">
                  <span className="font-medium">{senderName(x)}</span>
                  <SenderResult sender={x} />
                </li>
              ))}
            </ul>
            {anyAsked ? (
              <p className="text-muted">
                What Apple sends shows on the dashboard as it arrives, usually
                within a minute.
              </p>
            ) : null}
          </>
        ) : null}
      </div>
    </Dialog>
  );
}

function senderName(x: HookSender): string {
  if (x.kind === "notifications") {
    return `${x.app}: App Store Server Notifications, ${x.name}`;
  }
  return x.name ? `${x.app}: webhook “${x.name}”` : `${x.app}: webhooks`;
}

function SenderResult({ sender: x }: { sender: HookSender }) {
  switch (x.result) {
    case "asked":
      return (
        <span className="text-xs text-up">
          {x.kind === "notifications"
            ? "Asked Apple for a test notification. It arrives as TEST."
            : "Asked Apple to ping it. It arrives as webhookPingCreated."}
        </span>
      );
    case "off":
      return (
        <span className="text-xs text-muted">
          Turned off in App Store Connect, so not pinged.
        </span>
      );
    case "no_key":
      return (
        <span className="text-xs text-warn">
          This app&rsquo;s notifications come here, but no test was asked for:
          Apple takes that request only with an In-App Purchase key, and none is
          set. Make one in App Store Connect under Users and Access,
          Integrations, In-App Purchase, set it as APPSTOREANALYTICS_IAP_KEY_ID
          and APPSTOREANALYTICS_IAP_PRIVATE_KEY_BASE64, and restart.
        </span>
      );
    default:
      return <span className="text-xs text-accent">{x.message}</span>;
  }
}

/**
 * Whether a button that has just appeared should still ignore a press: a stuttered double click
 * lands its second click on whatever replaced the first button, and a Done there throws away a
 * secret shown once.
 */
function useSettling(shown: unknown, ms = 500) {
  const since = useRef(0);
  useEffect(() => {
    since.current = performance.now();
  }, [shown]);
  return () => performance.now() - since.current < ms;
}

/**
 * Adding an address: a label, and a secret, generated when left empty. Once added, the address
 * and its secret are shown here, the secret for the only time.
 */
function AddEndpointDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [label, setLabel] = useState("");
  const [secret, setSecret] = useState("");
  const [made, setMade] = useState<NewHookEndpoint | null>(null);
  const [error, setError] = useState("");
  const settling = useSettling(made);
  useEffect(() => {
    if (!open) return;
    setLabel("");
    setSecret("");
    setMade(null);
    setError("");
  }, [open]);

  const add = useMutation({
    mutationFn: () => postHooksEndpoints({ label, secret }),
    onSuccess: (e) => {
      setMade(e);
      void client.invalidateQueries({ queryKey: qk.hooks });
    },
    onError: (err) => setError(messageOf(err)),
  });

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={made ? "Address added" : "Add an address"}
      footer={
        made ? (
          <Button variant="solid" onClick={() => !settling() && onClose()}>
            Done
          </Button>
        ) : (
          <>
            <Button onClick={onClose}>Cancel</Button>
            <Button
              type="submit"
              form="hook-endpoint"
              variant="solid"
              disabled={add.isPending || !label.trim()}
            >
              Add
            </Button>
          </>
        )
      }
    >
      {made ? (
        <div className="flex flex-col gap-3 pb-4">
          <CopyBlock
            rows={[
              { label: "Address", value: made.url },
              { label: "Secret", value: made.secret },
            ]}
          />
          <p className="text-sm text-muted">
            The secret is shown once. An App Store Connect webhook takes both;
            App Store Server Notifications take only the address. A lost secret
            is replaced by adding another address and removing this one.
          </p>
        </div>
      ) : (
        <form
          id="hook-endpoint"
          className="flex flex-col gap-3 pb-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (label.trim()) add.mutate();
          }}
        >
          <Field label="Label">
            <TextField
              placeholder="Dots: App Store Connect"
              autoComplete="off"
              data-autofocus
              className="w-full"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          </Field>
          <Field
            label="Secret"
            hint="Empty: generated, 32 characters. Type one only for a webhook App Store Connect already has. Shown once either way."
          >
            <TextField
              placeholder="Generated"
              autoComplete="off"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              className="w-full font-mono"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
            />
          </Field>
          {error ? <p className="text-sm text-accent">{error}</p> : null}
        </form>
      )}
    </Dialog>
  );
}

/**
 * Everybody mailed of webhooks, each with the kinds of event they chose: as many as there are
 * people to tell, so the builds go to one and the refunds to another.
 */
function MailPanel({ hooks }: { hooks: HooksQuery }) {
  const relay = useQuery({ queryKey: qk.relay, queryFn: getSettingsRelay });
  // The entry being changed, null for one being added, undefined for no dialog.
  const [editing, setEditing] = useState<HookMail | null | undefined>(
    undefined,
  );
  const list = hooks.data?.mails ?? [];

  return (
    <Panel title="Webhook mail" state={hooks} dummy={<DummyRows />}>
      <p className="mb-3 text-sm text-muted">
        Each address here is mailed the kinds of event chosen for it, through
        the relay, once a delivery is proven to be Apple&rsquo;s.
      </p>
      {relay.data && !relay.data.configured ? (
        <p className="mb-3 text-sm text-warn">
          There is no mail relay, so nothing is sent until one is set up.
        </p>
      ) : null}
      {list.length > 0 && hooks.data ? (
        <ul className="mb-3 flex flex-col divide-y divide-line rounded-md border border-line bg-bg">
          {list.map((m) => (
            <MailRow
              key={m.id}
              mail={m}
              hooks={hooks.data}
              onChange={() => setEditing(m)}
            />
          ))}
        </ul>
      ) : (
        <p className="mb-3 text-sm text-muted">Nobody is mailed.</p>
      )}
      <Button onClick={() => setEditing(null)}>Add a recipient…</Button>
      {hooks.data ? (
        <MailDialog
          open={editing !== undefined}
          mail={editing ?? null}
          hooks={hooks.data}
          onClose={() => setEditing(undefined)}
        />
      ) : null}
    </Panel>
  );
}

/** One recipient: whom, of what in words, and its commands. */
function MailRow({
  mail: m,
  hooks,
  onChange,
}: {
  mail: HookMail;
  hooks: Hooks;
  onChange: () => void;
}) {
  const client = useQueryClient();
  const confirm = useConfirm();
  const remove = useMutation({
    mutationFn: () => deleteHooksMailsById(m.id),
    onSuccess: () => client.invalidateQueries({ queryKey: qk.hooks }),
  });

  return (
    <li className="flex flex-col gap-2 px-3 py-2.5 text-sm sm:flex-row sm:items-center sm:gap-3">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-medium break-all">{m.to}</span>
          <span className="text-xs text-faint">
            {m.events.length === 1 ? "1 kind" : `${m.events.length} kinds`}
          </span>
        </span>
        <span className="text-xs text-muted">{described(hooks, m.events)}</span>
        {remove.error ? (
          <span className="text-xs text-accent">{messageOf(remove.error)}</span>
        ) : null}
      </div>
      <span className="flex shrink-0 items-center gap-2">
        <Button size="bar" onClick={onChange}>
          Change…
        </Button>
        <Button
          size="bar"
          disabled={remove.isPending}
          onClick={async () => {
            const yes = await confirm({
              title: `Stop mailing ${m.to}?`,
              message:
                "Nothing more is mailed to it. Anybody else mailed of the same kinds still is.",
              confirm: "Stop mailing",
              danger: true,
            });
            if (yes) remove.mutate();
          }}
        >
          Remove…
        </Button>
      </span>
    </li>
  );
}

/**
 * Kinds of event in words, as few as say them: a whole service or group by its name, and the
 * rest by their titles. "Other" is in both services, so its kinds are named one by one.
 */
function described(hooks: Hooks, events: string[]): string {
  const left = new Set(events);
  const parts: string[] = [];
  const all = (names: string[]) =>
    names.length > 0 && names.every((n) => left.has(n));
  const take = (names: string[]) => names.forEach((n) => left.delete(n));
  for (const service of hooks.catalog) {
    const names = namesOf(service);
    if (all(names)) {
      parts.push(`all ${service.name}`);
      take(names);
      continue;
    }
    for (const g of service.groups) {
      const inGroup = namesOf(g);
      if (g.name !== "Other" && all(inGroup)) {
        parts.push(g.name);
        take(inGroup);
        continue;
      }
      for (const k of g.kinds) {
        if (left.has(k.name)) {
          parts.push(k.title);
          left.delete(k.name);
        }
      }
    }
  }
  return [...parts, ...left].join(", ");
}

/**
 * The kinds to choose from: every one Apple documents, by the service that sends it and then by
 * what it is about, then any delivered or chosen that it does not, such as a notification's
 * subtype.
 */
function choices(hooks: Hooks): HookService[] {
  const known = new Set(
    hooks.catalog.flatMap((s) =>
      s.groups.flatMap((g) => g.kinds.map((k) => k.name)),
    ),
  );
  const others = [
    ...new Set([
      ...hooks.events.map((e) => e.event),
      ...hooks.mails.flatMap((m) => m.events),
    ]),
  ]
    .filter((name) => !known.has(name))
    .toSorted();
  return others.length === 0
    ? hooks.catalog
    : [
        ...hooks.catalog,
        {
          name: "Others delivered",
          groups: [
            {
              name: "Not in Apple’s lists",
              kinds: others.map((name) => ({ name, title: name, meaning: "" })),
            },
          ],
        },
      ];
}

/** Every kind's name in a service or a group. */
function namesOf(item: HookService | HookGroup): string[] {
  return "groups" in item
    ? item.groups.flatMap((g) => g.kinds.map((k) => k.name))
    : item.kinds.map((k) => k.name);
}

/**
 * What is inside the box above, a step in: a line down from under that box, and the boxes of
 * this level lined up just past it, so which box covers which reads at a glance.
 */
function Nested({ children }: { children: ReactNode }) {
  return (
    <div className="ml-2 flex flex-col gap-2 border-l border-line pt-2 pl-4">
      {children}
    </div>
  );
}

/** How many of a kind have arrived: a type counts its subtypes, as mailing it does. */
function delivered(hooks: Hooks, kind: string): number {
  return hooks.events
    .filter((e) => e.event === kind || e.event.startsWith(kind + "/"))
    .reduce((sum, e) => sum + e.count, 0);
}

/**
 * One recipient and the kinds of event they are mailed of, a new one or one there is. Seeded each
 * time it opens, and applied only by Save, so closing it is abandoning it.
 */
function MailDialog({
  open,
  mail,
  hooks,
  onClose,
}: {
  open: boolean;
  /** The one being changed, null for one being added. */
  mail: HookMail | null;
  hooks: Hooks;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [to, setTo] = useState("");
  const [chosen, setChosen] = useState<string[]>([]);
  const [error, setError] = useState("");

  const seeded = useRef(false);
  useEffect(() => {
    if (!open) seeded.current = false;
  }, [open]);
  useEffect(() => {
    if (!open || seeded.current) return;
    seeded.current = true;
    setTo(mail?.to ?? "");
    setChosen(mail?.events ?? []);
    setError("");
  }, [open, mail]);

  const services = choices(hooks);
  const every = services.flatMap(namesOf);
  const toggle = (names: string[], on: boolean) =>
    setChosen((c) =>
      on
        ? [...c, ...names.filter((k) => !c.includes(k))]
        : c.filter((k) => !names.includes(k)),
    );

  const save = useMutation({
    mutationFn: () =>
      mail
        ? putHooksMailsById(mail.id, { to, events: chosen })
        : postHooksMails({ to, events: chosen }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: qk.hooks });
      onClose();
    },
    onError: (err) => setError(messageOf(err)),
  });

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={mail ? `Mail to ${mail.to}` : "Add a recipient"}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button
            type="submit"
            form="hook-mail"
            variant="solid"
            disabled={save.isPending || !to.trim() || chosen.length === 0}
          >
            {save.isPending ? "Saving…" : mail ? "Save" : "Add"}
          </Button>
        </>
      }
    >
      <form
        id="hook-mail"
        className="flex flex-col gap-5 pb-4"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Field label="To">
          <TextField
            type="email"
            data-autofocus
            className="w-full"
            placeholder="you@example.com"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </Field>
        <AllOf names={every} chosen={chosen} onToggle={toggle}>
          <span className="text-sm">Every kind</span>
        </AllOf>
        <div className="flex flex-col gap-5 border-t border-line pt-4">
          {services.map((service) => (
            <div key={service.name} role="group" aria-label={service.name}>
              <AllOf names={namesOf(service)} chosen={chosen} onToggle={toggle}>
                <span className="caps text-sm text-muted">{service.name}</span>
              </AllOf>
              <Nested>
                {service.groups.map((group) => (
                  <div
                    key={group.name}
                    role="group"
                    aria-label={group.name}
                    className="pb-1"
                  >
                    <AllOf
                      names={namesOf(group)}
                      chosen={chosen}
                      onToggle={toggle}
                    >
                      <span className="text-sm font-medium">{group.name}</span>
                    </AllOf>
                    <Nested>
                      {group.kinds.map((kind) => (
                        <KindBox
                          key={kind.name}
                          kind={kind}
                          count={delivered(hooks, kind.name)}
                          chosen={chosen.includes(kind.name)}
                          onToggle={(on) => toggle([kind.name], on)}
                        />
                      ))}
                    </Nested>
                  </div>
                ))}
              </Nested>
            </div>
          ))}
        </div>
        {error ? <p className="text-sm text-accent">{error}</p> : null}
      </form>
    </Dialog>
  );
}

/**
 * One kind of event: its name, how many have arrived, and what it means. Not selectable: the
 * whole of it is the box's label, so a double click is two presses, not a selection.
 */
function KindBox({
  kind,
  count,
  chosen,
  onToggle,
}: {
  kind: HookKind;
  count: number;
  chosen: boolean;
  onToggle: (on: boolean) => void;
}) {
  return (
    <label className="flex items-start gap-2 select-none">
      <Box>
        <input
          type="checkbox"
          className="size-4"
          checked={chosen}
          onChange={(e) => onToggle(e.target.checked)}
        />
      </Box>
      <span className="min-w-0 text-xs">
        <span className="block leading-5">
          <span className="font-mono break-all">{kind.name}</span>
          {count > 0 ? (
            <span className="text-faint">
              {" "}
              · {count.toLocaleString()} delivered
            </span>
          ) : null}
        </span>
        {kind.meaning ? (
          <span className="block text-muted">{kind.meaning}</span>
        ) : null}
      </span>
    </label>
  );
}

/**
 * As tall as a line of a kind's name, with the box centred on it: aligned to the top instead, a
 * box and a line of small text are a few pixels apart, by an amount that differs by browser.
 */
function Box({ children }: { children: ReactNode }) {
  return <span className="flex h-5 shrink-0 items-center">{children}</span>;
}

/**
 * One box for a set of kinds: ticked when every one is chosen, a dash when some are, and pressing
 * it chooses them all, or, when all were, none.
 */
function AllOf({
  names,
  chosen,
  onToggle,
  children,
}: {
  names: string[];
  chosen: string[];
  onToggle: (names: string[], on: boolean) => void;
  children: ReactNode;
}) {
  const n = names.filter((k) => chosen.includes(k)).length;
  const all = n === names.length;
  return (
    <label className="flex items-center gap-2 select-none">
      <Box>
        <input
          type="checkbox"
          className="size-4"
          checked={all}
          // indeterminate is a property with no attribute, so React cannot set it.
          ref={(el) => {
            if (el) {
              el.indeterminate = n > 0 && !all;
            }
          }}
          onChange={() => onToggle(names, !all)}
        />
      </Box>
      <span className="min-w-0 flex-1">{children}</span>
      <span className="shrink-0 text-xs text-faint tabular-nums">
        {n} of {names.length}
      </span>
    </label>
  );
}
