package migrations

// Which addresses somebody is mailed from, beside which kinds of event: a JSON array of their
// ids, where none is every address, those added later too, which is what everybody was mailed
// from before.
var hookMailEndpoints = Migration{
	Name: "20261117053003_hook_mail_endpoints",
	Up: exec(`
ALTER TABLE hook_mails ADD COLUMN endpoints TEXT NOT NULL DEFAULT '[]';
`),
}
