package hooks

// Kind is an event Apple says it sends, and what it means.
type Kind struct {
	Name    string `json:"name"`
	Meaning string `json:"meaning"`
}

// Group is kinds of event that go together: a subscription's, a refund's.
type Group struct {
	Name  string `json:"name"`
	Kinds []Kind `json:"kinds"`
}

// Service is one of Apple's services that sends events, and its kinds, grouped.
type Service struct {
	Name   string  `json:"name"`
	Groups []Group `json:"groups"`
}

// Catalog is every kind of event Apple documents, as of November 2026, for choosing what to be
// mailed of before any has arrived. A notification type covers its subtypes, which arrive as
// TYPE/SUBTYPE. See docs/app-store-connect.md for where each list comes from; the groups are
// this program's, to choose a handful at a time.
var Catalog = []Service{
	{Name: "App Store Server Notifications", Groups: []Group{
		{Name: "Subscriptions", Kinds: []Kind{
			{"SUBSCRIBED", "A customer subscribed: an initial buy, or a resubscribe."},
			{"DID_RENEW", "A subscription renewed, including after a billing problem was fixed."},
			{"DID_FAIL_TO_RENEW", "A renewal's billing failed, and billing retry started."},
			{"DID_CHANGE_RENEWAL_STATUS", "Auto-renew was turned on or off."},
			{"DID_CHANGE_RENEWAL_PREF", "A customer changed plan: an upgrade, a downgrade, a quantity."},
			{"OFFER_REDEEMED", "An active subscriber redeemed an offer."},
			{"PRICE_INCREASE", "A customer was told of a price increase, or accepted it."},
			{"EXPIRED", "A subscription expired."},
			{"GRACE_PERIOD_EXPIRED", "A grace period ended without a renewal."},
			{"RENEWAL_EXTENDED", "One subscription's renewal date was extended."},
			{"RENEWAL_EXTENSION", "A renewal-date extension for many subscribers finished or failed."},
		}},
		{Name: "Purchases and refunds", Kinds: []Kind{
			{"ONE_TIME_CHARGE", "A consumable, non-consumable or non-renewing subscription was bought."},
			{"REFUND", "A refund was granted."},
			{"REFUND_DECLINED", "A refund request was declined."},
			{"REFUND_REVERSED", "An earlier refund was reversed after a dispute."},
			{"CONSUMPTION_REQUEST", "A customer asked for a refund, and Apple wants consumption data."},
		}},
		{Name: "Sharing and consent", Kinds: []Kind{
			{"REVOKE", "Access through Family Sharing or a group was lost."},
			{"UNCLAIM_SEATS", "Seats were unclaimed in a group subscription."},
			{"RESCIND_CONSENT", "A parent or guardian withdrew consent for a child's use of the app."},
		}},
		{Name: "Advanced Commerce", Kinds: []Kind{
			{"METADATA_UPDATE", "A subscription's metadata changed."},
			{"MIGRATION", "A subscription was migrated."},
			{"PRICE_CHANGE", "A subscription's price was changed."},
		}},
		{Name: "Other", Kinds: []Kind{
			{"EXTERNAL_PURCHASE_TOKEN", "An external purchase token was created, or is to be reported."},
			{"TEST", "A test notification, asked for through the App Store Server API."},
		}},
	}},
	{Name: "App Store Connect webhooks", Groups: []Group{
		{Name: "Versions and builds", Kinds: []Kind{
			{"appStoreVersionAppVersionStateUpdated", "An App Store version changed state, such as to Ready for Sale."},
			{"buildUploadStateUpdated", "A build's upload changed state."},
			{"buildBetaDetailExternalBuildStateUpdated", "A build's external beta state changed."},
		}},
		{Name: "TestFlight feedback", Kinds: []Kind{
			{"betaFeedbackScreenshotSubmissionCreated", "A tester sent feedback with a screenshot."},
			{"betaFeedbackCrashSubmissionCreated", "A tester sent a crash report."},
		}},
		{Name: "Asset packs", Kinds: []Kind{
			{"backgroundAssetVersionStateUpdated", "An asset pack's processing state changed."},
			{"backgroundAssetVersionInternalBetaReleaseCreated", "An asset pack passed validation."},
			{"backgroundAssetVersionExternalBetaReleaseStateUpdated", "An asset pack's beta review finished."},
			{"backgroundAssetVersionAppStoreReleaseStateUpdated", "An asset pack's App Store review finished."},
		}},
		{Name: "Alternative distribution", Kinds: []Kind{
			{"alternativeDistributionPackageVersionCreated", "A new app version is available."},
			{"alternativeDistributionPackageAvailableUpdated", "A version is to be removed."},
			{"alternativeDistributionTerritoryAvailabilityUpdated", "Every version is to be removed."},
		}},
		{Name: "Other", Kinds: []Kind{
			{"webhookPingCreated", "A test sent with the Test button, as delivered."},
		}},
	}},
}
