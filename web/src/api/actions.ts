import { query, request } from "@app/api/transport";
import type { Analytics, App, Me, Sales, Status } from "@app/api/types";

/** Named mechanically from the route, so a call site and a handler find each other by grep. */

export const postAuthLogin = (body: { username: string; password: string }) =>
  request<void>("/api/auth/login", { method: "POST", body });

export const postAuthLogout = () =>
  request<void>("/api/auth/logout", { method: "POST" });

export const getAuthMe = () => request<Me>("/api/auth/me");

export const getApps = () => request<App[]>("/api/apps");

/** What a report is asked for: so many days up to the last one known, of one app or all. */
export type Ask = { days: number; app?: string };

export const getSales = ({ days, app }: Ask, signal?: AbortSignal) =>
  request<Sales>(`/api/sales${query({ days, app })}`, { signal });

export const getAnalytics = ({ days, app }: Ask, signal?: AbortSignal) =>
  request<Analytics>(`/api/analytics${query({ days, app })}`, { signal });

export const getStatus = () => request<Status>("/api/status");

export const postSync = () =>
  request<{ started: boolean }>("/api/sync", { method: "POST" });
