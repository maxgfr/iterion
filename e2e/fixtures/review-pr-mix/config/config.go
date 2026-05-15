// Package config loads runtime configuration from env vars.
package config

import (
	"os"
	"strconv"
)

// Config is the runtime knobs the server reads at startup.
type Config struct {
	Workers      int
	QueueSize    int
	SecretToken  string
	StorageRoot  string
}

// Load reads env vars and returns a populated Config. Missing values
// fall back to small defaults.
func Load() Config {
	c := Config{Workers: 4, QueueSize: 100, StorageRoot: "/tmp/jobqueue"}
	if v := os.Getenv("JOBQUEUE_WORKERS"); v != "" {
		n, _ := strconv.Atoi(v)
		c.Workers = n
	}
	if v := os.Getenv("JOBQUEUE_QUEUE_SIZE"); v != "" {
		n, _ := strconv.Atoi(v)
		c.QueueSize = n
	}
	if v := os.Getenv("JOBQUEUE_SECRET"); v != "" {
		c.SecretToken = v
	}
	if v := os.Getenv("JOBQUEUE_STORAGE_ROOT"); v != "" {
		c.StorageRoot = v
	}
	return c
}
