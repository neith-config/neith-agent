package config

import "testing"

func TestParseSnapshotEnvelopeRejectsUnsupportedShapesAndValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "null", body: `null`},
		{name: "missing config", body: `{"revision":"018f0000-0000-7000-8000-000000000001"}`},
		{name: "unknown field", body: `{"revision":"018f0000-0000-7000-8000-000000000001","config":{},"extra":true}`},
		{name: "invalid environment name", body: `{"revision":"018f0000-0000-7000-8000-000000000001","config":{"BAD-NAME":"value"}}`},
		{name: "NUL environment value", body: `{"revision":"018f0000-0000-7000-8000-000000000001","config":{"KEY":"bad\u0000value"}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseSnapshotEnvelope([]byte(tc.body)); err == nil {
				t.Fatalf("parseSnapshotEnvelope accepted %s", tc.name)
			}
		})
	}
}

func TestSnapshotParserRejectsMalformedJSONDetails(t *testing.T) {
	for _, body := range []string{
		`{"revision":"018f0000-0000-7000-8000-00000000000z","config":{}}`,
		`{"revision":"018f0000-0000-7000-8000-000000000001","config":{}} {}`,
		`{"revision":"018f0000-0000-7000-8000-000000000001","config":{"1BAD":"x"}}`,
		`{"revision":"018f0000-0000-7000-8000-000000000001","config":{"A":"x\u0000y"}}`,
	} {
		if _, err := parseSnapshotEnvelope([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	if _, err := parseConfigMap([]byte(`null`)); err == nil {
		t.Fatal("accepted null config")
	}
}

func TestConfigNamesRejectEmpty(t *testing.T) {
	if _, err := parseConfigMap([]byte(`{"":"value"}`)); err == nil {
		t.Fatal("accepted empty environment name")
	}
}
