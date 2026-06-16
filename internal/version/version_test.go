package version

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		want    Version
		wantErr bool
	}{
		{in: "1.2.3", want: Version{1, 2, 3}},
		{in: "v0.10.0", want: Version{0, 10, 0}},
		{in: "  1.0.0 ", want: Version{1, 0, 0}},
		{in: "", wantErr: true},
		{in: "1.2", wantErr: true},
		{in: "1.2.x", wantErr: true},
		{in: "1.-1.0", wantErr: true},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Parse(%q): want error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b Version
		want int
	}{
		{Version{1, 0, 0}, Version{1, 0, 0}, 0},
		{Version{1, 2, 0}, Version{1, 1, 9}, 1},
		{Version{0, 9, 9}, Version{1, 0, 0}, -1},
		{Version{1, 0, 1}, Version{1, 0, 0}, 1},
	}
	for _, c := range cases {
		if got := c.a.Compare(c.b); got != c.want {
			t.Errorf("%v.Compare(%v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNext(t *testing.T) {
	base := Version{1, 4, 2}
	cases := []struct {
		level   string
		want    Version
		wantErr bool
	}{
		{level: "major", want: Version{2, 0, 0}},
		{level: "minor", want: Version{1, 5, 0}},
		{level: "patch", want: Version{1, 4, 3}},
		{level: "build", wantErr: true},
	}
	for _, c := range cases {
		got, err := base.Next(c.level)
		if c.wantErr {
			if err == nil {
				t.Errorf("Next(%q): want error", c.level)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("Next(%q) = %v, %v; want %v", c.level, got, err, c.want)
		}
	}
}

func TestNextCalVer(t *testing.T) {
	now := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	if got := NextCalVer(Version{}, now); got != (Version{2026, 6, 0}) {
		t.Errorf("first release = %v, want 2026.6.0", got)
	}
	if got := NextCalVer(Version{2026, 6, 0}, now); got != (Version{2026, 6, 1}) {
		t.Errorf("same month = %v, want 2026.6.1", got)
	}
	if got := NextCalVer(Version{2026, 5, 7}, now); got != (Version{2026, 6, 0}) {
		t.Errorf("new month resets micro = %v, want 2026.6.0", got)
	}
}
