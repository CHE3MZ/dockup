package unit

import (
	"testing"

	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/userconfig"
)

func TestAutostartDefaultsOff(t *testing.T) {
	withTempHome(t)
	c, err := userconfig.Ensure()
	if err != nil {
		t.Fatal(err)
	}
	if c.Autostart {
		t.Fatal("autostart should default to false")
	}
}

func TestAutostartRoundTrip(t *testing.T) {
	withTempHome(t)
	c := userconfig.Defaults()
	c.Autostart = true
	if err := userconfig.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Autostart {
		t.Fatal("autostart not persisted")
	}
}

func TestParsePingOK(t *testing.T) {
	if !relay.ParsePingOK([]byte("HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\n\r\nOK")) {
		t.Fatal("200 OK should parse")
	}
	if !relay.ParsePingOK([]byte("HTTP/1.1 200 OK\r\n\r\n")) {
		t.Fatal("short 200 should parse")
	}
	for _, bad := range [][]byte{
		{},
		[]byte("short"),
		[]byte("HTTP/1.0 500 Internal Error\r\n\r\n"),
		[]byte("HTTP/1.0 404 Not Found\r\n\r\nxx"),
	} {
		if relay.ParsePingOK(bad) {
			t.Fatalf("should not parse: %q", bad)
		}
	}
}
