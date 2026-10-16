package entities

import "time"

// OTAAsset is one file of an over-the-air update (the JS bundle or an
// image/font it references), described the way the Expo Updates protocol
// needs it: Path is relative to the update's own directory, Hash is the
// base64url SHA-256 the client verifies after download, Key is a stable
// content-derived identifier the client caches by.
type OTAAsset struct {
	Path          string
	Hash          string
	Key           string
	ContentType   string
	FileExtension string
}

// OTAUpdate is one published release of the mobile app's JS for a single
// runtime version and platform — the output of `expo export`, as stored by
// the publish script (see `make ota-publish`). A rollback marker is also an
// OTAUpdate, with IsRollback set and no assets: it tells clients to go
// back to the JS embedded in their binary.
type OTAUpdate struct {
	// ID is a UUID derived from the export's metadata.json, so republishing
	// the exact same export yields the same ID (and no redundant download).
	ID             string
	Name           string // the publish directory name (a unix timestamp)
	RuntimeVersion string
	Platform       string
	CreatedAt      time.Time
	IsRollback     bool
	LaunchAsset    OTAAsset
	Assets         []OTAAsset
	// ExpoConfig is the app's public Expo config at publish time, as raw
	// JSON — passed through to clients as manifest.extra.expoClient
	// (Constants.expoConfig reads it). May be nil.
	ExpoConfig []byte
}
