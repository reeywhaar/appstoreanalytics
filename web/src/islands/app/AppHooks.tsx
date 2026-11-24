import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getHooksApple, postHooksAppleByAppConnect } from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import type {
  AppleApp,
  ApplePart,
  AppleSender,
  ConnectTo,
  HookEndpoint,
} from "@app/api/types";
import { Button } from "@app/components/Button";
import { Dialog } from "@app/components/Dialog";
import { DummyRows } from "@app/components/Dummy";
import { Field } from "@app/components/Field";
import { Panel } from "@app/components/Panel";
import { Segmented } from "@app/components/Segmented";
import { Select } from "@app/components/Select";
import { TextField } from "@app/components/TextField";

/** Each part: its column's name, and the dialog's. */
const parts: { part: ApplePart; name: string; title: string }[] = [
  { part: "webhook", name: "Webhook", title: "App Store Connect webhook" },
  { part: "production", name: "Production", title: "Server notifications" },
  { part: "sandbox", name: "Sandbox", title: "Sandbox notifications" },
];

/** What the automatic address of a part is labelled, as the server labels it. */
const automaticLabel: Record<ApplePart, string> = {
  webhook: "Webhook",
  production: "Production",
  sandbox: "Sandbox",
};

/** The columns: the app, then its three parts. */
const columns =
  "grid grid-cols-3 gap-x-3 sm:grid-cols-[minmax(0,9rem)_repeat(3,minmax(0,1fr))]";

/**
 * Where App Store Connect sends each app's webhook and its App Store Server Notifications: to
 * which of the addresses above, nowhere, or somewhere none of them is, which is a warning. A row
 * an app, a column a part, and each changed in a dialog of its own, by hand: nothing is set up
 * without being asked.
 */
export function AppsPanel({ endpoints }: { endpoints: HookEndpoint[] }) {
  const apple = useQuery({
    queryKey: qk.apple,
    queryFn: getHooksApple,
    // Asking Apple is an app at a time: not again on every return to the tab.
    staleTime: 60_000,
    retry: false,
  });
  const [changing, setChanging] = useState<{
    app: AppleApp;
    part: ApplePart;
  } | null>(null);

  return (
    <Panel
      title="App webhooks"
      state={{ ...apple, isError: false }}
      dummy={<DummyRows />}
    >
      <p className="mb-3 text-sm text-muted">
        Where App Store Connect sends each app&rsquo;s webhook and App Store
        Server Notifications. Choose one to change it.
      </p>
      {apple.error ? (
        <div className="flex flex-col items-start gap-2 text-sm">
          <p className="text-accent">{messageOf(apple.error)}</p>
          <Button onClick={() => void apple.refetch()}>Ask again</Button>
        </div>
      ) : apple.data && apple.data.apps.length === 0 ? (
        <p className="text-sm text-muted">App Store Connect lists no apps.</p>
      ) : (
        <div className="rounded-md border border-line bg-bg text-sm">
          <div
            className={`${columns} hidden border-b border-line px-3 py-1.5 text-xs text-muted sm:grid`}
            aria-hidden="true"
          >
            <span>App</span>
            {parts.map((p) => (
              <span key={p.part}>{p.name}</span>
            ))}
          </div>
          <ul className="flex flex-col divide-y divide-line">
            {apple.data?.apps.map((app) => (
              <li
                key={app.id}
                className={`${columns} items-center gap-y-0.5 px-3 py-1.5`}
              >
                <span className="col-span-3 truncate font-medium sm:col-span-1">
                  {app.name}
                </span>
                {parts.map((p) => (
                  <Cell
                    key={p.part}
                    app={app}
                    part={p}
                    endpoints={endpoints}
                    onChange={() => setChanging({ app, part: p.part })}
                  />
                ))}
              </li>
            ))}
          </ul>
        </div>
      )}
      <ChangeDialog
        change={changing}
        endpoints={endpoints}
        onClose={() => setChanging(null)}
      />
    </Panel>
  );
}

/** One part of an app: where it is sent, in a word or a label, and the press that changes it. */
function Cell({
  app,
  part: { part, name },
  endpoints,
  onChange,
}: {
  app: AppleApp;
  part: (typeof parts)[number];
  endpoints: HookEndpoint[];
  onChange: () => void;
}) {
  const said = standing(app[part], endpoints, app.name);
  return (
    <button
      type="button"
      onClick={onChange}
      title={said.title}
      aria-label={`${app.name}, ${name}: ${said.text}`}
      className="-mx-1.5 flex min-w-0 flex-col items-start rounded px-1.5 py-1 text-left hover:bg-control"
    >
      <span className="text-[11px] text-faint sm:hidden">{name}</span>
      <span
        className={`w-full truncate ${
          said.tone === "warn"
            ? "text-warn"
            : said.tone === "faint"
              ? "text-faint"
              : ""
        }`}
      >
        {said.text}
      </span>
    </button>
  );
}

/**
 * Where a part is sent, as a cell says it: an address's label; nothing; or a warning, for a
 * webhook turned off or sending only some events, and for an address none of these is, which one
 * at this instance's host is one since removed.
 */
function standing(
  s: AppleSender,
  endpoints: HookEndpoint[],
  app: string,
): { text: string; tone: "" | "warn" | "faint"; title?: string } {
  const full = endpoints.find((e) => e.id === s.endpoint)?.label ?? "";
  // In the app's own row, "Dots: Production" is its production, and "Dots" its main address: the
  // app's name goes without saying, and with it the label would be cut short.
  const label =
    full.toLowerCase() === app.toLowerCase()
      ? "Main"
      : full.toLowerCase().startsWith(`${app.toLowerCase()}: `)
        ? full.slice(app.length + 2)
        : full;
  switch (s.state) {
    case "ok":
      return { text: label, tone: "", title: full };
    case "off":
      return {
        text: `${label}, turned off`,
        tone: "warn",
        title: `${full}: turned off in App Store Connect`,
      };
    case "some":
      return {
        text: `${label}, some events`,
        tone: "warn",
        title: `${full}: sending only some kinds of event`,
      };
    case "none":
      return { text: "Empty", tone: "faint" };
    default: {
      const host = hostOf(s.url);
      const removed =
        host !== "" && endpoints.some((e) => hostOf(e.url) === host);
      return {
        text: removed ? "Removed address" : `Unknown: ${host || s.url}`,
        tone: "warn",
        title: s.url,
      };
    }
  }
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return "";
  }
}

type Choice = "auto" | "list" | "manual" | "empty";

/**
 * Changing where one part is sent: to its own automatic address, made the first time; to one of
 * the addresses; to one typed in, which a webhook also needs the secret of; or nowhere. Replacing
 * an address none of these is says so first: it may be the app's own server.
 */
function ChangeDialog({
  change,
  endpoints,
  onClose,
}: {
  change: { app: AppleApp; part: ApplePart } | null;
  endpoints: HookEndpoint[];
  onClose: () => void;
}) {
  const client = useQueryClient();
  const [choice, setChoice] = useState<Choice>("auto");
  const [auto, setAuto] = useState<"main" | "separate">("main");
  const [endpoint, setEndpoint] = useState("");
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState("");

  const part = change?.part;
  const sender = change ? change.app[change.part] : null;
  const webhook = part === "webhook";
  // The automatic address there is already, the app's main one or the part's own, if any.
  const own = change
    ? endpoints.find(
        (e) =>
          e.app === change.app.id &&
          e.part === (auto === "main" ? "main" : change.part),
      )
    : undefined;
  // Opened on what it is now: an automatic address, another of the addresses, one typed in, or
  // nothing, which opens on the app's main one. On opening only, with the addresses as they are
  // then, so a list read again behind it does not undo what is being chosen.
  useEffect(() => {
    if (!change) return;
    const now = change.app[change.part];
    const at = endpoints.find((e) => e.id === now.endpoint);
    const automatic = at?.app === change.app.id;
    setError("");
    setSecret("");
    setUrl(now.state === "unknown" ? now.url : "");
    setEndpoint(String(now.endpoint ?? endpoints[0]?.id ?? ""));
    setAuto(automatic && at?.part !== "main" ? "separate" : "main");
    if (now.state === "none" || automatic) {
      setChoice("auto");
    } else if (now.endpoint !== null) {
      setChoice("list");
    } else {
      setChoice("manual");
    }
  }, [change]);

  const save = useMutation({
    mutationFn: () => {
      const to: ConnectTo = { part: change!.part };
      if (choice === "auto") to.auto = auto;
      if (choice === "list") to.endpoint = Number(endpoint);
      if (choice === "manual") {
        to.url = url.trim();
        if (webhook) to.secret = secret;
      }
      return postHooksAppleByAppConnect(change!.app.id, to);
    },
    onSuccess: () => {
      // A new address joins the list above, and the apps' states change with it.
      void client.invalidateQueries({ queryKey: qk.hooks });
      onClose();
    },
    onError: (err) => setError(messageOf(err)),
  });

  const info = parts.find((p) => p.part === part);
  const ready =
    choice === "empty" ||
    choice === "auto" ||
    (choice === "list" && endpoint !== "") ||
    (choice === "manual" && url.trim() !== "" && (!webhook || secret !== ""));
  // What is there now and would be replaced, when it is none of the addresses.
  const replacing =
    sender?.state === "unknown" &&
    !(choice === "manual" && url.trim() === sender.url)
      ? sender.url
      : "";

  return (
    <Dialog
      open={change !== null}
      onClose={onClose}
      title={info?.title ?? ""}
      aside={change?.app.name}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button
            type="submit"
            form="apple-change"
            variant="solid"
            disabled={save.isPending || !ready}
          >
            Save
          </Button>
        </>
      }
    >
      <form
        id="apple-change"
        className="flex flex-col gap-3 pb-4 text-sm"
        onSubmit={(e) => {
          e.preventDefault();
          if (ready) save.mutate();
        }}
      >
        <Segmented<Choice>
          label="Send it to"
          value={choice}
          onChange={setChoice}
          options={[
            { value: "auto", label: "Auto" },
            { value: "list", label: "List" },
            { value: "manual", label: "Manual" },
            { value: "empty", label: "Empty" },
          ]}
        />
        {choice === "auto" ? (
          <>
            <Segmented<"main" | "separate">
              label="Which automatic address"
              value={auto}
              onChange={setAuto}
              options={[
                { value: "main", label: "Main" },
                { value: "separate", label: "Separate" },
              ]}
            />
            <p className="text-muted">
              {auto === "main"
                ? `One address for all of ${change?.app.name}'s parts. `
                : "An address for this part alone, so whoever is mailed can be mailed of it alone. "}
              {own ? (
                <>It is &ldquo;{own.label}&rdquo;, in the list above.</>
              ) : (
                <>
                  It is added to the list above as &ldquo;
                  {auto === "main"
                    ? change?.app.name
                    : `${change?.app.name}: ${part ? automaticLabel[part] : ""}`}
                  &rdquo;.
                </>
              )}
            </p>
          </>
        ) : choice === "list" ? (
          endpoints.length === 0 ? (
            <p className="text-muted">
              There are no addresses yet: add one above, or choose Auto.
            </p>
          ) : (
            <Select
              aria-label="Address"
              className="w-full"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
            >
              {endpoints.map((e) => (
                <option key={e.id} value={e.id}>
                  {e.label}
                </option>
              ))}
            </Select>
          )
        ) : choice === "manual" ? (
          <div className="flex flex-col gap-2">
            <TextField
              aria-label="Address"
              placeholder="https://example.com/apple"
              autoComplete="off"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              className="w-full font-mono"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
            {webhook ? (
              <Field
                label="Secret"
                hint="App Store Connect signs every delivery with it, and the server at that address checks it."
              >
                <TextField
                  autoComplete="off"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  className="w-full font-mono"
                  value={secret}
                  onChange={(e) => setSecret(e.target.value)}
                />
              </Field>
            ) : null}
          </div>
        ) : (
          <p className="text-muted">
            {webhook
              ? "The webhook is deleted in App Store Connect."
              : `Apple sends ${change?.app.name}'s ${part} notifications nowhere.`}
          </p>
        )}
        {webhook && choice !== "empty" ? (
          <p className="text-muted">
            The webhook is turned on and sends every kind of event; which are
            mailed is chosen under Webhook mail.
          </p>
        ) : null}
        {replacing ? (
          <p className="text-warn">
            It is sent to{" "}
            <code className="font-mono break-all">{replacing}</code> now, which
            is none of the addresses here. If that is {change?.app.name}
            &rsquo;s own server, it stops hearing of{" "}
            {webhook ? "these events" : "its subscriptions"}.
          </p>
        ) : null}
        {error ? <p className="text-accent">{error}</p> : null}
      </form>
    </Dialog>
  );
}
