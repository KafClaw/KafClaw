package skills

// BundledSkill describes a built-in skill shipped with KafClaw.
type BundledSkill struct {
	Name           string
	DefaultEnabled bool
}

// BundledCatalog is the baseline bundled skill set.
var BundledCatalog = []BundledSkill{
	{Name: "skill-creator", DefaultEnabled: false},
	{Name: "session-logs", DefaultEnabled: false},
	{Name: "summarize", DefaultEnabled: false},
	{Name: "github", DefaultEnabled: false},
	{Name: "gh-issues", DefaultEnabled: false},
	{Name: "weather", DefaultEnabled: false},
	{Name: "google-cli", DefaultEnabled: false},
	{Name: "google-workspace", DefaultEnabled: false},
	{Name: "m365", DefaultEnabled: false},
	{Name: "incident-comms", DefaultEnabled: false},
	{Name: "channel-onboarding", DefaultEnabled: true},
}
