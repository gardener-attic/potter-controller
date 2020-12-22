package admission

import "strings"

func containsSpiffTemplate(s string) bool {
	return strings.Contains(s, "((")
}
