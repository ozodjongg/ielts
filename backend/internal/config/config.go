package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func String(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
func Required(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		panic(fmt.Sprintf("missing required environment variable %s", key))
	}
	return v
}
func Int(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("invalid %s: %v", key, err))
	}
	return n
}
func Bool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		panic(fmt.Sprintf("invalid %s: %v", key, err))
	}
	return b
}
func Duration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("invalid %s: %v", key, err))
	}
	return d
}
func CSV(key string) []string {
	raw := String(key, "")
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
