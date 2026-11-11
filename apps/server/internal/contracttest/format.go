package contracttest

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// validateFormats checks the date formats used in the OpenAPI schemas after
// jsonschema-go validates the remaining constraints.
func (c *Contract) validateFormats(schema map[string]any, value any) error {
	if value == nil {
		return nil
	}
	if ref, ok := schema["$ref"].(string); ok {
		const prefix = "#/components/schemas/"
		if !strings.HasPrefix(ref, prefix) {
			return fmt.Errorf("unsupported schema reference %q", ref)
		}
		target, exists := c.Components.Schemas[strings.TrimPrefix(ref, prefix)]
		if !exists {
			return fmt.Errorf("unknown schema reference %q", ref)
		}
		if err := c.validateFormats(target, value); err != nil {
			return err
		}
	}
	if text, ok := value.(string); ok {
		format, _ := schema["format"].(string)
		switch format {
		case "date":
			if !matches(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`, text) {
				return fmt.Errorf("invalid date %q", text)
			}
			if _, err := time.Parse(time.DateOnly, text); err != nil {
				return fmt.Errorf("invalid date %q: %w", text, err)
			}
		case "date-time":
			if err := validateDateTime(text); err != nil {
				return err
			}
		}
	}
	switch instance := value.(type) {
	case map[string]any:
		properties, _ := schema["properties"].(map[string]any)
		for key, item := range instance {
			property, ok := properties[key].(map[string]any)
			if !ok {
				property, _ = schema["additionalProperties"].(map[string]any)
			}
			if property != nil {
				if err := c.validateFormats(property, item); err != nil {
					return fmt.Errorf("property %s: %w", key, err)
				}
			}
		}
	case []any:
		itemSchema, _ := schema["items"].(map[string]any)
		if itemSchema != nil {
			for i, item := range instance {
				if err := c.validateFormats(itemSchema, item); err != nil {
					return fmt.Errorf("item %d: %w", i, err)
				}
			}
		}
	}
	if branches, ok := schema["allOf"].([]any); ok {
		for _, branch := range branches {
			if item, ok := branch.(map[string]any); ok {
				if err := c.validateFormats(item, value); err != nil {
					return err
				}
			}
		}
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		branches, ok := schema[key].([]any)
		if !ok {
			continue
		}
		matched := false
		for _, branch := range branches {
			if item, ok := branch.(map[string]any); ok && c.validate(item, value) == nil {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("no %s schema matches the formatted value", key)
		}
	}
	return nil
}

func matches(pattern, value string) bool { ok, _ := regexp.MatchString(pattern, value); return ok }

func validateDateTime(value string) error {
	const pattern = `^[0-9]{4}-[0-9]{2}-[0-9]{2}[Tt][0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?([Zz]|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`
	if !matches(pattern, value) {
		return fmt.Errorf("invalid date-time %q", value)
	}
	normalized := strings.ToUpper(value)
	// time.Parse rejects leap seconds, so check the preceding second and its UTC date.
	leap := normalized[17:19] == "60"
	if leap {
		normalized = normalized[:17] + "59" + normalized[19:]
	}
	parsed, err := time.Parse(time.RFC3339Nano, normalized)
	if err != nil {
		return fmt.Errorf("invalid date-time %q: %w", value, err)
	}
	if leap {
		utc := parsed.UTC()
		leapDay := utc.AddDate(0, 0, 1).Day() == 1
		if utc.Hour() != 23 || utc.Minute() != 59 || !leapDay {
			return fmt.Errorf("invalid leap second %q", value)
		}
	}
	return nil
}
