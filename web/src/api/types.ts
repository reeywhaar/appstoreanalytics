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

/** Each app's own days, by app id and then by day: a day missing, or a measure missing from one,
 *  is none of it on a day the totals know. Only when every app is asked for. */
export type ByApp = Record<string, Record<string, Measures>>;

/** A day: a measure missing from it is not known yet, which is not zero. */
export type Day = { day: string } & Measures;

/** A row of a breakdown: a country, a source, an app or an in-app purchase. */
export type By = {
  id: string;
  name: string;
  app_name?: string;
  /** Each app's part of the row, by app id: with every app read, and not on the apps' own rows. */
  by_app?: Record<string, Measures>;
} & Measures;

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
  by_app: ByApp;
};

/** Every day measured, up to the day a range ends on, for charts to scroll back through. */
/** A year of the days measured, from `from` to `to`; `first` is the first day there is at all. */
export type History = {
  from: string;
  to: string;
  first: string;
  app: string;
  days: Day[];
  by_app: ByApp;
};

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

/** A kind of event a webhook has delivered: how often, and when last. */
export type HookEvent = { event: string; count: number; last_at: number };

/** One delivery, as a list shows it. */
export type Delivery = {
  id: number;
  received_at: number;
  /** The label of the address it came through, as it was then. */
  via: string;
  event: string;
  summary: string;
  size: number;
  /** Who it is proven to come from: "apple" by the signature chain, "secret" by the secret
   *  set; "" when it is not. */
  proven_by: "" | "apple" | "secret";
  /** Why its signature failed, "" when it did not, or it carried nothing to check. */
  proof_error: string;
  /** When it was handed to the relay, null for one not to be mailed. */
  mailed_at: number | null;
  /** Whom it was mailed to. */
  mailed_to: string[];
  /** Why the relay did not take it, to some or all, "" when it took it to all. */
  mail_error: string;
};

/** One delivery with its body, as it came and opened up. */
export type DeliveryBody = Delivery & {
  content_type: string;
  body: string;
  decoded: unknown;
};

/** Somebody mailed of webhooks, the kinds of event, and the addresses they are mailed from. */
export type HookMail = {
  id: number;
  to: string;
  events: string[];
  /** Address ids; none is every address, those added later too. */
  endpoints: number[];
};

/** A kind of event Apple documents, and what it means. */
export type HookKind = { name: string; title: string; meaning: string };

/** Kinds of event that go together: a subscription's, a refund's. */
export type HookGroup = { name: string; kinds: HookKind[] };

/** One of Apple's services that sends events, and its kinds, grouped. */
export type HookService = { name: string; groups: HookGroup[] };

/** An address webhooks are delivered to. */
export type HookEndpoint = {
  id: number;
  label: string;
  url: string;
  created_at: number;
  /** How many of the deliveries kept came through it, and when the last did. */
  count: number;
  last_at: number | null;
  /** False only for the address carried over from before there were several. */
  secret_set: boolean;
};

/**
 * One thing in App Store Connect that sends to an address, and what asking it for a test came
 * to: asked, turned off in App Store Connect, no In-App Purchase key to ask with, or failed.
 */
export type HookSender = {
  kind: "notifications" | "webhook";
  app: string;
  /** A webhook's name, or Production or Sandbox for an app's server notifications. */
  name: string;
  result: "asked" | "off" | "no_key" | "failed";
  message: string;
};

/** An address just added, with its secret: the one time that is shown. */
export type NewHookEndpoint = HookEndpoint & { secret: string };

export type Hooks = {
  endpoints: HookEndpoint[];
  mails: HookMail[];
  /** The kinds delivered so far. */
  events: HookEvent[];
  /** Every kind Apple documents, delivered or not. */
  catalog: HookService[];
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
