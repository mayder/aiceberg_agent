package usecase

import "testing"

func TestUpdateCandidateVersionPrecedence(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.8.71-rc.1", "0.8.70", 1}, {"0.8.71", "0.8.71-rc.1", 1}, {"0.8.71-rc.1", "0.8.71", -1},
		{"0.8.71-rc.2", "0.8.71-rc.10", -1}, {"0.8.71-beta.1", "0.8.71-rc.1", -1},
		{"0.8.71-rc.1", "0.8.71-rc.1.1", -1}, {"0.8.71-1", "0.8.71-alpha", -1},
		{"0.8.71+build.9", "0.8.71+build.1", 0}, {"v0.8.71", "0.8.71", 0}, {"0.8.9", "0.8.70", -1},
		{"0.8.99999999999999999999", "0.8.99999999999999999998", 1},
	} {
		t.Run(c.a+"/"+c.b, func(t *testing.T) {
			got := compareVersionStrings(c.a, c.b)
			if got != c.want || compareVersionStrings(c.b, c.a) != -c.want {
				t.Fatalf("got %d want %d", got, c.want)
			}
		})
	}
}
