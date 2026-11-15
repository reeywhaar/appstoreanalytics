/** What the API answers with. See docs/api.md. */

export type Me = { username: string };

export type App = { id: string; name: string; bundle_id: string; sku: string };

/** The days a report covers, and the as many days before them it is compared with. */
export type Span = {
  from: string;
  to: string;
  previous_from: string;
  app: string;
};

export type SalesDay = {
  day: string;
  downloads: number;
  redownloads: number;
  updates: number;
  in_app: number;
  proceeds: number;
};

export type SalesTotals = Omit<SalesDay, "day"> & { days: number };

export type SalesBy = {
  id: string;
  name: string;
  downloads: number;
  units: number;
  proceeds: number;
  /** The app an in-app purchase is bought in. */
  app_name?: string;
};

export type Sales = Span & {
  days: SalesDay[];
  totals: SalesTotals;
  previous: SalesTotals;
  countries: SalesBy[];
  apps: SalesBy[];
  in_app: SalesBy[];
  unconverted: string[];
};

export const metrics = [
  "impressions",
  "page_views",
  "first_downloads",
  "redownloads",
  "installs",
  "deletions",
  "sessions",
  "crashes",
] as const;

export type Metric = (typeof metrics)[number];

/** A day of analytics: a measure missing from it is not known yet, which is not zero. */
export type AnalyticsDay = { day: string } & Partial<Record<Metric, number>>;

export type Analytics = Span & {
  days: AnalyticsDay[];
  totals: Partial<Record<Metric, number>>;
  covered: Partial<Record<Metric, number>>;
  previous: Partial<Record<Metric, number>>;
  previous_covered: Partial<Record<Metric, number>>;
  sources: ({ source: string } & Partial<Record<Metric, number>>)[];
};

export type Part = {
  ran_at: number | null;
  ok_at: number | null;
  error: string;
};

export type Status = {
  running: boolean;
  /** The part of the sync under way, and "" when none is. */
  reading: "" | "apps" | "rates" | "sales" | "analytics";
  apps: { count: number; part: Part };
  sales: { first: string; last: string; days: number; part: Part };
  analytics: { first: string; last: string; instances: number; part: Part };
  rates: { at: number | null; part: Part };
};
