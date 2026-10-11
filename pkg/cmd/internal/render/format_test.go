package render

import "testing"

func TestValidateOutputFormat_EmptyAllowedPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("ValidateOutputFormat with no allowed values must panic")
		}
	}()

	_ = ValidateOutputFormat("json")
}

func TestValidateOutputFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		format  string
		allowed []string
		want    string
	}{
		{"empty is the default", "", []string{"json", "table"}, ""},
		{"json", "json", []string{"json", "table"}, ""},
		{"table", "table", []string{"json", "table"}, ""},
		{"unknown", "xml", []string{"json", "table"}, "unknown output format: xml (use 'json' or 'table')"},
		{"yaml rejected when not allowed", "yaml", []string{"json", "table"}, "unknown output format: yaml (use 'json' or 'table')"},
		{"case-sensitive json", "JSON", []string{"json", "table"}, "unknown output format: JSON (use 'json' or 'table')"},
		{"case-sensitive table", "Table", []string{"json", "table"}, "unknown output format: Table (use 'json' or 'table')"},
		{"yaml accepted when allowed", "yaml", []string{"json", "yaml", "table"}, ""},
		{"unknown with three", "xml", []string{"json", "yaml", "table"}, "unknown output format: xml (use 'json', 'yaml', or 'table')"},
		{"unknown with one", "xml", []string{"json"}, "unknown output format: xml (use 'json')"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateOutputFormat(tc.format, tc.allowed...)

			switch {
			case tc.want == "" && err != nil:
				t.Errorf("ValidateOutputFormat(%q, %v) = %v, want nil", tc.format, tc.allowed, err)
			case tc.want != "" && err == nil:
				t.Errorf("ValidateOutputFormat(%q, %v) = nil, want %q", tc.format, tc.allowed, tc.want)
			case tc.want != "" && err.Error() != tc.want:
				t.Errorf("ValidateOutputFormat(%q, %v) = %q, want %q", tc.format, tc.allowed, err.Error(), tc.want)
			}
		})
	}
}
