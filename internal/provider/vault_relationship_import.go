package provider

import "strings"

func vaultRelationshipImportID(value string) (string, string, bool) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}
