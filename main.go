// Command appstoreanalytics shows an App Store Connect account's analytics: downloads, proceeds
// and how the App Store pages did.
package main

import (
	"os"
	"time"
	// Apple's report days are Pacific days, and the image has no zone files of its own.
	_ "time/tzdata"

	"appstoreanalytics/internal/cli"
)

func main() {
	// Everything stored and logged is UTC.
	time.Local = time.UTC
	os.Exit(cli.Execute(os.Args[1:]))
}
