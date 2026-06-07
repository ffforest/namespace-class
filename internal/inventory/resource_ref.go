package inventory

import (
	"fmt"
	"strings"
)

type ResourceRef struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

func (r ResourceRef) Key() (string, error) {
	if strings.TrimSpace(r.APIVersion) == "" {
		return "", fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(r.Kind) == "" {
		return "", fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return "", fmt.Errorf("name is required")
	}

	return strings.Join([]string{r.APIVersion, r.Kind, r.Namespace, r.Name}, "\x00"), nil
}
