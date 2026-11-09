package historypolicy

import (
	"fmt"
	"sort"
)

// CategoryPolicy is what one category is given. A nil limit is unset and takes
// the default; a limit set to zero is unlimited. The two have to stay apart --
// a budget of 0 and a budget nobody wrote are opposite instructions, and a
// config format that cannot tell them apart silently turns one into the other.
type CategoryPolicy struct {
	Storage Storage `json:"storage,omitempty"`
	Keep    Keep    `json:"keep,omitempty"`
	// MiB and Events bound a `keep: history` category.
	MiB    *int `json:"mib,omitempty"`
	Events *int `json:"events,omitempty"`
	// Keys bounds a `keep: latest` category: how many distinct values it is
	// the present value of.
	Keys *int `json:"keys,omitempty"`
}

// RecordPolicy is the installation's policy for session records.
//
// Disabled turns every removal off, which is what the one switch in the policy
// it replaces did. It is not a per-category setting: a person who wants nothing
// removed does not want to say so fourteen times.
type RecordPolicy struct {
	Disabled   bool                        `json:"disabled,omitempty"`
	Categories map[Category]CategoryPolicy `json:"categories,omitempty"`
}

func ptr(n int) *int { return &n }

// defaults are the measured starting point, not a guess.
//
// `conversation` is 64MiB against a measured 4-15KiB of text, which is not
// generosity -- it is the category that must never be the one that gives way,
// so its budget is set where a budget stops being the thing that decides.
// `work_detail` is 32MiB against a measured 20-27MiB. `provider_reply` is off
// because it was 80-99% of the raw that survived and carried nothing the
// normalized events did not already say. See docs/plans/session-record-policy.md.
var defaults = map[Category]CategoryPolicy{
	CategoryConversation:     {StorageDisk, KeepHistory, ptr(64), ptr(200000), nil},
	CategoryTurnMetrics:      {StorageDisk, KeepHistory, ptr(8), ptr(50000), nil},
	CategoryWorkSummary:      {StorageDisk, KeepHistory, ptr(16), ptr(100000), nil},
	CategoryWorkDetail:       {StorageDisk, KeepHistory, ptr(32), ptr(50000), nil},
	CategoryWorkDiff:         {StorageDisk, KeepHistory, ptr(8), ptr(20000), nil},
	CategoryAccountLimits:    {StorageDisk, KeepLatest, nil, nil, ptr(64)},
	CategoryContextState:     {StorageDisk, KeepLatest, nil, nil, ptr(16)},
	CategoryModelCatalog:     {StorageDisk, KeepLatest, nil, nil, ptr(8)},
	CategoryAvailability:     {StorageDisk, KeepLatest, nil, nil, ptr(32)},
	CategoryProviderConstant: {StorageDisk, KeepLatest, nil, nil, ptr(16)},
	CategoryProviderStream:   {StorageDisk, KeepHistory, ptr(16), ptr(100000), nil},
	// Off, but bounded for the session that turns it on: a category with no
	// budget behind an off switch is an unbounded category one edit away.
	CategoryProviderReply: {StorageOff, KeepHistory, ptr(8), ptr(50000), nil},
	CategoryDiagnostic:    {StorageDisk, KeepHistory, ptr(4), ptr(20000), nil},
	CategoryControl:       {StorageDisk, KeepLatest, nil, nil, ptr(4096)},
}

// Default is what a category is given when nothing says otherwise.
func Default(c Category) CategoryPolicy {
	d, ok := defaults[c]
	if !ok {
		return defaults[CategoryDiagnostic]
	}
	return d
}

// For answers the policy in force for a category, with the defaults filled in
// for everything the installation did not set.
func (p RecordPolicy) For(c Category) CategoryPolicy {
	v, ok := p.Categories[c]
	if !ok {
		return Default(c)
	}
	d := Default(c)
	if v.Storage == "" {
		v.Storage = d.Storage
	}
	if v.Keep == "" {
		v.Keep = d.Keep
	}
	if v.MiB == nil {
		v.MiB = d.MiB
	}
	if v.Events == nil {
		v.Events = d.Events
	}
	if v.Keys == nil {
		v.Keys = d.Keys
	}
	// Control is not subject to storage. Validate refuses a policy that asks,
	// and this refuses to apply one that got in anyway -- through an older
	// writer, a hand-edited file, a rollback.
	if Control(c) {
		v.Storage, v.Keep = d.Storage, d.Keep
	}
	return v
}

// Bytes is the byte budget in force, and whether there is one. Zero bytes with
// ok true is unlimited, which is what an explicit 0 means.
func (p RecordPolicy) Bytes(c Category) (int64, bool) {
	if p.Disabled {
		return 0, false
	}
	v := p.For(c)
	if v.Keep == KeepLatest || v.MiB == nil {
		return 0, false
	}
	if *v.MiB == 0 {
		return 0, false
	}
	return int64(*v.MiB) * MiB, true
}

// Keys is the key budget in force for a `keep: latest` category.
func (p RecordPolicy) Keys(c Category) (int, bool) {
	if p.Disabled {
		return 0, false
	}
	v := p.For(c)
	if v.Keep != KeepLatest || v.Keys == nil || *v.Keys == 0 {
		return 0, false
	}
	return *v.Keys, true
}

// Records reports whether a category is recorded at all, and where.
func (p RecordPolicy) Storage(c Category) Storage { return p.For(c).Storage }

func (p RecordPolicy) Validate() error {
	names := make([]string, 0, len(p.Categories))
	for c := range p.Categories {
		names = append(names, string(c))
	}
	sort.Strings(names)
	for _, name := range names {
		c := Category(name)
		v := p.Categories[c]
		if _, ok := defaults[c]; !ok {
			return fmt.Errorf("unknown record category %q", name)
		}
		switch v.Storage {
		case "", StorageDisk, StorageMemory, StorageOff:
		default:
			return fmt.Errorf("%s: unknown storage %q (disk, memory, or off)", name, v.Storage)
		}
		switch v.Keep {
		case "", KeepHistory, KeepLatest:
		default:
			return fmt.Errorf("%s: unknown keep %q (history or latest)", name, v.Keep)
		}
		if Control(c) && (v.Storage == StorageMemory || v.Storage == StorageOff) {
			return fmt.Errorf("%s holds what a restart and a duplicate command need; it cannot be %s", name, v.Storage)
		}
		keep := v.Keep
		if keep == "" {
			keep = Default(c).Keep
		}
		if keep == KeepLatest && (v.MiB != nil || v.Events != nil) {
			return fmt.Errorf("%s keeps the latest value per key; bound it with keys, not bytes or events", name)
		}
		if keep == KeepHistory && v.Keys != nil {
			return fmt.Errorf("%s keeps history; bound it with mib or events, not keys", name)
		}
		for _, limit := range []struct {
			what string
			n    *int
			most int
		}{{"mib", v.MiB, 10240}, {"events", v.Events, 10000000}, {"keys", v.Keys, 1000000}} {
			if limit.n != nil && (*limit.n < 0 || *limit.n > limit.most) {
				return fmt.Errorf("%s: invalid %s %d (0 for unlimited, up to %d)", name, limit.what, *limit.n, limit.most)
			}
		}
	}
	return nil
}

// FromLegacy converts the policy this replaces.
//
// `max_mib` was one budget for everything, so there is no honest way to read a
// per-category intent out of it: it is treated as a scale on the categories
// that share a disk, and `conversation` is left at its default rather than
// scaled down -- the whole point of the new model is that the conversation
// stops being what a shortage takes. `raw_mib` becomes the stream's budget,
// because in the default conversion the replies are not recorded at all.
func FromLegacy(old Policy) RecordPolicy {
	p := RecordPolicy{Disabled: old.Disabled, Categories: map[Category]CategoryPolicy{}}
	diskLimit, _, _ := Policy{MaxMiB: old.MaxMiB}.Limits()
	scale := float64(diskLimit) / float64(100*MiB)
	for _, c := range Categories {
		d := Default(c)
		if d.Keep != KeepHistory || d.MiB == nil || c == CategoryConversation {
			continue
		}
		if c == CategoryProviderStream || c == CategoryProviderReply {
			continue
		}
		scaled := max(1, int(float64(*d.MiB)*scale))
		p.Categories[c] = CategoryPolicy{MiB: ptr(scaled)}
	}
	// Disabled rides on the RecordPolicy, so the budget itself is read without
	// it: converting a policy must not lose the number a person set because
	// removal happened to be off when they set it.
	raw := int(Policy{RawMiB: old.RawMiB}.RawLimit() / MiB)
	p.Categories[CategoryProviderStream] = CategoryPolicy{MiB: ptr(max(1, raw))}
	return p
}

// Window is unchanged by any of this: it is what a client chooses to hold, not
// what an installation keeps, and the two were already different axes.
