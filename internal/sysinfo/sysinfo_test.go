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
	got := parseTasklist(out, 9999)
	want := []dockupProc{{pid: 1234, kb: 12344}, {pid: 5678, kb: 8}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestParseTasklistEmpty(t *testing.T) {
	if got := parseTasklist("", 1); len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
	if got := parseTasklist("INFO: No tasks are running\n", 1); len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}
