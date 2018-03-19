// Package config defines configuration settings for mflg and functions for loading them from a file.
package config

import (
	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/tajtiattila/basedir"
)

type Config struct {
	TabWidth        int
	DefaultSoftTabs bool
	Lang            map[string]*LangConfig
}

type LangConfig struct {
	Formatter string
}

// Returns the appropriate LangConfig for a file with the given filename extension.
// If none exists, returns nil.
func (c *Config) ConfigForExt(ext string) *LangConfig { return c.Lang[ext] }

// Load finds and reads the primary configuration file for the current user, according to the
// XDG base directory specification for configuration files. It always returns a usable *Config,
// even if it also returns a non-nil error.
func Load() (*Config, error) {
	c := Config{TabWidth: 4, Lang: make(map[string]*LangConfig)}
	f, err := basedir.Config.Open("mflg.toml")
	if err != nil {
		return &c, errors.WithMessage(err, "error loading config file")
	}
	defer f.Close()
	_, err = toml.DecodeReader(f, &c)
	return &c, errors.WithMessage(err, "error loading config file")
}
