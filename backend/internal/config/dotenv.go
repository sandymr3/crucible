package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv reads a .env file into the process environment for local
// development. Existing environment variables always win, so an explicit
// export or a Cloud Run service variable is never silently overridden by a
// stale file left in the working directory.
//
// A missing file is not an error: on Cloud Run there is no .env, and that is
// the expected case rather than a degraded one.
//
// This is a deliberately small implementation rather than a dependency. It
// handles what a config file needs — comments, blank lines, optional "export"
// prefix, and quoted values — and nothing more.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Strip matched surrounding quotes, so a value containing '#' or
		// leading whitespace can be expressed.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if key == "" {
			continue
		}
		if _, present := os.LookupEnv(key); present {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
