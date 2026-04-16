package render

import (
	"bytes"
	"text/template"
)

func String(tmpl string, data any) (string, error) {
	parsed, err := template.New("value").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	if err := parsed.Execute(&out, data); err != nil {
		return "", err
	}

	return out.String(), nil
}

func Strings(values []string, data any) ([]string, error) {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		item, err := String(value, data)
		if err != nil {
			return nil, err
		}
		rendered = append(rendered, item)
	}

	return rendered, nil
}

func Map(values map[string]string, data any) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	rendered := make(map[string]string, len(values))
	for key, value := range values {
		keyOut, err := String(key, data)
		if err != nil {
			return nil, err
		}
		valueOut, err := String(value, data)
		if err != nil {
			return nil, err
		}
		rendered[keyOut] = valueOut
	}

	return rendered, nil
}
