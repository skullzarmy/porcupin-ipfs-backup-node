package config

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// KeyValue represents a single configuration key-value pair
type KeyValue struct {
	Key   string
	Value string
}

// NormalizeDotKey converts dashes to underscores in dot-notation keys
// so both "backup.max-concurrency" and "backup.max_concurrency" work.
func NormalizeDotKey(key string) string {
	return strings.ReplaceAll(key, "-", "_")
}

// GetByDotNotation retrieves a config value by dot-notation key.
// Example: "backup.max_concurrency" returns "5"
func GetByDotNotation(cfg *Config, key string) (string, error) {
	key = NormalizeDotKey(key)

	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid key %q: must be in section.field format (e.g. backup.max_concurrency)", key)
	}

	section, field := parts[0], parts[1]

	sectionVal, err := findSection(cfg, section)
	if err != nil {
		return "", err
	}

	fieldVal, err := findField(sectionVal, field)
	if err != nil {
		return "", fmt.Errorf("unknown key %q in section %q. Run 'porcupin settings list' to see all keys", field, section)
	}

	return formatValue(fieldVal), nil
}

// SetByDotNotation sets a config value by dot-notation key.
// The value string is automatically parsed to the correct type.
func SetByDotNotation(cfg *Config, key string, value string) error {
	key = NormalizeDotKey(key)

	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid key %q: must be in section.field format (e.g. backup.max_concurrency)", key)
	}

	section, field := parts[0], parts[1]

	sectionVal, err := findSectionPtr(cfg, section)
	if err != nil {
		return err
	}

	fieldVal, err := findField(sectionVal, field)
	if err != nil {
		return fmt.Errorf("unknown key %q in section %q. Run 'porcupin settings list' to see all keys", field, section)
	}

	if !fieldVal.CanSet() {
		return fmt.Errorf("cannot set key %q", key)
	}

	return parseAndSet(fieldVal, value, key)
}

// ListAll returns all config settings as flat dot-notation key-value pairs.
func ListAll(cfg *Config) []KeyValue {
	var result []KeyValue

	cfgVal := reflect.ValueOf(cfg).Elem()
	cfgType := cfgVal.Type()

	for i := 0; i < cfgType.NumField(); i++ {
		sectionField := cfgType.Field(i)
		sectionTag := sectionField.Tag.Get("yaml")
		if sectionTag == "" || sectionTag == "-" {
			continue
		}
		sectionVal := cfgVal.Field(i)
		collectFields(sectionTag, sectionVal, &result)
	}

	return result
}

// ValidKeys returns all valid dot-notation keys.
func ValidKeys() []string {
	cfg := DefaultConfig()
	items := ListAll(cfg)
	keys := make([]string, len(items))
	for i, item := range items {
		keys[i] = item.Key
	}
	sort.Strings(keys)
	return keys
}

// collectFields recursively collects fields from a struct value
func collectFields(prefix string, val reflect.Value, result *[]KeyValue) {
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	valType := val.Type()

	for i := 0; i < valType.NumField(); i++ {
		field := valType.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		// Strip yaml options like ",omitempty"
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}

		fieldVal := val.Field(i)
		fullKey := prefix + "." + tag

		// If this is a nested struct, recurse
		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeOf(time.Duration(0)) {
			collectFields(fullKey, fieldVal, result)
			continue
		}

		*result = append(*result, KeyValue{
			Key:   fullKey,
			Value: formatValue(fieldVal),
		})
	}
}

// findSection finds a top-level section in the config by yaml tag name (read-only)
func findSection(cfg *Config, section string) (reflect.Value, error) {
	cfgVal := reflect.ValueOf(cfg).Elem()
	return findSectionIn(cfgVal, section)
}

// findSectionPtr finds a top-level section in the config by yaml tag name (writable)
func findSectionPtr(cfg *Config, section string) (reflect.Value, error) {
	cfgVal := reflect.ValueOf(cfg).Elem()
	return findSectionIn(cfgVal, section)
}

func findSectionIn(cfgVal reflect.Value, section string) (reflect.Value, error) {
	cfgType := cfgVal.Type()
	for i := 0; i < cfgType.NumField(); i++ {
		field := cfgType.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == section {
			return cfgVal.Field(i), nil
		}
	}
	return reflect.Value{}, fmt.Errorf("unknown section %q. Valid sections: ipfs, server, backup, tzkt, api", section)
}

// findField finds a field in a struct section by yaml tag name.
// Supports nested structs (e.g. "tls.cert" within the api section).
func findField(sectionVal reflect.Value, field string) (reflect.Value, error) {
	if sectionVal.Kind() == reflect.Ptr {
		sectionVal = sectionVal.Elem()
	}

	// Check for nested dot notation (e.g. api.tls.cert)
	parts := strings.SplitN(field, ".", 2)

	sectionType := sectionVal.Type()
	for i := 0; i < sectionType.NumField(); i++ {
		f := sectionType.Field(i)
		tag := f.Tag.Get("yaml")
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		if tag == parts[0] {
			if len(parts) == 2 && f.Type.Kind() == reflect.Struct {
				// Recurse into nested struct
				return findField(sectionVal.Field(i), parts[1])
			}
			return sectionVal.Field(i), nil
		}
	}
	return reflect.Value{}, fmt.Errorf("field %q not found", field)
}

// formatValue converts a reflect.Value to its string representation
func formatValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int64:
		// Special case for time.Duration
		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			return v.Interface().(time.Duration).String()
		}
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// parseAndSet parses a string value and sets it on the target field
func parseAndSet(field reflect.Value, value string, key string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		return nil

	case reflect.Int, reflect.Int64:
		// Special case for time.Duration
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid duration for %q: %w (example: 2m, 5m30s, 1h)", key, err)
			}
			field.Set(reflect.ValueOf(d))
			return nil
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer for %q: %w", key, err)
		}
		field.SetInt(n)
		return nil

	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for %q: %w (use true/false)", key, err)
		}
		field.SetBool(b)
		return nil

	default:
		return fmt.Errorf("unsupported type %s for key %q", field.Type(), key)
	}
}
