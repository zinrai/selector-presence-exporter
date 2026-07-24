package main

import "testing"

// A non-empty result is present, an empty result is absent, and a non-success status or a malformed
// body is an error rather than absent.
func TestParseQueryResult(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantPresent bool
		wantErr     bool
	}{
		{
			name:        "non-empty is present",
			body:        `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1,"1"]}]}}`,
			wantPresent: true,
		},
		{
			name: "empty is absent",
			body: `{"status":"success","data":{"resultType":"vector","result":[]}}`,
		},
		{
			name:    "status error is err (not absent)",
			body:    `{"status":"error","errorType":"bad_data","error":"boom"}`,
			wantErr: true,
		},
		{
			name:    "broken JSON is err",
			body:    `{not json`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			present, err := parseQueryResult([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if present != tt.wantPresent {
				t.Errorf("present=%v want=%v", present, tt.wantPresent)
			}
		})
	}
}
