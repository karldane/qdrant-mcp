package config

import (
	"flag"
	"os"
	"strconv"
	"sync"
)

var initFlags sync.Once

func ResetForTest() {
	initFlags = sync.Once{}
}

type Config struct {
	AdminURL        string
	AdminKey        string
	UserSecret      string
	Host            string
	Username        string
	Collection      string
	VectorSize      int
	TimeoutSeconds  int
	isReadOnly      bool
	LogJSON         bool
}

func (c *Config) ReadOnly() bool {
	return c.isReadOnly
}

func Load() *Config {
	initFlags.Do(func() {
		flag.Parse()
	})

	cfg := &Config{
		VectorSize:     1536,
		TimeoutSeconds: 30,
		isReadOnly:     false,
		LogJSON:        false,
	}

	if v := os.Getenv("QDRANT_ADMIN_URL"); v != "" {
		cfg.AdminURL = v
	}

	if v := os.Getenv("QDRANT_ADMIN_KEY"); v != "" {
		cfg.AdminKey = v
	}

	if v := os.Getenv("QDRANT_USER_SECRET"); v != "" {
		cfg.UserSecret = v
	}

	if v := os.Getenv("QDRANT_HOST"); v != "" {
		cfg.Host = v
	}

	if v := os.Getenv("QDRANT_USERNAME"); v != "" {
		cfg.Username = v
	}

	if v := os.Getenv("QDRANT_COLLECTION"); v != "" {
		cfg.Collection = v
	}

	if v := os.Getenv("QDRANT_VECTOR_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.VectorSize = size
		}
	}

	if v := os.Getenv("QDRANT_TIMEOUT_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.TimeoutSeconds = secs
		}
	}

	return cfg
}

func MergeCLIFlags(cfg *Config) *Config {
	if flag.Lookup("admin-url") != nil {
		if v := flag.Lookup("admin-url").Value.String(); v != "" {
			cfg.AdminURL = v
		}
	}
	if flag.Lookup("admin-key") != nil {
		if v := flag.Lookup("admin-key").Value.String(); v != "" {
			cfg.AdminKey = v
		}
	}
	if flag.Lookup("user-secret") != nil {
		if v := flag.Lookup("user-secret").Value.String(); v != "" {
			cfg.UserSecret = v
		}
	}
	if flag.Lookup("host") != nil {
		if v := flag.Lookup("host").Value.String(); v != "" {
			cfg.Host = v
		}
	}
	if flag.Lookup("username") != nil {
		if v := flag.Lookup("username").Value.String(); v != "" {
			cfg.Username = v
		}
	}
	if flag.Lookup("collection") != nil {
		if v := flag.Lookup("collection").Value.String(); v != "" {
			cfg.Collection = v
		}
	}
	if flag.Lookup("vector-size") != nil {
		if v := flag.Lookup("vector-size").Value.String(); v != "" {
			if size, err := strconv.Atoi(v); err == nil && size > 0 {
				cfg.VectorSize = size
			}
		}
	}
	if flag.Lookup("timeout") != nil {
		if v := flag.Lookup("timeout").Value.String(); v != "" {
			if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
				cfg.TimeoutSeconds = secs
			}
		}
	}
	if flag.Lookup("readonly") != nil {
		cfg.isReadOnly = flag.Lookup("readonly").Value.String() == "true"
	}
	if flag.Lookup("log-json") != nil {
		cfg.LogJSON = flag.Lookup("log-json").Value.String() == "true"
	}
	return cfg
}

func init() {
	flag.String("admin-url", "", "Qdrant admin URL")
	flag.String("admin-key", "", "Qdrant admin API key")
	flag.String("user-secret", "", "Secret for deriving user API key")
	flag.String("host", "", "Qdrant host")
	flag.String("username", "", "User identifier")
	flag.String("collection", "", "Collection name (derived from sanitised email)")
	flag.Int("vector-size", 1536, "Vector size for collection (default: 1536 for OpenAI)")
	flag.Int("timeout", 30, "HTTP timeout in seconds")
	flag.Bool("readonly", false, "Disable all mutating tools")
	flag.Bool("log-json", false, "Emit structured JSON logs")
}