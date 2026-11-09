package historypolicy

// A session records several different things and has one budget for them. The
// measurement behind docs/plans/session-record-policy.md found what that costs:
// five sessions all sat at the 100MiB ceiling, 75-92% of each one's history had
// already been deleted to make room, and the conversation text in it was
// 4-15KiB. What filled the budget was a model catalog recorded 33,416 times for
// 13 distinct values, a quota payload whose rendered field alone was 2.48KiB an
// event, a command stored three times and its output twice, and verbatim copies
// of the provider replies that produced all of it.
//
// So a category is what a record means, not which wire kind carried it. One
// kind splits where it carries more than one meaning: `usage` is an account's
// remaining quota in one subtype and a turn's token spend in another, and a
// tool call is a name and a target alongside arguments that can be megabytes.
//
// This file is the contract those categories are named by. The policy a
// category is given, and the reader's view of what is no longer there, are
// below. See cxz#336.
type Category string

const (
	// What was said. Never removed because another category went over.
	CategoryConversation Category = "conversation"
	// What a turn spent. A fact about that turn, not a current value, so it is
	// never overwritten by the latest state.
	CategoryTurnMetrics Category = "turn_metrics"
	// That a tool ran, on what, and how it ended. Survives its own detail.
	CategoryWorkSummary Category = "work_summary"
	// Arguments, output and results. The largest honest category: 20-27MiB a
	// session in the measurement.
	CategoryWorkDetail Category = "work_detail"
	CategoryWorkDiff   Category = "work_diff"
	// An account's remaining quota, kept as the latest value per window.
	CategoryAccountLimits Category = "account_limits"
	CategoryContextState  Category = "context_state"
	CategoryModelCatalog  Category = "model_catalog"
	// Whether a thing is being polled, is unsupported, failed.
	CategoryAvailability Category = "availability"
	// Values a provider repeats on every call -- the decisions available on an
	// approval, for one -- which are constants wearing an event's clothes.
	CategoryProviderConstant Category = "provider_constant"
	// The verbatim stream that produced the conversation.
	CategoryProviderStream Category = "provider_stream"
	// The verbatim replies that produced the normalized state. 80-99% of the
	// raw that survives retention, and a second copy of what the normalized
	// events already say.
	CategoryProviderReply Category = "provider_reply"
	CategoryDiagnostic    Category = "diagnostic"
	// What a restart, a duplicate command, or an unanswered question needs.
	// Not subject to storage: see Control.
	CategoryControl Category = "control"
)

// Categories is every category, in the order the policy and the settings
// surface present them: what was said first, what it cost last.
var Categories = []Category{
	CategoryConversation, CategoryTurnMetrics, CategoryWorkSummary,
	CategoryWorkDetail, CategoryWorkDiff, CategoryAccountLimits,
	CategoryContextState, CategoryModelCatalog, CategoryAvailability,
	CategoryProviderConstant, CategoryProviderStream, CategoryProviderReply,
	CategoryDiagnostic, CategoryControl,
}

// Storage is where a category's records are kept.
type Storage string

const (
	// On disk, in the session's own file.
	StorageDisk Storage = "disk"
	// In the supervisor's bounded memory: readable while it runs, gone when it
	// restarts, and never copied to disk behind the policy's back.
	StorageMemory Storage = "memory"
	// Not recorded. A reader is told that, rather than being handed an empty
	// body to interpret.
	StorageOff Storage = "off"
)

// Keep is whether a category's past matters.
type Keep string

const (
	// Every occurrence, bounded by bytes and count.
	KeepHistory Keep = "history"
	// The present value per semantic key, bounded by the number of keys. A
	// poll that answers what is already recorded does not add to it -- and
	// `unchanged` is decided on meaning, because cxz stamps a quota payload
	// with the instant it was read and no two are equal as bytes (cxz#344).
	KeepLatest Keep = "latest"
)

// Control reports whether a category holds what a restart or a duplicate
// command needs. Those records are not subject to storage: a request to keep
// them in memory, or not at all, is refused rather than honoured, because the
// cost of honouring it is a session that cannot come back or a command that
// runs twice.
func Control(c Category) bool { return c == CategoryControl }

// TurnBoundary reports whether removing from this category has to wait for a
// completed turn.
//
// Trimming the conversation does: what is left has to be something a session
// can resume from. Expiring one tool result does not, and neither does folding
// a catalog into its latest value -- but today all three wait behind the same
// condition (internal/supervisor/history.go), so a long turn holds up the
// removal of the things that cost the most and need no boundary at all.
func TurnBoundary(c Category) bool { return c == CategoryConversation }

// Availability is what a reader is told about a body it cannot see. Absence is
// never left to be inferred from an empty payload or a gap in the sequence.
type Availability string

const (
	AvailabilityAvailable Availability = "available"
	// Recorded, then removed by a budget. The original size, the category and
	// when it went are answered with it.
	AvailabilityExpired Availability = "expired"
	// The policy said off when this happened.
	AvailabilityNotRecorded Availability = "not_recorded"
	// It was in memory and the supervisor restarted.
	AvailabilityVolatileLost Availability = "volatile_lost"
)
