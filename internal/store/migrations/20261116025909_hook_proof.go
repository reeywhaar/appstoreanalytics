package migrations

// Whether a delivery proved where it came from: by Apple's signature, by the secret set for App
// Store Connect webhooks, or not at all, and why one failed to.
var hookProof = Migration{
	Name: "20261116025909_hook_proof",
	Up: exec(`
ALTER TABLE hooks ADD COLUMN proven_by TEXT NOT NULL DEFAULT '';
ALTER TABLE hooks ADD COLUMN proof_error TEXT NOT NULL DEFAULT '';
`),
}
