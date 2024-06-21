package compose

import (
	"fmt"

	"github.com/spf13/cobra"
)

// EnumValue struct to hold enumeration.
type EnumValue struct {
	Enum         []string
	defaultValue string
	value        *string
}

// Set sets EnumValue.
func (e *EnumValue) Set(value string) error {
	for _, allowed := range e.Enum {
		if value == allowed {
			*e.value = value
			return nil
		}
	}
	return fmt.Errorf("expected one of %v, got '%s'", e.Enum, value)
}

// Type returns a string name for this EnumValue type
func (e *EnumValue) Type() string {
	return "string"
}

func (e *EnumValue) String() string {
	return *e.value
}

// EnumVarP adds custom enum flag to cobra command.
func EnumVarP(cmd *cobra.Command, value *string, name, shorthand string, defaultValue string, enumValues []string, usage string) {
	ev := &EnumValue{
		Enum:         enumValues,
		defaultValue: defaultValue,
		value:        value,
	}

	*ev.value = ev.defaultValue
	cmd.Flags().VarP(ev, name, shorthand, usage)
}
