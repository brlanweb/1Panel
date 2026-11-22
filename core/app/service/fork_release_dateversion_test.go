package service

import "testing"

func TestCompareForkDateVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// Date versions must supersede every legacy fork release.
		{"v2.20261122.1", "v2.5.1-cluster.6", 1},
		{"v2.20261122.1", "v2.5.1", 1},
		{"v2.20261122.2", "v2.20261122.1", 1},
		{"v2.20261123.1", "v2.20261122.9", 1},
		{"v2.20261122.10", "v2.20261122.9", 1},
		{"v2.20261122.1", "v2.20261122.1", 0},
	}
	for _, c := range cases {
		if got := compareForkVersion(c.a, c.b); got != c.want {
			t.Fatalf("compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
