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

type yamlSchema map[string]interface{}

func yamlDeploymentField(field reflect.Value) (*yaml.Node, error) {
	valueField := field.FieldByName("Value")
	var valueStr string
	switch v := valueField.Interface().(type) {
	case time.Duration:
		valueStr = v.String()
	case bool:
		valueStr = strconv.FormatBool(v)
	case string:
		valueStr = v
	default:
		return nil, xerrors.Errorf(
			"unsupported DeploymentField type: %s", valueField.Type().String(),
		)
	}
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: valueStr,
	}, nil
}

// MarshalYAML converts the deployment config to its MarshalYAML representation.
func MarshalYAML(config *codersdk.DeploymentConfig) ([]byte, error) {
	var (
		schema      = yamlSchema{}
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
			node, err := yamlDeploymentField(configField)
			if err != nil {
				merr = multierror.Append(merr, err)
				continue
			}
			schema[fieldName] = node
		default:
			merr = multierror.Append(merr, xerrors.Errorf("unsupported type: %s", typeName))
		}
	}
	byt, err := yaml.Marshal(schema)
	merr = multierror.Append(merr, err)
	return byt, merr.ErrorOrNil()
}
