package core

import (
	"fmt"
	"github.com/InazumaV/V2bX/conf"
)

var configValidators = map[string]func(*conf.CoreConfig) error{}

func RegisterConfigValidator(kind string, validate func(*conf.CoreConfig) error) {
	configValidators[kind] = validate
}
func ValidateCoreConfigs(configs []conf.CoreConfig) error {
	for i := range configs {
		if validate := configValidators[configs[i].Type]; validate != nil {
			if err := validate(&configs[i]); err != nil {
				return fmt.Errorf("core %q: %w", configs[i].Name, err)
			}
		}
	}
	return nil
}
