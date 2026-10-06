package sysinfo

import (
	"reflect"
	"testing"
)

func TestParseTasklist(t *testing.T) {
	out := "\"dockup.exe\",\"1234\",\"Console\",\"1\",\"12,344 K\"\n" +
		"\"DOCKUP.EXE\",\"5678\",\"Console\",\"1\",\"8 K\"\n" + // casing must not matter
		"\"dockup.exe\",\"9999\",\"Console\",\"1\",\"4 K\"\n" + // self is skipped
		"\"other.exe\",\"1111\",\"Console\",\"1\",\"4 K\"\n" +
		"garbage line\n"
	got := parseTasklist(out, 9999, "dockup.exe")
	want := []dockupProc{{pid: 1234, kb: 12344}, {pid: 5678, kb: 8}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestParseTasklistEmpty(t *testing.T) {
	if got := parseTasklist("", 1, "dockup.exe"); len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
	if got := parseTasklist("INFO: No tasks are running\n", 1, "dockup.exe"); len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}

func TestParseTasklistRenamedBinary(t *testing.T) {
	// dist/renamed binaries (dockup-windows-amd64.exe) must match by their
	// own name and ignore the default one.
	out := "\"dockup-windows-amd64.exe\",\"4321\",\"Console\",\"1\",\"9,000 K\"\n" +
		"\"dockup.exe\",\"1234\",\"Console\",\"1\",\"12,344 K\"\n"
	got := parseTasklist(out, 9999, "dockup-windows-amd64.exe")
	want := []dockupProc{{pid: 4321, kb: 9000}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestSelfImageNameFallback(t *testing.T) {
	if got := selfImageName(); got == "" {
		t.Fatal("must never be empty (shutdown builds a tasklist filter from it)")
	}
}
