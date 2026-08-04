// Package httpheader contains HTTP header helpers shared by protocol adapters.
package httpheader

import "net/http"

// Merge combines header maps in order, with later layers overriding earlier
// layers case-insensitively. Returned names use Go's canonical HTTP spelling.
func Merge(layers ...map[string]string) map[string]string {
	size := 0
	for _, layer := range layers {
		size += len(layer)
	}
	if size == 0 {
		return nil
	}

	merged := make(map[string]string, size)
	for _, layer := range layers {
		for name, value := range layer {
			merged[http.CanonicalHeaderKey(name)] = value
		}
	}
	return merged
}
