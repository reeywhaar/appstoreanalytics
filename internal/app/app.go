// Package app holds what this program is, as distinct from what it was told.
//
// The line against internal/config: whether an operator can change it without rebuilding.
package app

const Name = "appstoreanalytics"

// ListenAddr is fixed inside the container. Remap with `docker run -p 8080:80`.
const ListenAddr = ":80"

// WebDir is where the image puts the built bundle. Fixed by the image layout, so it is what
// this program is rather than what it was told.
const WebDir = "/srv/web"

// AppleAPI is App Store Connect's address.
const AppleAPI = "https://api.appstoreconnect.apple.com"

// StoreKitAPI and StoreKitSandboxAPI are the App Store Server API's, which sends a test
// notification to an app's production or sandbox URL by which of them is asked.
const (
	StoreKitAPI        = "https://api.storekit.itunes.apple.com"
	StoreKitSandboxAPI = "https://api.storekit-sandbox.itunes.apple.com"
)

// Version is stamped at link time:
//
//	-ldflags "-X appstoreanalytics/internal/app.Version=$(git rev-parse --short HEAD)"
var Version = "dev"
