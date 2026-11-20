package migrations

// The day index carries each measure's value, so a range of days of every app is summed from the
// index alone; without it every row in the range costs a second lookup in the table, which for a
// year of an account with many countries is seconds. The primary key's columns are in each entry
// already, the table being WITHOUT ROWID.
var metricsDayValue = Migration{
	Name: "20261120010613_metrics_day_value",
	Up: exec(`
DROP INDEX metrics_day;
CREATE INDEX metrics_day ON metrics (day, metric, value);
`),
}
