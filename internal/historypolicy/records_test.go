package historypolicy

import (
	"encoding/json"
	"testing"
)

// A budget of zero and a budget nobody wrote are opposite instructions.
func TestUnsetIsNotZero(t *testing.T) {
	var p RecordPolicy
	if b, ok := p.Bytes(CategoryWorkDetail); !ok || b != 32*MiB {
		t.Fatal("an unset budget did not take the default", b, ok)
	}
	p = RecordPolicy{Categories: map[Category]CategoryPolicy{
		CategoryWorkDetail: {MiB: ptr(0)},
	}}
	if _, ok := p.Bytes(CategoryWorkDetail); ok {
		t.Fatal("an explicit zero was read as a budget rather than as unlimited")
	}
	// And it survives the file: a format that cannot tell them apart turns one
	// into the other on the next save.
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back RecordPolicy
	if err = json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if _, ok := back.Bytes(CategoryWorkDetail); ok {
		t.Fatal("the explicit zero did not survive the round trip:", string(b))
	}
	if v := back.For(CategoryWorkDetail); v.Events == nil || *v.Events != 50000 {
		t.Fatal("a set byte budget dropped the default event budget beside it")
	}
}

// Disabled turns every removal off, which is the one switch the policy it
// replaces had.
func TestDisabledRemovesNothing(t *testing.T) {
	p := RecordPolicy{Disabled: true}
	for _, c := range Categories {
		if _, ok := p.Bytes(c); ok {
			t.Fatalf("%s is still bounded", c)
		}
		if _, ok := p.Keys(c); ok {
			t.Fatalf("%s is still bounded by keys", c)
		}
	}
}

// What a restart, an unanswered question and a duplicate command need is not
// subject to storage. The cost of honouring such a request is a session that
// cannot come back or a command that runs twice.
func TestControlCannotBeMadeVolatile(t *testing.T) {
	for _, s := range []Storage{StorageMemory, StorageOff} {
		p := RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryControl: {Storage: s}}}
		if err := p.Validate(); err == nil {
			t.Fatalf("a policy asking for control in %s was accepted", s)
		}
		// And a policy that got in anyway -- an older writer, a hand edit, a
		// rollback -- is not applied.
		if got := p.Storage(CategoryControl); got != StorageDisk {
			t.Fatalf("control was kept in %s", got)
		}
	}
}

func TestImpossibleCombinationsAreRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    RecordPolicy
	}{
		{"unknown category", RecordPolicy{Categories: map[Category]CategoryPolicy{"invented": {}}}},
		{"unknown storage", RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryWorkDetail: {Storage: "tape"}}}},
		{"unknown keep", RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryWorkDetail: {Keep: "some"}}}},
		{"bytes on a latest category", RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryModelCatalog: {MiB: ptr(4)}}}},
		{"events on a latest category", RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryAccountLimits: {Events: ptr(10)}}}},
		{"keys on a history category", RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryConversation: {Keys: ptr(10)}}}},
		{"negative", RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryWorkDetail: {MiB: ptr(-1)}}}},
		{"absurd", RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryWorkDetail: {MiB: ptr(99999)}}}},
	} {
		if err := tc.p.Validate(); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}
	// And the combinations that are the point of the model are accepted.
	ok := RecordPolicy{Categories: map[Category]CategoryPolicy{
		CategoryProviderStream: {Storage: StorageMemory},
		CategoryProviderReply:  {Storage: StorageOff},
		CategoryWorkDetail:     {MiB: ptr(0), Events: ptr(0)},
		CategoryAccountLimits:  {Keys: ptr(128)},
	}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

// The conversation is 0.01% of the measured budget and the one thing that must
// not be what a shortage takes.
func TestTheConversationIsNotWhatAShortageTakes(t *testing.T) {
	var p RecordPolicy
	body, _ := p.Bytes(CategoryConversation)
	detail, _ := p.Bytes(CategoryWorkDetail)
	stream, _ := p.Bytes(CategoryProviderStream)
	if body <= detail || body <= stream {
		t.Fatal("the conversation's budget is not the largest", body, detail, stream)
	}
	if p.Storage(CategoryProviderReply) != StorageOff {
		t.Fatal("provider replies are recorded by default; they were 80-99% of the raw that survived")
	}
	// Turning them on is bounded rather than unlimited.
	on := RecordPolicy{Categories: map[Category]CategoryPolicy{CategoryProviderReply: {Storage: StorageDisk}}}
	if _, ok := on.Bytes(CategoryProviderReply); !ok {
		t.Fatal("switching provider replies on left them unbounded")
	}
}

// Converting the old policy must not quietly change what a person set, and must
// not scale down the one category the new model exists to protect.
func TestLegacyConversion(t *testing.T) {
	p := FromLegacy(Policy{})
	if v := p.For(CategoryConversation); v.MiB == nil || *v.MiB != 64 {
		t.Fatal("the default conversion moved the conversation's budget", v.MiB)
	}
	if v := p.For(CategoryProviderStream); v.MiB == nil || *v.MiB != 20 {
		t.Fatal("raw_mib did not become the stream's budget", v.MiB)
	}
	if p.Storage(CategoryProviderReply) != StorageOff {
		t.Fatal("the conversion started recording provider replies")
	}

	// Half the disk budget halves what shares the disk, and leaves the
	// conversation where it was.
	half := FromLegacy(Policy{MaxMiB: 50, RawMiB: 4})
	if v := half.For(CategoryWorkDetail); v.MiB == nil || *v.MiB != 16 {
		t.Fatal("work detail was not scaled", v.MiB)
	}
	if v := half.For(CategoryConversation); v.MiB == nil || *v.MiB != 64 {
		t.Fatal("the conversation was scaled down with everything else", v.MiB)
	}
	if v := half.For(CategoryProviderStream); v.MiB == nil || *v.MiB != 4 {
		t.Fatal("the stream did not take raw_mib", v.MiB)
	}
	if err := half.Validate(); err != nil {
		t.Fatal(err)
	}

	// Removal being off when a budget was written does not lose the budget.
	off := FromLegacy(Policy{Disabled: true, RawMiB: 7})
	if !off.Disabled {
		t.Fatal("disabled was dropped")
	}
	if v := off.For(CategoryProviderStream); v.MiB == nil || *v.MiB != 7 {
		t.Fatal("a budget set while removal was off was lost", v.MiB)
	}
	// A tiny budget stays a budget rather than becoming unlimited.
	tiny := FromLegacy(Policy{MaxMiB: 1})
	if v := tiny.For(CategoryWorkDiff); v.MiB == nil || *v.MiB != 1 {
		t.Fatal("a scaled budget rounded to unlimited", v.MiB)
	}
}

// Every category has a default, so no category is silently unbounded.
func TestEveryCategoryHasABudget(t *testing.T) {
	var p RecordPolicy
	for _, c := range Categories {
		v := p.For(c)
		if v.Storage == "" || v.Keep == "" {
			t.Fatalf("%s has no default storage or keep", c)
		}
		switch v.Keep {
		case KeepHistory:
			if v.MiB == nil && p.Storage(c) != StorageOff {
				t.Fatalf("%s keeps history with no byte budget", c)
			}
		case KeepLatest:
			if v.Keys == nil {
				t.Fatalf("%s keeps the latest value with no key budget", c)
			}
		}
	}
	if len(Categories) != len(defaults) {
		t.Fatal("the category list and the defaults disagree")
	}
}
