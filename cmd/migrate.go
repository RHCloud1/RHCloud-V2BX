package cmd

import (
	"fmt"
	"os"

	"github.com/InazumaV/V2bX/common/singconfig"
	"github.com/InazumaV/V2bX/conf"
	"github.com/spf13/cobra"
)

func newMigrateCommand() *cobra.Command {
	var path string
	var write bool
	c := &cobra.Command{Use: "migrate-sing", Short: "Preview supported sing-box configuration migrations; --write backs up and replaces the file", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			updated, changes, err := singconfig.Migrate(data)
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No supported legacy fields found.")
				return nil
			}
			for _, change := range changes {
				fmt.Fprintln(cmd.OutOrStdout(), "- "+change)
			}
			f, err := os.CreateTemp("", "v2bx-migrate-*.json")
			if err != nil {
				return err
			}
			defer os.Remove(f.Name())
			if _, err = f.Write(updated); err != nil {
				f.Close()
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
			c := &conf.Conf{CoresConfig: []conf.CoreConfig{{Type: "sing", SingConfig: &conf.SingConfig{OriginalPath: f.Name()}}}}
			if err := checkCompiledCores(c); err != nil {
				return fmt.Errorf("migrated configuration was not accepted: %w", err)
			}
			if !write {
				fmt.Fprintln(cmd.OutOrStdout(), "Preview only; use --write to save with a timestamped backup.")
				return nil
			}
			if err := writeConfigBundle(map[string][]byte{path: updated}, path); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Migration saved; previous configuration backed up.")
			return nil
		}}
	c.Flags().StringVarP(&path, "config", "c", "/etc/V2bX/sing_origin.json", "sing-box OriginalPath JSON file")
	c.Flags().BoolVar(&write, "write", false, "write the migration after validation and backup")
	return c
}
func init() { command.AddCommand(newMigrateCommand()) }
