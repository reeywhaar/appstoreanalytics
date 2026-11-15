package migrations

// Analytics reports are what is read, and sales reports fill in what they have not said yet. A
// measure says what was bought and where it came from, and what the sales reports already read
// said becomes measures of that kind, so no day is read again for it. See
// docs/app-store-connect.md.
var analyticsWithSales = Migration{
	Name: "20261115233727_analytics_with_sales",
	Up: exec(`
-- A measure of a day, by where it happened, where the visitor came from and what was bought.
-- Proceeds are in millionths of a US dollar, so a sum is exact; everything else is a count.
CREATE TABLE metrics_new (
  app_id    TEXT NOT NULL,
  day       TEXT NOT NULL,
  metric    TEXT NOT NULL,
  territory TEXT NOT NULL,
  source    TEXT NOT NULL,
  item      TEXT NOT NULL,
  value     INTEGER NOT NULL,
  -- 'analytics', or 'sales' for a placeholder until analytics says the same app's day.
  origin    TEXT NOT NULL,
  PRIMARY KEY (app_id, day, metric, territory, source, item)
) WITHOUT ROWID;
INSERT INTO metrics_new
  SELECT app_id, day, metric, territory, source, '', value, 'analytics' FROM metrics;
DROP TABLE metrics;
ALTER TABLE metrics_new RENAME TO metrics;
CREATE INDEX metrics_day ON metrics (day, metric);

-- What is bought, named by Apple's identifier for it: an app, or something inside one.
CREATE TABLE items (
  id     TEXT PRIMARY KEY,
  app_id TEXT NOT NULL,
  name   TEXT NOT NULL,
  in_app INTEGER NOT NULL
);
INSERT INTO items SELECT id, app_id, title, kind = 'in_app' FROM products;

-- What the sales reports read so far counted.
INSERT INTO metrics (app_id, day, metric, territory, source, item, value, origin)
  SELECT app_id, day, metric, country, '', item, SUM(units), 'sales'
    FROM (SELECT app_id, day, country, units,
                 CASE kind WHEN 'download' THEN 'first_downloads' WHEN 'redownload' THEN 'redownloads'
                           WHEN 'update' THEN 'updates' ELSE 'purchases' END AS metric,
                 CASE WHEN kind = 'in_app' THEN product_id ELSE '' END AS item
            FROM sales WHERE kind IN ('download', 'redownload', 'update', 'in_app')) s
   WHERE NOT EXISTS (SELECT 1 FROM metrics m WHERE m.app_id = s.app_id AND m.day = s.day
                       AND m.metric = s.metric AND m.origin = 'analytics')
   GROUP BY app_id, day, metric, country, item
  HAVING SUM(units) != 0;

-- And what it earned, in dollars at the rates already read; a currency with none is left out.
INSERT INTO metrics (app_id, day, metric, territory, source, item, value, origin)
  SELECT app_id, day, 'proceeds', country, '', product_id, CAST(ROUND(SUM(usd)) AS INTEGER), 'sales'
    FROM (SELECT s.app_id, s.day, s.country, s.product_id,
                 s.proceeds * (CASE WHEN s.currency = 'USD' THEN 1.0 ELSE 1.0 / r.per_usd END) AS usd
            FROM sales s LEFT JOIN rates r ON r.currency = s.currency
           WHERE s.proceeds != 0 AND (s.currency = 'USD' OR r.per_usd > 0)) s
   WHERE NOT EXISTS (SELECT 1 FROM metrics m WHERE m.app_id = s.app_id AND m.day = s.day
                       AND m.metric = 'proceeds' AND m.origin = 'analytics')
   GROUP BY app_id, day, country, product_id
  HAVING ROUND(SUM(usd)) != 0;

DROP TABLE sales;
DROP TABLE products;

-- Read again, for the measures they were not read for before. What they said stays until then.
DELETE FROM analytics_instances;
`),
}
