package plugins

import (
	"encoding/json"
	"fmt"
	"math"
)

// Schema implements a documented, intentionally small JSON Schema subset.
type Schema struct {
	Type       string            `json:"type"`
	Properties map[string]Schema `json:"properties,omitempty"`
	Required   []string          `json:"required,omitempty"`
	Default    json.RawMessage   `json:"default,omitempty"`
	Enum       []any             `json:"enum,omitempty"`
	Minimum    *float64          `json:"minimum,omitempty"`
	Maximum    *float64          `json:"maximum,omitempty"`
	Items      *Schema           `json:"items,omitempty"`
}

func (s Schema) Validate(value any) (any, error) {
	if value == nil && s.Default != nil {
		if err := json.Unmarshal(s.Default, &value); err != nil {
			return nil, err
		}
	}
	switch s.Type {
	case "", "object":
		if value == nil {
			value = map[string]any{}
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object")
		}
		out := map[string]any{}
		for k, v := range obj {
			child, ok := s.Properties[k]
			if !ok {
				return nil, fmt.Errorf("unknown config field %s", k)
			}
			next, err := child.Validate(v)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = next
		}
		for k, child := range s.Properties {
			if _, ok := out[k]; !ok && child.Default != nil {
				next, err := child.Validate(nil)
				if err != nil {
					return nil, err
				}
				out[k] = next
			}
		}
		for _, k := range s.Required {
			if _, ok := out[k]; !ok {
				return nil, fmt.Errorf("required config field %s", k)
			}
		}
		value = out
	case "string":
		if _, ok := value.(string); !ok {
			return nil, fmt.Errorf("expected string")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("expected boolean")
		}
	case "number", "integer":
		n, ok := value.(float64)
		if !ok || (s.Type == "integer" && math.Trunc(n) != n) {
			return nil, fmt.Errorf("expected %s", s.Type)
		}
		if s.Minimum != nil && n < *s.Minimum || s.Maximum != nil && n > *s.Maximum {
			return nil, fmt.Errorf("number outside limits")
		}
	case "array":
		a, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("expected array")
		}
		if s.Items == nil {
			return nil, fmt.Errorf("array schema needs items")
		}
		out := make([]any, len(a))
		for i, v := range a {
			n, err := s.Items.Validate(v)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		value = out
	default:
		return nil, fmt.Errorf("unsupported schema type %s", s.Type)
	}
	if len(s.Enum) > 0 {
		a, _ := json.Marshal(value)
		for _, v := range s.Enum {
			b, _ := json.Marshal(v)
			if string(a) == string(b) {
				return value, nil
			}
		}
		return nil, fmt.Errorf("value not in enum")
	}
	return value, nil
}
