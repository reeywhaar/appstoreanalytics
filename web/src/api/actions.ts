import { query, request } from "@app/api/transport";
import type {
  App,
  History,
  Me,
  Relay,
  Report,
  Session,
  Status,
} from "@app/api/types";

/** Named mechanically from the route, so a call site and a handler find each other by grep. */

export const postAuthLogin = (body: { username: string; password: string }) =>
  request<void>("/api/auth/login", { method: "POST", body });

export const postAuthLogout = () =>
  request<void>("/api/auth/logout", { method: "POST" });

export const getAuthMe = () => request<Me>("/api/auth/me");

export const getApps = () => request<App[]>("/api/apps");

/** What a report is asked for: so many days up to the last one known, of one app or all. */
export type Ask = { days: number; app?: string };

export const getReport = ({ days, app }: Ask, signal?: AbortSignal) =>
  request<Report>(`/api/report${query({ days, app })}`, { signal });

export const getHistory = (app: string | undefined, signal?: AbortSignal) =>
  request<History>(`/api/history${query({ app })}`, { signal });

export const getStatus = () => request<Status>("/api/status");

export const postSync = () =>
  request<{ started: boolean }>("/api/sync", { method: "POST" });

export const getSettingsRelay = () => request<Relay>("/api/settings/relay");

export const putSettingsRelay = (body: {
  host: string;
  port: number;
  security: string;
  username: string;
  password: string;
  from_address: string;
  from_name: string;
}) => request<Relay>("/api/settings/relay", { method: "PUT", body });

export const deleteSettingsRelay = () =>
  request<void>("/api/settings/relay", { method: "DELETE" });

export const postSettingsRelayTest = (body: { to: string }) =>
  request<void>("/api/settings/relay/test", { method: "POST", body });

export const getSessions = () =>
  request<{ sessions: Session[] }>("/api/sessions");

export const deleteSessionsById = (id: string) =>
  request<void>(`/api/sessions/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });

export const deleteSessions = () =>
  request<void>("/api/sessions", { method: "DELETE" });
