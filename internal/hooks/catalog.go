package hooks

// Kind is an event Apple says it sends, what to call it, and what it means.
type Kind struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
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
			{"SUBSCRIBED", "Subscribed", "A customer subscribed: an initial buy, or a resubscribe."},
			{"DID_RENEW", "Renewed", "A subscription renewed, including after a billing problem was fixed."},
			{"DID_FAIL_TO_RENEW", "Renewal failed", "A renewal's billing failed, and billing retry started."},
			{"DID_CHANGE_RENEWAL_STATUS", "Auto-renew changed", "Auto-renew was turned on or off."},
			{"DID_CHANGE_RENEWAL_PREF", "Plan changed", "A customer changed plan: an upgrade, a downgrade, a quantity."},
			{"OFFER_REDEEMED", "Offer redeemed", "An active subscriber redeemed an offer."},
			{"PRICE_INCREASE", "Price increase", "A customer was told of a price increase, or accepted it."},
			{"EXPIRED", "Expired", "A subscription expired."},
			{"GRACE_PERIOD_EXPIRED", "Grace period ended", "A grace period ended without a renewal."},
			{"RENEWAL_EXTENDED", "Renewal extended", "One subscription's renewal date was extended."},
			{"RENEWAL_EXTENSION", "Renewal extension for many", "A renewal-date extension for many subscribers finished or failed."},
		}},
		{Name: "Purchases and refunds", Kinds: []Kind{
			{"ONE_TIME_CHARGE", "Purchase", "A consumable, non-consumable or non-renewing subscription was bought."},
			{"REFUND", "Refunded", "A refund was granted."},
			{"REFUND_DECLINED", "Refund declined", "A refund request was declined."},
			{"REFUND_REVERSED", "Refund reversed", "An earlier refund was reversed after a dispute."},
			{"CONSUMPTION_REQUEST", "Refund requested", "A customer asked for a refund, and Apple wants consumption data."},
		}},
		{Name: "Sharing and consent", Kinds: []Kind{
			{"REVOKE", "Access revoked", "Access through Family Sharing or a group was lost."},
			{"UNCLAIM_SEATS", "Seats unclaimed", "Seats were unclaimed in a group subscription."},
			{"RESCIND_CONSENT", "Consent withdrawn", "A parent or guardian withdrew consent for a child's use of the app."},
		}},
		{Name: "Advanced Commerce", Kinds: []Kind{
			{"METADATA_UPDATE", "Metadata changed", "A subscription's metadata changed."},
			{"MIGRATION", "Migrated", "A subscription was migrated."},
			{"PRICE_CHANGE", "Price changed", "A subscription's price was changed."},
		}},
		{Name: "Other", Kinds: []Kind{
			{"EXTERNAL_PURCHASE_TOKEN", "External purchase token", "An external purchase token was created, or is to be reported."},
			{"TEST", "Test notification", "A test notification, asked for through the App Store Server API."},
		}},
	}},
	{Name: "App Store Connect webhooks", Groups: []Group{
		{Name: "Versions and builds", Kinds: []Kind{
			{"appStoreVersionAppVersionStateUpdated", "App version state changed", "An App Store version changed state, such as to Ready for Sale."},
			{"buildUploadStateUpdated", "Build upload state changed", "A build's upload changed state."},
			{"buildBetaDetailExternalBuildStateUpdated", "Beta build state changed", "A build's external beta state changed."},
		}},
		{Name: "TestFlight feedback", Kinds: []Kind{
			{"betaFeedbackScreenshotSubmissionCreated", "Tester feedback", "A tester sent feedback with a screenshot."},
			{"betaFeedbackCrashSubmissionCreated", "Tester crash report", "A tester sent a crash report."},
		}},
		{Name: "Asset packs", Kinds: []Kind{
			{"backgroundAssetVersionStateUpdated", "Asset pack state changed", "An asset pack's processing state changed."},
			{"backgroundAssetVersionInternalBetaReleaseCreated", "Asset pack validated", "An asset pack passed validation."},
			{"backgroundAssetVersionExternalBetaReleaseStateUpdated", "Asset pack beta review finished", "An asset pack's beta review finished."},
			{"backgroundAssetVersionAppStoreReleaseStateUpdated", "Asset pack App Store review finished", "An asset pack's App Store review finished."},
		}},
		{Name: "Alternative distribution", Kinds: []Kind{
			{"alternativeDistributionPackageVersionCreated", "Alternative distribution version available", "A new app version is available."},
			{"alternativeDistributionPackageAvailableUpdated", "Alternative distribution version to be removed", "A version is to be removed."},
			{"alternativeDistributionTerritoryAvailabilityUpdated", "Alternative distribution versions to be removed", "Every version is to be removed."},
		}},
		{Name: "Other", Kinds: []Kind{
			{"webhookPingCreated", "Webhook test", "A test sent with the Test button, as delivered."},
		}},
	}},
}
