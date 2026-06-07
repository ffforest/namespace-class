package template

import (
	"fmt"
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`{{\s*([^{}]+?)\s*}}`)

func RenderString(input string, values map[string]string) (string, error) {
	var missing []string
	rendered := tokenPattern.ReplaceAllStringFunc(input, func(token string) string {
		matches := tokenPattern.FindStringSubmatch(token)
		if len(matches) != 2 {
			missing = append(missing, token)
			return token
		}

		key := strings.TrimSpace(matches[1])
		value, ok := values[key]
		if !ok {
			missing = append(missing, key)
			return token
		}
		return value
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("missing template values: %s", strings.Join(missing, ", "))
	}
	return rendered, nil
}
