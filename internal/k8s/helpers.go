package k8s

// Status constants shared across KubeRocketCI CRD resources.
const (
	StatusCreated    = "created"
	StatusInProgress = "in_progress"
	StatusFailed     = "failed"
)

const availableKey = "available"

// nestedString safely extracts a string value from a map.
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

// isAvailable extracts the "available" boolean from a status map.
func isAvailable(m map[string]any) bool {
	if m == nil {
		return false
	}

	v, ok := m[availableKey]
	if !ok {
		return false
	}

	b, ok := v.(bool)
	if !ok {
		return false
	}

	return b
}
