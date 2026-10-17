package config

import "testing"

func TestParseRevisionRequiresUUIDv7(t *testing.T) {
	valid, err := ParseRevision("018f0000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatalf("ParseRevision(valid): %v", err)
	}
	if got := valid.String(); got != "018f0000-0000-7000-8000-000000000001" {
		t.Fatalf("revision round-trip = %q", got)
	}

	for _, input := range []string{
		"018f0000-0000-6000-8000-000000000001",
		"018f0000-0000-7000-c000-000000000001",
		"not-a-uuid",
	} {
		if _, err := ParseRevision(input); err == nil {
			t.Errorf("ParseRevision(%q) accepted invalid UUIDv7", input)
		}
	}
}

func TestRevisionCompareUsesUUIDv7Order(t *testing.T) {
	older, err := ParseRevision("018f0000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	newer, err := ParseRevision("018f0000-0000-7000-8000-000000000002")
	if err != nil {
		t.Fatal(err)
	}

	if older.Compare(newer) >= 0 || newer.Compare(older) <= 0 || older.Compare(older) != 0 {
		t.Fatal("unexpected UUIDv7 comparison order")
	}
}

func TestParseSnapshotEnvelope(t *testing.T) {
	snapshot, err := parseSnapshotEnvelope([]byte(`{"revision":"018f0000-0000-7000-8000-000000000001","config":{"KEY":"VALUE"}}`))
	if err != nil {
		t.Fatalf("parseSnapshotEnvelope: %v", err)
	}
	if snapshot.Config["KEY"] != "VALUE" {
		t.Fatalf("unexpected config: %#v", snapshot.Config)
	}
}
