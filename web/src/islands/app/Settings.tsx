import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  deleteSessions,
  deleteSessionsById,
  deleteSettingsRelay,
  getSessions,
  getSettingsRelay,
} from "@app/api/actions";
import { qk } from "@app/api/keys";
import { messageOf } from "@app/api/transport";
import { Button } from "@app/components/Button";
import { useConfirm } from "@app/components/Confirm";
import { DummyRows } from "@app/components/Dummy";
import { Panel } from "@app/components/Panel";
import { ago, moment } from "@app/format";
import { HooksPanels } from "@app/islands/app/Hooks";
import { RelayDialog, TestMailDialog } from "@app/islands/app/RelayDialog";
import { leaveFor } from "@app/leave";

/**
 * How this instance is set up: where its mail goes, what webhooks deliver, and who is signed
 * in, which is also where this browser signs out.
 */
export function Settings() {
  return (
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-2 pt-2 pb-6 sm:gap-10 sm:px-4 sm:pt-4 sm:pb-12">
      <h1 className="text-lg font-semibold">Settings</h1>
      <RelayPanel />
      <HooksPanels />
      <Sessions />
    </main>
  );
}

function RelayPanel() {
  const client = useQueryClient();
  const relay = useQuery({ queryKey: qk.relay, queryFn: getSettingsRelay });
  const [editing, setEditing] = useState(false);
  const [testing, setTesting] = useState(false);

  const remove = useMutation({
    mutationFn: () => deleteSettingsRelay(),
    onSuccess: () => client.invalidateQueries({ queryKey: qk.relay }),
  });

  const configured = relay.data?.configured ?? false;

  return (
    <Panel title="Mail relay" state={relay}>
      <p className="mb-3 text-sm text-muted">
        App Store analytics hands messages to a relay you already have: the
        webhooks chosen under Webhook mail. Without a relay nothing is sent.
      </p>

      <p className="text-sm">
        {configured ? (
          <>
            <span className="font-mono text-xs text-faint">
              {relay.data?.host}:{relay.data?.port}
            </span>{" "}
            · {relay.data?.security} · from {relay.data?.from_address}
          </>
        ) : (
          <span className="text-muted">Not set up. Nothing can be sent.</span>
        )}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button onClick={() => setEditing(true)}>
          {configured ? "Change it…" : "Set it up…"}
        </Button>
        {configured ? (
          <>
            <Button onClick={() => setTesting(true)}>Send a test…</Button>
            <Button variant="link" onClick={() => remove.mutate()}>
              Forget it
            </Button>
          </>
        ) : null}
      </div>

      <RelayDialog open={editing} onClose={() => setEditing(false)} />
      <TestMailDialog open={testing} onClose={() => setTesting(false)} />
    </Panel>
  );
}

/**
 * Every browser signed in, and a way to sign any of them out: the one in a pocket that was
 * lost, or all but this one.
 */
function Sessions() {
  const confirm = useConfirm();
  const client = useQueryClient();
  const sessions = useQuery({ queryKey: qk.sessions, queryFn: getSessions });

  const revoke = useMutation({
    mutationFn: (id: string) => deleteSessionsById(id),
    onSuccess: (_result, id) => {
      const wasCurrent = sessions.data?.sessions.find(
        (s) => s.id === id,
      )?.current;
      if (wasCurrent) leaveFor("/login");
      else void client.invalidateQueries({ queryKey: qk.sessions });
    },
  });

  const revokeOthers = useMutation({
    mutationFn: () => deleteSessions(),
    onSuccess: () => client.invalidateQueries({ queryKey: qk.sessions }),
  });

  const list = sessions.data?.sessions ?? [];

  return (
    <Panel title="Sessions" state={sessions} dummy={<DummyRows />}>
      <p className="mb-3 text-sm text-muted">
        Every browser signed in. Changing the password in the environment signs
        them all out.
      </p>
      <ul className="flex flex-col border-t border-line">
        {list.map((session) => (
          <li
            key={session.id}
            className="flex items-start gap-2 border-b border-line py-2 text-sm"
          >
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-baseline gap-x-2">
                <span>{session.device || "Unknown"}</span>
                {session.current ? (
                  <span className="text-xs text-muted">this browser</span>
                ) : null}
                <span className="text-xs text-faint">
                  last used {ago(session.last_seen_at)}, signed in{" "}
                  {moment(session.created_at)}
                </span>
              </div>
              {/* The guess is a parse of a field browsers have been lying in since 1993, so
                    the string it was guessed from sits under it, after where it was last seen
                    from: the two raw facts the line above is read from. */}
              {/* Scrolled rather than cut short: the end of a user agent is often the part that
                    tells two browsers apart. */}
              <p className="scroll-thin overflow-x-auto text-xs whitespace-nowrap text-faint">
                {session.ip ? (
                  <span className="tabular-nums">{session.ip} · </span>
                ) : null}
                {session.user_agent}
              </p>
            </div>
            <Button
              variant="link"
              onClick={async () => {
                const yes =
                  !session.current ||
                  (await confirm({
                    title: "Sign this browser out?",
                    message:
                      "This is the browser you are using. Signing it out returns you to the sign-in page.",
                    confirm: "Sign out",
                  }));
                if (yes) revoke.mutate(session.id);
              }}
            >
              {session.current ? "Sign out…" : "Sign out"}
            </Button>
          </li>
        ))}
      </ul>
      {revoke.error || revokeOthers.error ? (
        <p className="mt-2 text-sm text-accent">
          {messageOf(revoke.error ?? revokeOthers.error)}
        </p>
      ) : null}
      {list.length > 1 ? (
        <Button
          className="mt-3"
          disabled={revokeOthers.isPending}
          onClick={() => revokeOthers.mutate()}
        >
          Sign out everywhere else
        </Button>
      ) : null}
    </Panel>
  );
}
