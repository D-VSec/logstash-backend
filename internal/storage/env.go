package storage

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open environment file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(name) == "" {
			return fmt.Errorf("invalid environment assignment on line %d", lineNumber)
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		if _, exists := os.LookupEnv(name); !exists {
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("set environment variable %s: %w", name, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read environment file: %w", err)
	}
	return nil
}
