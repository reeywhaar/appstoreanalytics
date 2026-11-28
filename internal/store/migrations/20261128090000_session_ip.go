package migrations

// A session keeps the address it was last seen from, beside its browser: two sessions of the same
// browser are told apart by where they are, and one from somewhere unexpected stands out.
var sessionIP = Migration{
	Name: "20261128090000_session_ip",
	Up: exec(`
ALTER TABLE sessions ADD COLUMN ip TEXT NOT NULL DEFAULT '';
`),
}
