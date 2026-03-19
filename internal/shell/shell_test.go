package shell

import (
	"strings"
	"testing"

	"hostward/internal/state"
)

func TestBannerListModeUsesNames(t *testing.T) {
	snapshot := state.Snapshot{
		TotalCount:   5,
		FailingCount: 2,
		Failing:      []string{"backup-nightly", "photos-rsync"},
	}

	got := Banner(snapshot, BannerList)
	want := "hostward: failing: backup-nightly, photos-rsync"
	if got != want {
		t.Fatalf("Banner() = %q, want %q", got, want)
	}
}

func TestBannerCountModeFallsBackToCounts(t *testing.T) {
	snapshot := state.Snapshot{
		TotalCount:   6,
		FailingCount: 2,
		Failing:      []string{"backup-nightly", "photos-rsync"},
	}

	got := Banner(snapshot, BannerCount)
	want := "hostward: failing: 2 of 6"
	if got != want {
		t.Fatalf("Banner() = %q, want %q", got, want)
	}
}

func TestSnippetZshExportsFailingCount(t *testing.T) {
	got, err := Snippet("zsh")
	if err != nil {
		t.Fatalf("Snippet() error = %v", err)
	}

	if !strings.Contains(got, "HOSTWARD_FAILING_COUNT") {
		t.Fatalf("Snippet() missing HOSTWARD_FAILING_COUNT export: %q", got)
	}
}

func TestBannerShowsUnknownWhenPresent(t *testing.T) {
	snapshot := state.Snapshot{
		TotalCount:   3,
		UnknownCount: 1,
		StatusCounts: state.StatusCounts{
			OK:      2,
			Unknown: 1,
		},
	}

	got := Banner(snapshot, BannerCount)
	want := "hostward: 2 ok, 1 unknown"
	if got != want {
		t.Fatalf("Banner() = %q, want %q", got, want)
	}
}
