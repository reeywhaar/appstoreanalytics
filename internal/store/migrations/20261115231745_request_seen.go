package migrations

// The most instances a report request has ever listed, so a snapshot that never delivered
// anything can be told from one whose old instances Apple has since deleted.
var requestSeen = Migration{
	Name: "20261115231745_request_seen",
	Up: exec(`
ALTER TABLE report_requests ADD COLUMN seen INTEGER NOT NULL DEFAULT 0;
UPDATE report_requests SET seen = delivered;
`),
}
