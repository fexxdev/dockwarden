package firmwareversion

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		want      int
	}{
		{name: "equal", candidate: "01.01.00.15", current: "1.1.0.15", want: 0},
		{name: "newer", candidate: "01.01.00.15", current: "01.01.00.13", want: 1},
		{name: "older", candidate: "01.01.00.13", current: "01.01.00.15", want: -1},
		{name: "leading zeroes", candidate: "01.023", current: "1.22", want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Compare(test.candidate, test.current)
			if err != nil {
				t.Fatalf("Compare(%q, %q) error = %v", test.candidate, test.current, err)
			}
			if got != test.want {
				t.Fatalf("Compare(%q, %q) = %d, want %d", test.candidate, test.current, got, test.want)
			}
		})
	}
}

func TestCompareRejectsInvalidVersion(t *testing.T) {
	if _, err := Compare("01.01.bad", "01.01.00.15"); err == nil {
		t.Fatal("Compare accepted an invalid firmware version")
	}
}
