import { useMemo, useSyncExternalStore } from "react";

/**
 * Where the app is, read from the address bar and nowhere else, so a reload or a copied link
 * lands on the same thing.
 */
export type Route =
  | { page: "dashboard"; app: string | null; days: number }
  | { page: "status" }
  | { page: "missing" };

/** The ranges offered, in days. */
export const ranges = [7, 30, 90, 365] as const;

export const defaultDays = 30;

export const paths = {
  dashboard: (app: string | null, days: number) =>
    (app ? `/apps/${encodeURIComponent(app)}` : "/") +
    (days === defaultDays ? "" : `?days=${days}`),
  status: "/status",
};

/** A route from an address: its path, and its query for the range. */
export function parse(address: string): Route {
  const at = address.indexOf("?");
  const path = at < 0 ? address : address.slice(0, at);
  const params = new URLSearchParams(at < 0 ? "" : address.slice(at + 1));
  const asked = Number(params.get("days"));
  const days = (ranges as readonly number[]).includes(asked)
    ? asked
    : defaultDays;
  const parts = path.split("/").filter(Boolean).map(decodeURIComponent);
  if (parts.length === 0) return { page: "dashboard", app: null, days };
  if (parts[0] === "apps" && parts.length === 2 && /^\d+$/.test(parts[1]!)) {
    return { page: "dashboard", app: parts[1]!, days };
  }
  if (parts[0] === "status" && parts.length === 1) return { page: "status" };
  return { page: "missing" };
}

const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  window.addEventListener("popstate", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("popstate", listener);
  };
}

const here = () => window.location.pathname + window.location.search;

/** Moves within the island, as a link does, without loading another document. */
export function go(address: string) {
  if (address === here()) return;
  window.history.pushState(null, "", address);
  for (const listener of listeners) listener();
}

export function useRoute(): Route {
  const address = useSyncExternalStore(subscribe, here);
  return useMemo(() => parse(address), [address]);
}
