// Package config defines configuration settings for mflg and functions for loading them from a file.
package config

import (
	"github.com/BurntSushi/toml"
	"github.com/dpinela/mflg/internal/color"

	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	TabWidth    int
	ScrollSpeed int
	TextStyle   struct {
		Comment, String Style
	}
	Lang map[string]LangConfig
}

type Style struct {
	Foreground, Background  *color.Color
	Bold, Italic, Underline bool
}

type LangConfig struct {
	Formatter []string // Formatter program and arguments to pass to it
}

// Returns the appropriate LangConfig for a file with the given filename extension.
// If none exists, returns a zero LangConfig.
func (c *Config) ConfigForExt(ext string) LangConfig { return c.Lang[ext] }

// Load finds and reads the primary configuration file for the current user, according to the
// XDG base directory specification for configuration files. It always returns a usable *Config,
// even if it also returns a non-nil error.
// The file is expected to be at mflg/config.toml in one of the appropriate configuration directories.
func Load() (c *Config, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("error loading config file: %w", err)
		}
	}()
	c = &Config{
		TabWidth:    4,
		ScrollSpeed: 1,
		Lang:        make(map[string]LangConfig),
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return c, err
	}
	f, err := os.Open(filepath.Join(dir, "mflg", "config.toml"))
	if err != nil {
		return c, err
	}
	defer f.Close()
	_, err = toml.DecodeReader(f, c)
	if c.TextStyle.Comment == (Style{}) {
		c.TextStyle.Comment = Style{Foreground: &color.Color{R: 0, G: 200, B: 0}}
	}
	if c.TextStyle.String == (Style{}) {
		c.TextStyle.String = Style{Foreground: &color.Color{R: 0, G: 0, B: 200}}
	}
	return c, err
}
