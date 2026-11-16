/** What the API answers with. See docs/api.md. */

export type Me = { username: string };

export type App = { id: string; name: string; bundle_id: string };

/** The days a report covers, and the as many days before them it is compared with. */
export type Span = {
  from: string;
  to: string;
  previous_from: string;
  app: string;
};

export const metrics = [
  "first_downloads",
  "proceeds",
  "purchases",
  "redownloads",
  "updates",
  "impressions",
  "page_views",
  "installs",
  "deletions",
  "sessions",
  "crashes",
] as const;

/** A measure; proceeds are in US dollars and everything else is a count. */
export type Metric = (typeof metrics)[number];

type Measures = Partial<Record<Metric, number>>;

/** A day: a measure missing from it is not known yet, which is not zero. */
export type Day = { day: string } & Measures;

/** A row of a breakdown: a country, a source, an app or an in-app purchase. */
export type By = { id: string; name: string; app_name?: string } & Measures;

export type Report = Span & {
  days: Day[];
  totals: Measures;
  /** How many days each total is of. */
  covered: Measures;
  previous: Measures;
  previous_covered: Measures;
  countries: By[];
  sources: By[];
  apps: By[];
  in_app: By[];
};

/** Every day measured, up to the day a range ends on, for charts to scroll back through. */
export type History = { from: string; to: string; app: string; days: Day[] };

/** The mail relay, as the form sees it: the password never comes back, only whether there is one. */
export type Relay = {
  configured: boolean;
  host: string;
  port: number;
  security: "starttls" | "implicit";
  username: string;
  password_set: boolean;
  from_address: string;
  from_name: string;
};

/** One sign-in. */
export type Session = {
  id: string;
  current: boolean;
  created_at: number;
  last_seen_at: number;
  expires_at: number;
  /** A guess at the browser and platform, from user_agent. */
  device: string;
  user_agent: string;
};

export type Part = {
  ran_at: number | null;
  ok_at: number | null;
  error: string;
};

/** Where an app's analytics reports stand with Apple. */
export type AppRequests = {
  app_id: string;
  name: string;
  /** "running", "stopped" when Apple stopped it, or "" when none was asked for. */
  ongoing: "" | "running" | "stopped";
  snapshot: boolean;
  /** When first asked for, or first seen when asked for elsewhere: Unix seconds. */
  since: number;
  /** How many report instances Apple lists, and the latest one's processing date. */
  delivered: number;
  latest: string;
  read: number;
};

export type Status = {
  running: boolean;
  requests: AppRequests[];
  /** The part of the sync under way, and "" when none is. */
  reading: "" | "apps" | "analytics" | "rates" | "sales";
  apps: { count: number; part: Part };
  analytics: {
    first: string;
    last: string;
    instances: number;
    /** How many apps Apple has been asked to make reports of, and since when. */
    requested_apps: number;
    requested_since: number | null;
    part: Part;
  };
  /** Sales reports, which fill in what analytics reports have not said yet; on when a vendor number is set. */
  sales: { on: boolean; first: string; last: string; days: number; part: Part };
  rates: { at: number | null; part: Part };
};
