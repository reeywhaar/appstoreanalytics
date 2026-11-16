package migrations

// A sales report's purchase on a day the purchases report has processed goes: from the first day
// an app's purchases report said anything of to the last, a day it says nothing of had no
// purchases, since it counts every one, and a sales report dates a purchase a day before it does,
// so the two counted one purchase twice. The other reports leave small counts out, and a sales
// report's count of a day they say nothing of stays.
var salesInsideAnalytics = Migration{
	Name: "20261116180204_sales_inside_analytics",
	Up: exec(`
WITH report (metric, name) AS (VALUES
  ('purchases', 'purchases'), ('proceeds', 'purchases')
),
span (app_id, name, first, last) AS (
  SELECT m.app_id, r.name, MIN(m.day), MAX(m.day)
    FROM metrics m JOIN report r ON r.metric = m.metric
   WHERE m.origin = 'analytics'
   GROUP BY m.app_id, r.name
)
DELETE FROM metrics
 WHERE origin = 'sales'
   AND EXISTS (SELECT 1 FROM span s JOIN report r ON r.name = s.name
                WHERE r.metric = metrics.metric AND s.app_id = metrics.app_id
                  AND metrics.day BETWEEN s.first AND s.last);
`),
}
