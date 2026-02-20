package pgxaudit

import "testing"

func TestNullString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		isNil  bool
		wantVal string
	}{
		{"empty returns nil", "", true, ""},
		{"non-empty returns pointer", "hello", false, "hello"},
		{"whitespace is not empty", " ", false, " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nullString(tt.input)
			if tt.isNil {
				if got != nil {
					t.Errorf("nullString(%q) = %v, want nil", tt.input, *got)
				}
			} else {
				if got == nil {
					t.Fatalf("nullString(%q) = nil, want %q", tt.input, tt.wantVal)
				}
				if *got != tt.wantVal {
					t.Errorf("nullString(%q) = %q, want %q", tt.input, *got, tt.wantVal)
				}
			}
		})
	}
}

func TestScannerInterface(t *testing.T) {
	// Verify that the scanner interface is correctly defined
	// by checking it can be used as a type constraint.
	var _ scanner = (*mockScanner)(nil)
}

type mockScanner struct {
	err error
}

func (m *mockScanner) Scan(_ ...any) error {
	return m.err
}
