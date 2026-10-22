package vault

import (
	"fmt"
	"strings"

	"github.com/miskawu/piper/internal/plan"
)

// ValidateProblemStatement checks that content contains a "不做什麼" section.
func ValidateProblemStatement(content string) error {
	if plan.ExtractSection(content, "不做什麼") == "" {
		return fmt.Errorf("must contain '## 不做什麼' section")
	}
	return nil
}

// ValidateADR checks that content contains "背景", "決定", and "後果" sections.
// Returns all missing sections at once so the caller can fix them in a single pass.
func ValidateADR(content string) error {
	required := []string{"背景", "決定", "後果"}
	var missing []string
	for _, section := range required {
		if plan.ExtractSection(content, section) == "" {
			missing = append(missing, "'## "+section+"'")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("must contain sections: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateContract parses a contract YAML and checks required fields.
// Phase 1 requires id/version/type; http-api additionally needs http.method + http.path.
func ValidateContract(content string) error {
	c, err := ParseContract([]byte(content))
	if err != nil {
		return err
	}
	var missing []string
	if strings.TrimSpace(c.ID) == "" {
		missing = append(missing, "id")
	}
	if strings.TrimSpace(c.Version) == "" {
		missing = append(missing, "version")
	}
	if strings.TrimSpace(string(c.Type)) == "" {
		missing = append(missing, "type")
	}
	if len(missing) > 0 {
		return fmt.Errorf("contract missing required fields: %s", strings.Join(missing, ", "))
	}

	switch c.Type {
	case ContractTypeHTTPAPI:
		if c.HTTP == nil {
			return fmt.Errorf("contract type http-api requires 'http' block")
		}
		var httpMissing []string
		if strings.TrimSpace(c.HTTP.Method) == "" {
			httpMissing = append(httpMissing, "http.method")
		}
		if strings.TrimSpace(c.HTTP.Path) == "" {
			httpMissing = append(httpMissing, "http.path")
		}
		if len(httpMissing) > 0 {
			return fmt.Errorf("contract missing required fields: %s", strings.Join(httpMissing, ", "))
		}
	case ContractTypeMessage, ContractTypeUIProps, ContractTypeDataSchema:
		// Phase 1 accepts these types without type-specific validation.
	default:
		return fmt.Errorf("unknown type %q (expected http-api, message, ui-props, data-schema)", c.Type)
	}
	return nil
}
