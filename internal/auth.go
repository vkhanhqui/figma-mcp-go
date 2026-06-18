package internal

import (
	"crypto/subtle"
	"net/http"
	"sort"
	"strings"
)

const maxRPCBodyBytes int64 = 16 * 1024 * 1024

func requestAuthorized(r *http.Request, authToken string) bool {
	if authToken == "" {
		return true
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(authToken)) == 1
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func writeUnauthorized(w http.ResponseWriter) {
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func paramKeys(params map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
