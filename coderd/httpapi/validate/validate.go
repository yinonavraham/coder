package validate

import (
	"fmt"
	"sync"

	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"
)

type Validator struct {
	*validator.Validate
}

func New() *Validator {
	return &Validator{
		Validate: validator.New(),
	}
}

type customErrors struct {
	Exported any `json:""` // Any is the actual struct to validate

	sync.Mutex
	errors map[string]error
}

func (ce *customErrors) AddError(field string, err error) {
	ce.Lock()
	defer ce.Unlock()

	ce.errors[field] = err
}

// DetailedFieldError includes a custom "reason" error to explain why the
// validation failed.
type DetailedFieldError struct {
	validator.FieldError
	Reason error
}

// Struct overrides the default Struct method to allow for custom errors.
func (v *Validator) Struct(value interface{}) error {
	c := &customErrors{
		errors:   make(map[string]error),
		Exported: value,
	}
	err := v.Validate.Struct(c)
	if err == nil && len(c.errors) == 0 {
		return nil
	}

	var validErrors validator.ValidationErrors
	if xerrors.As(err, &validErrors) {
		for i, ve := range validErrors {
			fieldName := ve.Namespace()
			if reason, ok := c.errors[fieldName]; ok {
				validErrors[i] = DetailedFieldError{
					FieldError: ve,
					Reason:     reason,
				}
				delete(c.errors, fieldName)
			}
		}
		if len(c.errors) > 0 {
			panic(fmt.Sprintf("%d custom errors remain: %v", len(c.errors), c.errors))
		}
		return validErrors
	}
	return err
}

type FuncWithError func(fl validator.FieldLevel) error

// RegisterValidation adds a validation with the given tag
//
// NOTES:
// - if the key already exists, the previous validation function will be replaced.
// - this method is not thread-safe it is intended that these all be registered prior to any validation
func (v *Validator) RegisterValidation(tag string, fn FuncWithError, callValidationEvenIfNull ...bool) error {
	return v.Validate.RegisterValidation(tag, func(fl validator.FieldLevel) bool {
		err := fn(fl)
		if err != nil {
			top := fl.Top().Interface()
			ce, ok := (top).(*customErrors)
			if ok {
				// We cannot get the full namespace resolution. So hopefully
				// the field name with the parent type is unique enough.
				namespace := fmt.Sprintf("%s.%s=%v",
					fl.Parent().Type().Name(), fl.FieldName(), fl.Field().Interface())
				ce.AddError(namespace, err)
				// Always return false, because this error will be added after.
				return true
			}
			return false
		}
		return true
	}, callValidationEvenIfNull...)
}
