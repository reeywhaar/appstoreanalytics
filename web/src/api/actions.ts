import { query, request } from "@app/api/transport";
import type {
  App,
  Delivery,
  DeliveryBody,
  History,
  HookMail,
  Hooks,
  HookSender,
  Me,
  NewHookEndpoint,
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

/** Signs in to the relay in the form, saved or not, and sends nothing. */
export const postSettingsRelayCheck = (body: {
  host: string;
  port: number;
  security: string;
  username: string;
  password: string;
}) => request<void>("/api/settings/relay/check", { method: "POST", body });

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

export const getHooks = () => request<Hooks>("/api/hooks");

export const getHooksDeliveries = () =>
  request<{ deliveries: Delivery[] }>("/api/hooks/deliveries");

export const getHooksById = (id: number) =>
  request<DeliveryBody>(`/api/hooks/${id}`);

/** An empty secret is generated. */
export const postHooksEndpoints = (body: { label: string; secret: string }) =>
  request<NewHookEndpoint>("/api/hooks/endpoints", { method: "POST", body });

/** Asks Apple to send a test to whatever in App Store Connect sends to the address. */
export const postHooksEndpointsByIdTest = (id: number) =>
  request<{ senders: HookSender[] }>(`/api/hooks/endpoints/${id}/test`, {
    method: "POST",
  });

export const deleteHooksEndpointsById = (id: number) =>
  request<void>(`/api/hooks/endpoints/${id}`, { method: "DELETE" });

export const postHooksMails = (body: { to: string; events: string[] }) =>
  request<HookMail>("/api/hooks/mails", { method: "POST", body });

export const putHooksMailsById = (
  id: number,
  body: { to: string; events: string[] },
) => request<HookMail>(`/api/hooks/mails/${id}`, { method: "PUT", body });

export const deleteHooksMailsById = (id: number) =>
  request<void>(`/api/hooks/mails/${id}`, { method: "DELETE" });
