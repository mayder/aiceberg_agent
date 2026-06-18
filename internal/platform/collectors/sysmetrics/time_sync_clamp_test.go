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

func TestTimeSyncStatusClassifiesClockSkew(t *testing.T) {
	cases := []struct {
		name     string
		offsetMs int64
		want     string
	}{
		{name: "normal", offsetMs: 250, want: "ok"},
		{name: "normal negative", offsetMs: -999, want: "ok"},
		{name: "warning", offsetMs: 1_000, want: "warning"},
		{name: "warning negative", offsetMs: -4_999, want: "warning"},
		{name: "critical", offsetMs: 5_000, want: "critical"},
		{name: "critical negative", offsetMs: -30_000, want: "critical"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := timeSyncStatus(tc.offsetMs)
			if got != tc.want {
				t.Fatalf("timeSyncStatus(%d)=%q want=%q", tc.offsetMs, got, tc.want)
			}
		})
	}
}
