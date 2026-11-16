package migrations

// What webhooks delivered. The URL's token and which events are mailed, and to whom, are kept in
// meta: one of each.
var hooks = Migration{
	Name: "20261116024912_hooks",
	Up: exec(`
CREATE TABLE hooks (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  received_at  INTEGER NOT NULL,
  event        TEXT NOT NULL,
  summary      TEXT NOT NULL,
  content_type TEXT NOT NULL,
  body         BLOB NOT NULL,
  -- When it was handed to the relay, and why it could not be, for one that was to be mailed.
  mailed_at    INTEGER,
  mail_error   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX hooks_received ON hooks (received_at);
`),
}
