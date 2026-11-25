package modrinth

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRateLimitPerMinute = 300
	searchPageSize            = 100
	batchSize                 = 1000
)

type CrawlConfig struct {
	Enabled            bool
	APIBase            string
	UserAgent          string
	Interval           time.Duration
	Timeout            time.Duration
	HourUTC            int
	MinuteUTC          int
	RunOnStart         bool
	RateLimitPerMinute int
	Concurrency        int
	MaxProjects        int
	ScopeFacet         string
	WorkMem            string
}

func DefaultCrawlConfig() CrawlConfig {
	return CrawlConfig{
		Enabled:            false,
		APIBase:            "https://api.modrinth.com",
		UserAgent:          "openMCDB/1.0",
		Interval:           24 * time.Hour,
		Timeout:            2 * time.Hour,
		RateLimitPerMinute: defaultRateLimitPerMinute,
		Concurrency:        16,
		WorkMem:            "64MB",
	}
}

func NormalizeCrawlConfig(cfg CrawlConfig) (CrawlConfig, error) {
	defaults := DefaultCrawlConfig()
	if cfg.APIBase == "" {
		cfg.APIBase = defaults.APIBase
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaults.UserAgent
	}
	if cfg.WorkMem == "" {
		cfg.WorkMem = defaults.WorkMem
	}
	if cfg.Interval == 0 {
		cfg.Interval = defaults.Interval
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.RateLimitPerMinute == 0 {
		cfg.RateLimitPerMinute = defaults.RateLimitPerMinute
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = defaults.Concurrency
	}
	return cfg, ValidateCrawlConfig(cfg)
}

func ValidateCrawlConfig(cfg CrawlConfig) error {
	var errs []error
	check := func(ok bool, message string) {
		if !ok {
			errs = append(errs, fmt.Errorf("openmcdb/modrinth: %s", message))
		}
	}
	base, err := url.Parse(cfg.APIBase)
	check(err == nil && base != nil && (base.Scheme == "http" || base.Scheme == "https") && base.Host != "" && base.RawQuery == "" && base.Fragment == "", "APIBase must be an absolute HTTP(S) URL without a query or fragment")
	check(strings.TrimSpace(cfg.UserAgent) != "" && !strings.ContainsAny(cfg.UserAgent, "\r\n"), "UserAgent must be nonempty and contain no line breaks")
	check(cfg.Interval > 0, "Interval must be positive")
	check(cfg.Timeout > 0, "Timeout must be positive")
	check(cfg.HourUTC >= 0 && cfg.HourUTC <= 23, "HourUTC must be between 0 and 23")
	check(cfg.MinuteUTC >= 0 && cfg.MinuteUTC <= 59, "MinuteUTC must be between 0 and 59")
	check(cfg.RateLimitPerMinute > 0 && uint64(cfg.RateLimitPerMinute) <= uint64(time.Minute), "RateLimitPerMinute must be positive and permit at least 1ns between requests")
	check(cfg.Concurrency > 0, "Concurrency must be positive")
	check(cfg.MaxProjects >= 0, "MaxProjects must be nonnegative (zero means unlimited)")
	if _, err := workMemLiteral(cfg.WorkMem); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func LoadCrawlConfig() (CrawlConfig, error) {
	cfg := DefaultCrawlConfig()
	var errs []error
	read := func(name string, parse func(string) error) {
		if raw, ok := os.LookupEnv(name); ok {
			if err := parse(raw); err != nil {
				errs = append(errs, fmt.Errorf("openmcdb/modrinth: invalid %s=%q: %w", name, raw, err))
			}
		}
	}
	boolean := func(name string, dest *bool) {
		read(name, func(raw string) error {
			v, err := strconv.ParseBool(raw)
			if err == nil {
				*dest = v
			}
			return err
		})
	}
	integer := func(name string, dest *int) {
		read(name, func(raw string) error {
			v, err := strconv.Atoi(raw)
			if err == nil {
				*dest = v
			}
			return err
		})
	}
	duration := func(name string, dest *time.Duration) {
		read(name, func(raw string) error {
			v, err := time.ParseDuration(raw)
			if err == nil {
				*dest = v
			}
			return err
		})
	}
	text := func(name string, dest *string) {
		read(name, func(raw string) error { *dest = raw; return nil })
	}
	boolean("OPENMCDB_MODRINTH_ENABLED", &cfg.Enabled)
	text("OPENMCDB_MODRINTH_API_BASE", &cfg.APIBase)
	text("OPENMCDB_MODRINTH_USER_AGENT", &cfg.UserAgent)
	duration("OPENMCDB_MODRINTH_INTERVAL", &cfg.Interval)
	duration("OPENMCDB_MODRINTH_TIMEOUT", &cfg.Timeout)
	integer("OPENMCDB_MODRINTH_HOUR_UTC", &cfg.HourUTC)
	integer("OPENMCDB_MODRINTH_MINUTE_UTC", &cfg.MinuteUTC)
	boolean("OPENMCDB_MODRINTH_RUN_ON_START", &cfg.RunOnStart)
	integer("OPENMCDB_MODRINTH_RATE_LIMIT_PER_MINUTE", &cfg.RateLimitPerMinute)
	integer("OPENMCDB_MODRINTH_CONCURRENCY", &cfg.Concurrency)
	integer("OPENMCDB_MODRINTH_MAX_PROJECTS", &cfg.MaxProjects)
	text("OPENMCDB_MODRINTH_SCOPE_FACET", &cfg.ScopeFacet)
	text("OPENMCDB_MODRINTH_WORK_MEM", &cfg.WorkMem)
	errs = append(errs, ValidateCrawlConfig(cfg))
	return cfg, errors.Join(errs...)
}
