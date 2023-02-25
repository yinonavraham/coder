package deployment

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/go-wordwrap"
)

func scalarNode(v any) (*yaml.Node, error) {
	var valueStr string
	switch v := v.(type) {
	case time.Duration:
		valueStr = v.String()
	case bool:
		valueStr = strconv.FormatBool(v)
	case string:
		valueStr = v
	case int:
		valueStr = strconv.Itoa(v)
	case int64:
		valueStr = strconv.Itoa(int(v))
	default:
		return nil, xerrors.Errorf(
			"unsupported scalar type: %T", v,
		)
	}
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: valueStr,
	}, nil
}

func valueOrDefault(v reflect.Value) reflect.Value {
	if val := v.FieldByName("Value"); !val.IsZero() {
		return val
	}
	return v.FieldByName("Default")
}

func yamlDeploymentField(field reflect.Value) (*yaml.Node, error) {
	valueKind := field.FieldByName("Value").Kind()
	effectiveValue := valueOrDefault(field)

	switch valueKind {
	case reflect.Slice:
		var content []*yaml.Node
		for i := 0; i < effectiveValue.Len(); i++ {
			vi := effectiveValue.Index(i)
			n, err := scalarNode(vi.Interface())
			if err != nil {
				return nil, xerrors.Errorf("converting scalar slice element: %w", err)
			}
			content = append(content, n)
		}
		return &yaml.Node{
			Kind:    yaml.SequenceNode,
			Content: content,
		}, nil
	case reflect.Bool, reflect.Int, reflect.Int64, reflect.String:
		return scalarNode(effectiveValue.Interface())
	default:
		return nil, xerrors.Errorf("unsupported kind: %s", valueKind.String())
	}
}

// MarshalYAML converts the deployment config to it's yaml representation.
// It accepts `any` because it calls itself recursively on its values.
func MarshalYAML(config any) (*yaml.Node, error) {
	var (
		document = &yaml.Node{
			Kind: yaml.MappingNode,
		}
		configValue = reflect.ValueOf(config)
		merr        *multierror.Error
	)

	if configValue.Kind() == reflect.Ptr {
		configValue = configValue.Elem()
	}

	for i := 0; i < configValue.NumField(); i++ {
		var (
			configField = configValue.Field(i).Elem()
			typeName    = configField.Type().String()
			fieldName   = configValue.Type().Field(i).Name
		)

		switch fieldName {
		case "ConfigPath", "WriteConfig":
			// These make no sense in the rendered YAML.
			continue
		}

		switch {
		case strings.HasPrefix(typeName, "codersdk.DeploymentConfigField["):
			if configField.FieldByName("Hidden").Bool() && configField.FieldByName("Value").IsZero() {
				continue
			}
			node, err := yamlDeploymentField(configField)
			if err != nil {
				merr = multierror.Append(merr, err)
				continue
			}
			comment := configField.FieldByName("Usage").String()

			if def := fmt.Sprintf("%v", configField.FieldByName("Default")); len(def) > 0 {
				comment += fmt.Sprintf(" (default: %+v)", def)
			}

			// Write field name.
			document.Content = append(document.Content, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: fieldName,
				HeadComment: wordwrap.WrapString(
					comment, 80,
				),
			})

			// Write node contents.
			document.Content = append(document.Content, node)
		case configField.Kind() == reflect.Struct:
			// Recursively resolve configuration group values.
			node, err := MarshalYAML(configField.Interface())
			if err != nil {
				merr = multierror.Append(
					merr,
					xerrors.Errorf("marshal group %s: %w", fieldName, err),
				)
				continue
			}
			// Write field name.
			document.Content = append(document.Content, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: fieldName,
			})
			document.Content = append(document.Content, node)
		default:
			merr = multierror.Append(merr, xerrors.Errorf("unsupported type: %s", typeName))
		}
	}
	return document, merr.ErrorOrNil()
}

func unmarshalScalar(v reflect.Value, node *yaml.Node) error {
	switch v.Kind() {
	case reflect.Int, reflect.Int64:
		i, err := strconv.Atoi(node.Value)
		if err != nil {
			return xerrors.Errorf("parsing int: %w", err)
		}
		v.SetInt(int64(i))
	case reflect.Bool:
		b, err := strconv.ParseBool(node.Value)
		if err != nil {
			return xerrors.Errorf("parsing bool: %w", err)
		}
		v.SetBool(b)
	case reflect.String:
		v.SetString(node.Value)
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return xerrors.Errorf("expected sequence node, got %s", node.Kind)
		}
		for i := 0; i < len(node.Content); i++ {
			vi := reflect.New(v.Type().Elem())
			if err := unmarshalScalar(vi.Elem(), node.Content[i]); err != nil {
				return xerrors.Errorf("unmarshaling slice element: %w", err)
			}
			v.Set(reflect.Append(v, vi.Elem()))
		}
	default:
		return xerrors.Errorf("unsupported kind: %s", v.Kind().String())
	}
	return nil
}

// UnmarshalYAML reads the deployment config (or a portion of it)
// from a yaml node.
func UnmarshalYAML(config any, body *yaml.Node) error {
	if body.Kind != yaml.MappingNode {
		return xerrors.Errorf("expected mapping node, got %s", body.Kind)
	}
	var (
		// YAML parsing switches between field name and field value mode.
		nameMode  = true
		fieldName string
	)
	for i := 0; i < len(body.Content); i++ {
		node := body.Content[i]
		if nameMode {
			if node.Kind != yaml.ScalarNode {
				return xerrors.Errorf(
					"expected scalar node (field name), got %s, value: %v",
					node.Kind, node.Value,
				)
			}
			fieldName = node.Value
			nameMode = false
		} else {
			// We're in value mode.
			field := reflect.ValueOf(config).Elem().FieldByName(fieldName)
			if field.IsZero() {
				return xerrors.Errorf("unknown field: %s", fieldName)
			}
			switch node.Kind {
			case yaml.ScalarNode:
				if err := unmarshalScalar(field, node); err != nil {
					return xerrors.Errorf("unmarshal scalar %v: %w", fieldName, err)
				}
			}
		}
	}
}
