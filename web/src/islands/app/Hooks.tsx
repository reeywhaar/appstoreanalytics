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
  getSettingsRelay,
  postHooksEndpoints,
  putHooksMail,
} from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type {
  HookEndpoint,
  HookGroup,
  HookKind,
  Hooks,
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
    </li>
  );
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

function MailPanel({ hooks }: { hooks: HooksQuery }) {
  const client = useQueryClient();
  const relay = useQuery({ queryKey: qk.relay, queryFn: getSettingsRelay });
  const [editing, setEditing] = useState(false);
  const stop = useMutation({
    mutationFn: () => putHooksMail({ to: "", events: [] }),
    onSuccess: () => client.invalidateQueries({ queryKey: qk.hooks }),
  });
  const mail = hooks.data?.mail;
  const on = Boolean(mail?.to && mail.events.length > 0);

  return (
    <Panel title="Webhook mail" state={hooks}>
      <p className="mb-3 text-sm text-muted">
        A delivery of a kind chosen here is mailed through the relay, once it is
        proven to be Apple&rsquo;s.
      </p>
      <p className="text-sm">
        {on && mail ? (
          <>
            To <span className="text-fg">{mail.to}</span>:{" "}
            <span className="font-mono text-xs text-faint">
              {mail.events.join(", ")}
            </span>
          </>
        ) : (
          <span className="text-muted">Nobody is mailed.</span>
        )}
      </p>
      {relay.data && !relay.data.configured ? (
        <p className="mt-2 text-sm text-warn">
          There is no mail relay, so nothing is sent until one is set up.
        </p>
      ) : null}
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button onClick={() => setEditing(true)}>
          {on ? "Change it…" : "Set it up…"}
        </Button>
        {on ? (
          <Button variant="link" onClick={() => stop.mutate()}>
            Stop mailing
          </Button>
        ) : null}
      </div>
      {hooks.data ? (
        <MailDialog
          open={editing}
          hooks={hooks.data}
          onClose={() => setEditing(false)}
        />
      ) : null}
    </Panel>
  );
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
    ...new Set([...hooks.events.map((e) => e.event), ...hooks.mail.events]),
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
              kinds: others.map((name) => ({ name, meaning: "" })),
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
 * Who is mailed and of which kinds of event. Seeded from what is saved each time it opens, and
 * applied only by Save, so closing it is abandoning it.
 */
function MailDialog({
  open,
  hooks,
  onClose,
}: {
  open: boolean;
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
    setTo(hooks.mail.to);
    setChosen(hooks.mail.events);
    setError("");
  }, [open, hooks.mail]);

  const services = choices(hooks);
  const every = services.flatMap(namesOf);
  const toggle = (names: string[], on: boolean) =>
    setChosen((c) =>
      on
        ? [...c, ...names.filter((k) => !c.includes(k))]
        : c.filter((k) => !names.includes(k)),
    );

  const save = useMutation({
    mutationFn: () => putHooksMail({ to, events: chosen }),
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
      title="Webhook mail"
      footer={
        <Button
          type="submit"
          form="hook-mail"
          variant="solid"
          disabled={save.isPending}
        >
          {save.isPending ? "Saving…" : "Save"}
        </Button>
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
