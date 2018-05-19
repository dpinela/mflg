// Package config defines configuration settings for mflg and functions for loading them from a file.
package config

import (
	"github.com/BurntSushi/toml"
	"github.com/dpinela/mflg/internal/color"
	"github.com/pkg/errors"
	"github.com/tajtiattila/basedir"

	"path/filepath"
)

type Config struct {
	TabWidth  int
	TextStyle struct {
		Comment, String Style
	}
	Lang map[string]LangConfig
}

type Style struct {
	Foreground, Background *color.Color
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
func Load() (*Config, error) {
	c := Config{
		TabWidth: 4,
		Lang:     make(map[string]LangConfig),
	}
	c.TextStyle.Comment.Foreground = &color.Color{R: 0, G: 200, B: 0}
	c.TextStyle.String.Foreground = &color.Color{R: 0, G: 0, B: 200}
	f, err := basedir.Config.Open(filepath.Join("mflg", "config.toml"))
	if err != nil {
		return &c, errors.WithMessage(err, "error loading config file")
	}
	defer f.Close()
	_, err = toml.DecodeReader(f, &c)
	return &c, errors.WithMessage(err, "error loading config file")
}
