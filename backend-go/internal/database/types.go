package database

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999+00:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05+00:00",
	"2006-01-02 15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02",
}

func ParseTime(val any) (time.Time, error) {
	if val == nil {
		return time.Time{}, nil
	}
	switch v := val.(type) {
	case time.Time:
		return v.UTC(), nil
	case []byte:
		return parseTimeString(string(v))
	case string:
		return parseTimeString(v)
	default:
		return parseTimeString(fmt.Sprintf("%v", v))
	}
}

func parseTimeString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %q", s)
}

func ParseNullTime(val any) (*time.Time, error) {
	if val == nil {
		return nil, nil
	}
	if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
		return nil, nil
	}
	if b, ok := val.([]byte); ok && len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}
	t, err := ParseTime(val)
	if err != nil {
		return nil, err
	}
	if t.IsZero() {
		return nil, nil
	}
	return &t, nil
}

func ParseDateString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case time.Time:
		return v.Format("2006-01-02")
	case []byte:
		s := strings.TrimSpace(string(v))
		if len(s) >= 10 {
			return s[:10]
		}
		return s
	case string:
		s := strings.TrimSpace(v)
		if len(s) >= 10 {
			return s[:10]
		}
		return s
	default:
		s := fmt.Sprintf("%v", v)
		if len(s) >= 10 {
			return s[:10]
		}
		return s
	}
}

func ParseUUID(val any) (uuid.UUID, error) {
	if val == nil {
		return uuid.Nil, fmt.Errorf("nil uuid")
	}
	switch v := val.(type) {
	case uuid.UUID:
		return v, nil
	case [16]byte:
		return uuid.UUID(v), nil
	case []byte:
		if len(v) == 16 {
			return uuid.FromBytes(v)
		}
		return uuid.Parse(strings.TrimSpace(string(v)))
	case string:
		return uuid.Parse(strings.TrimSpace(v))
	default:
		return uuid.Parse(strings.TrimSpace(fmt.Sprintf("%v", v)))
	}
}

func ParseNullUUID(val any) (*uuid.UUID, error) {
	if val == nil {
		return nil, nil
	}
	if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
		return nil, nil
	}
	if b, ok := val.([]byte); ok && len(b) == 0 {
		return nil, nil
	}
	u, err := ParseUUID(val)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func ParseNullString(val any) *string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case string:
		s := v
		return &s
	case []byte:
		s := string(v)
		return &s
	default:
		s := fmt.Sprintf("%v", v)
		return &s
	}
}

func ParseBool(val any) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	case []byte:
		s := strings.ToLower(strings.TrimSpace(string(v)))
		return s == "1" || s == "true" || s == "t"
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "1" || s == "true" || s == "t"
	default:
		return false
	}
}

func ParseJSONMap(val any) map[string]any {
	if val == nil {
		return nil
	}
	var raw []byte
	switch v := val.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return nil
	}
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func ParseJSONAny(val any) any {
	if val == nil {
		return nil
	}
	var raw []byte
	switch v := val.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return nil
	}
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
