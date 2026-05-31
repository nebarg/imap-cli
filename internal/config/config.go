// Package config loads IMAP connection settings from the environment / .env file.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Account holds the connection settings for a single IMAP account.
type Account struct {
	Name     string // logical name ("" for the default account)
	Host     string
	Port     int
	Username string
	Password string
	TLS      bool // true: implicit TLS (port 993); false: STARTTLS (port 143)
}

// Addr returns the host:port dial address.
func (a Account) Addr() string {
	return fmt.Sprintf("%s:%d", a.Host, a.Port)
}

// Load reads configuration from envPath (if it exists) and the process
// environment, returning the requested account.
//
// Variables are read as IMAP_HOST, IMAP_PORT, IMAP_USERNAME, IMAP_PASSWORD,
// IMAP_TLS. To support multiple accounts, set a non-empty account name and
// define IMAP_<ACCOUNT>_HOST etc.; any value not set for the named account
// falls back to the unprefixed default. This keeps single-account setups
// trivial while leaving multi-account support a matter of configuration.
func Load(envPath, account string) (Account, error) {
	// Load .env if present. Missing file is not an error: real environment
	// variables may supply everything.
	if envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			if err := godotenv.Load(envPath); err != nil {
				return Account{}, fmt.Errorf("loading %s: %w", envPath, err)
			}
		}
	}

	get := func(key string) string {
		if account != "" {
			prefixed := fmt.Sprintf("IMAP_%s_%s", strings.ToUpper(account), key)
			if v := os.Getenv(prefixed); v != "" {
				return v
			}
		}
		return os.Getenv("IMAP_" + key)
	}

	acc := Account{
		Name:     account,
		Host:     get("HOST"),
		Username: get("USERNAME"),
		Password: get("PASSWORD"),
		Port:     993,
		TLS:      true,
	}

	if v := get("TLS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Account{}, fmt.Errorf("invalid IMAP_TLS value %q: %w", v, err)
		}
		acc.TLS = b
	}
	// Default the port based on the transport when not explicitly set.
	if !acc.TLS {
		acc.Port = 143
	}
	if v := get("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return Account{}, fmt.Errorf("invalid IMAP_PORT value %q: %w", v, err)
		}
		acc.Port = p
	}

	var missing []string
	if acc.Host == "" {
		missing = append(missing, "IMAP_HOST")
	}
	if acc.Username == "" {
		missing = append(missing, "IMAP_USERNAME")
	}
	if acc.Password == "" {
		missing = append(missing, "IMAP_PASSWORD")
	}
	if len(missing) > 0 {
		return Account{}, fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}

	return acc, nil
}
