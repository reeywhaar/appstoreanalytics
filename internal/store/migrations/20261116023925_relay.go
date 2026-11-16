package migrations

// The mail relay messages are handed to: one, or none. Its password is kept as typed, beside
// everything else this instance knows, which is the same place the sign-in that changes it is.
var relay = Migration{
	Name: "20261116023925_relay",
	Up: exec(`
CREATE TABLE smtp (
  singleton    INTEGER PRIMARY KEY CHECK (singleton = 1),
  host         TEXT NOT NULL,
  port         INTEGER NOT NULL,
  security     TEXT NOT NULL,
  username     TEXT NOT NULL DEFAULT '',
  password     TEXT NOT NULL DEFAULT '',
  from_address TEXT NOT NULL,
  from_name    TEXT NOT NULL DEFAULT ''
);
`),
}
