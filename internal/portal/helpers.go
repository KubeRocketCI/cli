package portal

func nestedString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}

	v, ok := m[key]
	if !ok {
		return ""
	}

	s, ok := v.(string)
	if !ok {
		return ""
	}

	return s
}

func nestedMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}

	v, ok := m[key]
	if !ok {
		return nil
	}

	nested, ok := v.(map[string]any)
	if !ok {
		return nil
	}

	return nested
}

// traversePath walks a nested map along the given key path and returns the
// value at the final key. Returns (nil, false) if any intermediate key is missing.
func traversePath(m map[string]any, keys ...string) (any, bool) {
	if len(keys) == 0 {
		return nil, false
	}

	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			break
		}

		current = nestedMap(current, key)
		if current == nil {
			return nil, false
		}
	}

	raw, ok := current[keys[len(keys)-1]]
	return raw, ok
}

func nestedStringSlice(m map[string]any, keys ...string) []string {
	raw, ok := traversePath(m, keys...)
	if !ok {
		return nil
	}

	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(items))

	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}

		result = append(result, s)
	}

	return result
}

func nestedInt64(m map[string]any, keys ...string) int64 {
	raw, ok := traversePath(m, keys...)
	if !ok {
		return 0
	}

	switch v := raw.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func isAvailable(m map[string]any) bool {
	if m == nil {
		return false
	}

	v, ok := m["available"]
	if !ok {
		return false
	}

	b, ok := v.(bool)
	if !ok {
		return false
	}

	return b
}
