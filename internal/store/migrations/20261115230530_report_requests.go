package migrations

// The report requests made of Apple, and what each has delivered, so the status page can say
// per app what was asked for, since when, and whether Apple has made anything yet.
var reportRequests = Migration{
	Name: "20261115230530_report_requests",
	Up: exec(`
CREATE TABLE report_requests (
  id          TEXT PRIMARY KEY,
  app_id      TEXT NOT NULL,
  access_type TEXT NOT NULL,
  stopped     INTEGER NOT NULL,
  -- When it was asked for here, or first seen when it was asked for elsewhere: Apple does not
  -- say when a request was made.
  since       INTEGER NOT NULL,
  -- What Apple listed at the last look: instances of the reports read, and the latest
  -- processing date among them.
  delivered   INTEGER NOT NULL DEFAULT 0,
  latest      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX report_requests_app ON report_requests (app_id);
`),
}
