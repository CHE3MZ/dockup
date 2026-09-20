package unit

import (
	"reflect"
	"testing"

	"github.com/CHE3MZ/dockup/internal/restore"
)

func TestRemovalList(t *testing.T) {
	current := []string{"docker-ce", "curl", "cowsay", "socat", "htop"}
	snapshot := []string{"docker-ce", "curl", "socat"}
	got := restore.RemovalList(current, snapshot)
	want := []string{"cowsay", "htop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRemovalListEmptySnapshotKeepsEngine(t *testing.T) {
	current := []string{"docker-ce", "docker-ce-cli", "containerd.io", "cowsay"}
	got := restore.RemovalList(current, nil)
	want := []string{"cowsay"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRemovalListUpgradesAreSafe(t *testing.T) {
	// Same names, newer versions elsewhere: name-based diff must not flag.
	current := []string{"docker-ce", "curl"}
	snapshot := []string{"docker-ce", "curl"}
	if got := restore.RemovalList(current, snapshot); len(got) != 0 {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestRemovalListEmpty(t *testing.T) {
	if got := restore.RemovalList(nil, nil); len(got) != 0 {
		t.Fatalf("got %q, want empty", got)
	}
}
