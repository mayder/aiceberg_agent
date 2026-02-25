package sysmetrics

import "testing"

func TestClampAbsInt64(t *testing.T) {
	const lim = int64(8_388_607)

	cases := []struct {
		name string
		in   int64
		want int64
	}{
		{name: "in range positive", in: 1200, want: 1200},
		{name: "in range negative", in: -1200, want: -1200},
		{name: "at positive limit", in: lim, want: lim},
		{name: "at negative limit", in: -lim, want: -lim},
		{name: "above limit", in: lim + 1, want: lim},
		{name: "below limit", in: -(lim + 1), want: -lim},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clampAbsInt64(tc.in, lim)
			if got != tc.want {
				t.Fatalf("clampAbsInt64(%d,%d)=%d want=%d", tc.in, lim, got, tc.want)
			}
		})
	}
}
