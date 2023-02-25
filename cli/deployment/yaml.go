package deployment

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/hashicorp/go-multierror"

	"github.com/coder/coder/codersdk"
)

// func goNameToYAML(name string) string {
// }

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

func yamlDeploymentField(field reflect.Value) (*yaml.Node, error) {
	valueField := field.FieldByName("Value")
	valueKind := valueField.Kind()

	switch valueKind {
	case reflect.Slice:
		var content []*yaml.Node
		for i := 0; i < valueField.Len(); i++ {
			vi := valueField.Index(i)
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
		return scalarNode(valueField.Interface())
	default:
		return nil, xerrors.Errorf("unsupported kind: %s", valueKind.String())
	}
}

// MarshalYAML converts the deployment config to its MarshalYAML representation.
func MarshalYAML(config *codersdk.DeploymentConfig) ([]byte, error) {
	var (
		document = yaml.Node{
			Kind: yaml.MappingNode,
		}
		configValue = reflect.ValueOf(config).Elem()
		merr        *multierror.Error
	)
	for i := 0; i < configValue.NumField(); i++ {
		var (
			configField = configValue.Field(i).Elem()
			typeName    = configField.Type().String()
			fieldName   = configValue.Type().Field(i).Name
		)

		switch {
		case strings.HasPrefix(typeName, "codersdk.DeploymentConfigField["):
			// Write field name.
			document.Content = append(document.Content, &yaml.Node{
				Kind:        yaml.ScalarNode,
				Value:       fieldName,
				HeadComment: configField.FieldByName("Usage").String(),
			})
			node, err := yamlDeploymentField(configField)
			if err != nil {
				merr = multierror.Append(merr, err)
				continue
			}
			// Write node contents.
			document.Content = append(document.Content, node)
		default:
			merr = multierror.Append(merr, xerrors.Errorf("unsupported type: %s", typeName))
		}
	}
	byt, err := yaml.Marshal(document)
	if err != nil {
		merr = multierror.Append(
			merr, xerrors.Errorf("marshal failed: %w\n%+v", err, document),
		)
	}
	return byt, merr.ErrorOrNil()
}
