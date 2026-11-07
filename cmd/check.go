package cmd

import (
	"fmt"

	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/spf13/cobra"
)

func newCheckCommand() *cobra.Command {
	var configPath string
	c := &cobra.Command{
		Use: "check", Short: "Validate local configuration without contacting Xboard or starting listeners", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := conf.New()
			if err := c.LoadFromPath(configPath); err != nil {
				return err
			}
			if err := c.Validate(); err != nil {
				return err
			}
			if err := checkCompiledCores(c); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Local configuration OK: %d cores, %d nodes. Panel ports, DNS, firewall and proxy connectivity are not checked.\n", len(c.CoresConfig), len(c.NodeConfig))
			return nil
		},
	}
	c.Flags().StringVarP(&configPath, "config", "c", "/etc/V2bX/config.json", "config file path")
	return c
}

func checkCompiledCores(c *conf.Conf) error {
	if err := checkCompiledCoreTypes(c); err != nil {
		return err
	}
	return vCore.ValidateCoreConfigs(c.CoresConfig)
}

func checkCompiledCoreTypes(c *conf.Conf) error {
	compiled := make(map[string]bool)
	for _, name := range vCore.RegisteredCore() {
		compiled[name] = true
	}
	for _, core := range c.CoresConfig {
		if !compiled[core.Type] {
			return fmt.Errorf("core %q is not included in this binary", core.Type)
		}
	}
	return nil
}

func init() { command.AddCommand(newCheckCommand()) }
