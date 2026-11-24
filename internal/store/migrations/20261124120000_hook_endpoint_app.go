package migrations

// An address made automatically for one of an app's parts says which: the app's id, and webhook,
// production or sandbox. Connecting that part automatically again finds it rather than making
// another. One added by hand has neither.
var hookEndpointApp = Migration{
	Name: "20261124120000_hook_endpoint_app",
	Up: exec(`
ALTER TABLE hook_endpoints ADD COLUMN app_id TEXT NOT NULL DEFAULT '';
ALTER TABLE hook_endpoints ADD COLUMN part TEXT NOT NULL DEFAULT '';
`),
}
