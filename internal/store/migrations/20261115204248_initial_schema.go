package migrations

// Sign-ins, and what is read from App Store Connect.
//
// A report's day is TEXT, YYYY-MM-DD: Apple's reports are by calendar day, in its own zone,
// and a day is not a moment. See docs/conventions.md.
var initialSchema = Migration{
	Name: "20261115204248_initial_schema",
	Up: exec(`
-- What the process needs to remember between runs that has no table of its own.
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE sessions (
  id_hash      BLOB PRIMARY KEY,
  user_agent   TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  expires_at   INTEGER NOT NULL
);
CREATE INDEX sessions_expiry ON sessions (expires_at);

-- Apple's own ids: an app's is the number in its App Store link.
CREATE TABLE apps (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  bundle_id  TEXT NOT NULL DEFAULT '',
  sku        TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
);

-- Everything a sales report names: apps, and what is bought inside them.
CREATE TABLE products (
  id       TEXT PRIMARY KEY,
  -- '' when the app it belongs to could not be told.
  app_id   TEXT NOT NULL,
  title    TEXT NOT NULL,
  sku      TEXT NOT NULL,
  kind     TEXT NOT NULL,
  seen_day TEXT NOT NULL
);

-- The days whose sales report has been read. A day with no row is not known, which is not the
-- same as a day with no sales.
CREATE TABLE sales_days (
  day        TEXT PRIMARY KEY,
  rows       INTEGER NOT NULL,
  fetched_at INTEGER NOT NULL
);

CREATE TABLE sales (
  day          TEXT NOT NULL,
  app_id       TEXT NOT NULL,
  product_id   TEXT NOT NULL,
  product_type TEXT NOT NULL,
  kind         TEXT NOT NULL,
  country      TEXT NOT NULL,
  currency     TEXT NOT NULL,
  units        INTEGER NOT NULL,
  -- Millionths of the currency, so a sum is exact.
  proceeds     INTEGER NOT NULL,
  PRIMARY KEY (day, product_id, product_type, country, currency)
) WITHOUT ROWID;
CREATE INDEX sales_app ON sales (app_id, day);

-- How many of each currency a US dollar buys.
CREATE TABLE rates (
  currency   TEXT PRIMARY KEY,
  per_usd    REAL NOT NULL,
  fetched_at INTEGER NOT NULL
);

-- The analytics report instances that have been read, so none is read twice.
CREATE TABLE analytics_instances (
  id              TEXT PRIMARY KEY,
  app_id          TEXT NOT NULL,
  report          TEXT NOT NULL,
  processing_date TEXT NOT NULL,
  rows            INTEGER NOT NULL,
  read_at         INTEGER NOT NULL
);

-- An analytics measure for a day, by where it happened and where the visitor came from.
CREATE TABLE metrics (
  app_id    TEXT NOT NULL,
  day       TEXT NOT NULL,
  metric    TEXT NOT NULL,
  territory TEXT NOT NULL,
  source    TEXT NOT NULL,
  value     INTEGER NOT NULL,
  PRIMARY KEY (app_id, day, metric, territory, source)
) WITHOUT ROWID;
CREATE INDEX metrics_day ON metrics (day, metric);

-- How each part of a sync last went.
CREATE TABLE sync_state (
  part   TEXT PRIMARY KEY,
  ran_at INTEGER NOT NULL,
  ok_at  INTEGER,
  error  TEXT NOT NULL DEFAULT ''
);
`),
}
