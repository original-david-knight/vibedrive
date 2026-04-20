package plan

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type StringList []string

func (l *StringList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		*l = nil
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: expected a list of strings, got %s", node.Line, nodeKind(node.Kind))
	}

	items := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		value, err := decodeStringListItem(item)
		if err != nil {
			return err
		}
		items = append(items, value)
	}

	*l = items
	return nil
}

func decodeStringListItem(node *yaml.Node) (string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value, nil
	case yaml.MappingNode:
		// YAML treats `- label: value` as a one-entry map. For plan fields that are
		// semantically string lists, flatten that back into the intended text.
		if len(node.Content) == 2 && node.Content[0].Kind == yaml.ScalarNode && node.Content[1].Kind == yaml.ScalarNode {
			key := node.Content[0].Value
			value := node.Content[1].Value
			if value == "" {
				return key + ":", nil
			}
			return key + ": " + value, nil
		}
		return "", fmt.Errorf("line %d: expected a string list item, got a mapping; quote strings containing ':'", node.Line)
	default:
		return "", fmt.Errorf("line %d: expected a string list item, got %s", node.Line, nodeKind(node.Kind))
	}
}

func nodeKind(kind yaml.Kind) string {
	switch kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "unknown"
	}
}
