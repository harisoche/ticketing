package validation

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator wraps go-playground/validator for Echo.
type Validator struct {
	v *validator.Validate
}

func NewValidator() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return &Validator{v: v}
}

func (vw *Validator) Validate(i any) error {
	return vw.v.Struct(i)
}

// FormatErrors turns validator.ValidationErrors into a {field: [messages]} map.
func FormatErrors(err error) map[string][]string {
	out := map[string][]string{}
	verrs, ok := err.(validator.ValidationErrors)
	if !ok {
		return out
	}
	for _, fe := range verrs {
		field := fe.Field()
		out[field] = append(out[field], messageFor(fe))
	}
	return out
}

func messageFor(fe validator.FieldError) string {
	field := fe.Field()
	switch fe.Tag() {
	case "required":
		return field + " is required"
	case "email":
		return field + " must be a valid email address"
	case "min":
		return field + " must be at least " + fe.Param() + " characters long"
	case "max":
		return field + " must be at most " + fe.Param() + " characters long"
	}
	return field + " is invalid"
}
