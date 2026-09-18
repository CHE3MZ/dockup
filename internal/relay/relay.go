// Package relay serves \\.\pipe\docker_engine (go-winio) and proxies
// raw bytes to 127.0.0.1:<port>. Must be a raw byte-copy to support
// HTTP hijack (run -it, logs -f, exec). Full go-winio wiring lands in P1;
// this stub keeps the build green.
package relay

import (
	"fmt"
)

const PipeName = `\\.\pipe\docker_engine`

// Serve blocks serving the named pipe. ctx-cancel/SIGINT must stop the
// listener (implemented in P1 with go-winio).
func Serve(targetPort int) error {
	return fmt.Errorf("relay: not yet implemented (target 127.0.0.1:%d, pipe %s)", targetPort, PipeName)
}
