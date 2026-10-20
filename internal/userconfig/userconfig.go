// Package userconfig manages ~/.dockup/config.json: user-facing settings.
// Installation state (installed/distro location facts) lives in state.json,
// not here: config.json holds only user intent (paths, ports, styling).
//
//	{
//	  "default_path": "D:/WSL/Dockup",
//	  "current_path": "D:/WSL/Dockup",
//	  "port": 2375,
//	  "use_tcp": false,
//	  "pipe_name": "\\\\.\\pipe\\dockup_engine",
//	  "color": true,
//	  "autostart": false
//	}
//
// default_path is the prefilled install root for the next `dockup setup`
// (updated to whatever path was used on every successful setup).
// current_path is where the installed distro actually lives. The root holds
// distro\ (the WSL import target) and cache\ (the verified setup image).
// port/use_tcp control the optional 127.0.0.1 TCP bridge.
package userconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CHE3MZ/dockup/internal/config"
)

const (
	// DefaultPort is the TCP bridge port when use_tcp is enabled.
	DefaultPort = 2375
	// DefaultPipeName mirrors config.PipeName.
	DefaultPipeName = config.PipeName
	// MaxPort is the largest valid TCP port.
	MaxPort = 65535
)

// Config is the ~/.dockup/config.json schema.
type Config struct {
	DefaultPath string `json:"default_path"`
	CurrentPath string `json:"current_path"`
	Port        int    `json:"port"`
	UseTCP      bool   `json:"use_tcp"`
	PipeName    string `json:"pipe_name"`
	Color       bool   `json:"color"`
	Autostart   bool   `json:"autostart"`
}

// Dir is ~/.dockup.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return filepath.Join(home, ".dockup")
}

// File is ~/.dockup/config.json.
func File() string { return filepath.Join(Dir(), "config.json") }

// Defaults returns the bare-minimum config for a fresh machine.
func Defaults() Config {
	return Config{
		DefaultPath: filepath.ToSlash(config.DefaultInstallRoot()),
		CurrentPath: "",
		Port:        DefaultPort,
		UseTCP:      false,
		PipeName:    DefaultPipeName,
		Color:       true,
		Autostart:   false,
	}
}

// Load reads config.json. A missing file returns Defaults with nil error;
// a corrupt file returns the error so the user can fix or delete it.
func Load() (Config, error) {
	data, err := os.ReadFile(File())
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil
		}
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", File(), err)
	}
	return c.WithDefaults(), nil
}

// WithDefaults fills zero-valued fields so hand-edited files stay valid.
func (c Config) WithDefaults() Config {
	d := Defaults()
	if c.DefaultPath == "" {
		c.DefaultPath = d.DefaultPath
	}
	if c.Port == 0 {
		c.Port = d.Port
	}
	if c.PipeName == "" {
		c.PipeName = d.PipeName
	}
	return c
}

// Save writes config.json (creating ~/.dockup as needed).
func Save(c Config) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.WithDefaults(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(File(), append(data, '\n'), 0o600)
}

// Ensure creates the bare-minimum config on first run and returns it.
func Ensure() (Config, error) {
	if _, err := os.Stat(File()); err == nil {
		return Load()
	}
	c := Defaults()
	if err := Save(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// ValidatePort rejects out-of-range TCP ports.
func ValidatePort(p int) error {
	if p < 1 || p > MaxPort {
		return fmt.Errorf("port %d out of range (1-%d)", p, MaxPort)
	}
	return nil
}

// NormalizePath cleans a user-supplied install path: trims whitespace and
// surrounding quotes, requires an absolute path, returns slash form.
func NormalizePath(s string) (string, error) {
	p := strings.TrimSpace(s)
	p = strings.Trim(p, `"'`)
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path (e.g. D:/WSL/Dockup)")
	}
	if !filepath.IsAbs(p) && !filepath.IsAbs(filepath.FromSlash(p)) {
		return "", fmt.Errorf("path %q is not absolute (e.g. D:/WSL/Dockup)", s)
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(p))), nil
}

// ResolveInstallDir picks the setup install root: --path wins, otherwise
// the stored default_path. Returns the OS-native path.
func ResolveInstallDir(flagPath string, cfg Config) (string, error) {
	if strings.TrimSpace(flagPath) != "" {
		p, err := NormalizePath(flagPath)
		if err != nil {
			return "", err
		}
		return filepath.FromSlash(p), nil
	}
	p, err := NormalizePath(cfg.DefaultPath)
	if err != nil {
		return "", fmt.Errorf("bad default_path in %s: %w", File(), err)
	}
	return filepath.FromSlash(p), nil
}

// EffectivePipe returns the pipe to serve (custom or default).
func (c Config) EffectivePipe() string {
	if c.PipeName != "" {
		return c.PipeName
	}
	return DefaultPipeName
}

// EffectivePort returns the TCP port (custom or default).
func (c Config) EffectivePort() int {
	if c.Port != 0 {
		return c.Port
	}
	return DefaultPort
}

// InstallRoot returns the install root: current_path when installed, then
// default_path (which survives uninstall so the cache can still be found),
// then the legacy pre-config location.
func InstallRoot() string {
	c, _ := Load()
	c = c.WithDefaults()
	for _, raw := range []string{c.CurrentPath, c.DefaultPath} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if p, err := NormalizePath(raw); err == nil {
			return filepath.FromSlash(p)
		}
	}
	return config.WslInstallDir()
}

// DistroDir resolves the actual wsl --import target under root. New installs
// use root\distro; installs created before the split have ext4.vhdx directly
// in root and are used in place.
func DistroDir(root string) string {
	nested := filepath.Join(root, config.DistroSubdir)
	if hasVHDX(nested) {
		return nested
	}
	if hasVHDX(root) {
		return root
	}
	return nested
}

// DistroDirCurrent is DistroDir for the active install.
func DistroDirCurrent() string { return DistroDir(InstallRoot()) }

func hasVHDX(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "ext4.vhdx"))
	return err == nil && !st.IsDir()
}

// TCPAddr is the 127.0.0.1-only listen address for the optional TCP bridge.
func (c Config) TCPAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", c.EffectivePort())
}
