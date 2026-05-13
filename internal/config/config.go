package config

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultBind     = "127.0.0.1:8080"
	DefaultTimezone = "Asia/Seoul"
	DefaultWorkers  = 3
)

type Config struct {
	DataDir     string   `json:"data_dir"`
	DBPath      string   `json:"db_path"`
	Bind        string   `json:"bind"`
	Token       string   `json:"-"`
	CORS        []string `json:"cors"`
	Workers     int      `json:"workers"`
	Timezone    string   `json:"timezone"`
	BackupTo    string   `json:"-"`
	RestoreFrom string   `json:"-"`
}

func Default() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	dataDir := filepath.Join(home, ".corn-agent-dashboard")
	return Config{
		DataDir:  dataDir,
		DBPath:   filepath.Join(dataDir, "data.db"),
		Bind:     DefaultBind,
		CORS:     []string{"http://127.0.0.1:5173", "http://localhost:5173"},
		Workers:  DefaultWorkers,
		Timezone: DefaultTimezone,
	}, nil
}

func Load(args []string) (Config, []string, error) {
	cfg, err := Default()
	if err != nil {
		return Config{}, nil, err
	}
	applyEnv(&cfg)
	explicitDBPath := os.Getenv("CORN_AGENT_DASHBOARD_DB") != ""

	fs := flag.NewFlagSet("corn-agent-dashboard", flag.ContinueOnError)
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "data directory")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	fs.StringVar(&cfg.Bind, "bind", cfg.Bind, "HTTP bind address")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "optional bearer token")
	cors := fs.String("cors", strings.Join(cfg.CORS, ","), "comma-separated CORS allowed origins")
	fs.IntVar(&cfg.Workers, "workers", cfg.Workers, "worker pool size")
	fs.StringVar(&cfg.Timezone, "timezone", cfg.Timezone, "system timezone")
	fs.StringVar(&cfg.BackupTo, "to", cfg.BackupTo, "backup destination path")
	fs.StringVar(&cfg.RestoreFrom, "from", cfg.RestoreFrom, "restore source path")
	if err := fs.Parse(args); err != nil {
		return Config{}, nil, err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			explicitDBPath = true
		}
	})
	cfg.CORS = splitCSV(*cors)
	if !explicitDBPath || cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "data.db")
	}
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.Timezone == "" {
		cfg.Timezone = DefaultTimezone
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, nil, err
	}
	return cfg, fs.Args(), nil
}

func (c Config) Validate() error {
	if c.DataDir == "" {
		return errors.New("data dir is required")
	}
	if c.DBPath == "" {
		return errors.New("db path is required")
	}
	if c.Bind == "" {
		return errors.New("bind address is required")
	}
	if c.Workers <= 0 {
		return errors.New("workers must be positive")
	}
	if exposesNonLocalhost(c.Bind) && c.Token == "" {
		return errors.New("token is required when binding outside localhost")
	}
	return nil
}

func (c Config) AuthMode() string {
	if c.Token == "" {
		return "none"
	}
	return "token"
}

func EnsureDirs(c Config) error {
	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(c.DataDir, "runs"), 0o755)
}

func applyEnv(c *Config) {
	setString(&c.DataDir, "CORN_AGENT_DASHBOARD_DATA_DIR")
	setString(&c.DBPath, "CORN_AGENT_DASHBOARD_DB")
	setString(&c.Bind, "CORN_AGENT_DASHBOARD_BIND")
	setString(&c.Token, "CORN_AGENT_DASHBOARD_TOKEN")
	setString(&c.Timezone, "CORN_AGENT_DASHBOARD_TIMEZONE")
	if v := os.Getenv("CORN_AGENT_DASHBOARD_CORS"); v != "" {
		c.CORS = splitCSV(v)
	}
	if v := os.Getenv("CORN_AGENT_DASHBOARD_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Workers = n
		}
	}
}

func setString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func exposesNonLocalhost(bind string) bool {
	host := bind
	if i := strings.LastIndex(bind, ":"); i >= 0 {
		host = bind[:i]
	}
	host = strings.Trim(host, "[]")
	return host == "" || host == "0.0.0.0" || host == "::" || (!strings.HasPrefix(host, "127.") && host != "localhost" && host != "::1")
}
