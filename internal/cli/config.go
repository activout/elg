package cli

import (
	"github.com/activout/elg/internal/config"
	"github.com/spf13/cobra"
	"sort"
	"strings"
)

func (a *app) configCommands() *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Manage profiles; API keys are never stored"}
	root.AddCommand(command("show", "Show configuration without credential values", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.settings(); err != nil {
			return err
		}
		cfg, err := config.Load(a.configPath)
		if err != nil {
			return invalid(err.Error())
		}
		return a.write(cfg, map[string]any{"command": "config show"})
	}))
	profiles := &cobra.Command{Use: "profile", Short: "Manage named profiles"}
	root.AddCommand(profiles)
	var project, variable, base string
	set := command("set NAME", "Set a profile using an environment variable reference", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		p := config.Profile{Project: project, APIKeyEnv: variable, APIBaseURL: base}
		if err := config.Validate(p); err != nil {
			return invalid(err.Error())
		}
		if base != "" {
			if err := validateBaseURL(base); err != nil {
				return err
			}
		}
		if strings.TrimSpace(args[0]) == "" {
			return invalid("profile name is required")
		}
		return a.changeConfig(c, args[0], p, func(cfg *config.Config) error { cfg.Profiles[args[0]] = p; return nil })
	})
	// This local --project is the profile's value, not a connection override.
	set.Flags().StringVar(&project, "project", "", "Project ID to store")
	set.Flags().StringVar(&variable, "api-key-env", "", "Environment variable containing the API key")
	set.Flags().StringVar(&base, "api-base-url", "", "Explicit custom HTTPS API origin")
	_ = set.MarkFlagRequired("project")
	_ = set.MarkFlagRequired("api-key-env")
	profiles.AddCommand(set)
	profiles.AddCommand(command("list", "List profiles", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.settings(); err != nil {
			return err
		}
		cfg, err := config.Load(a.configPath)
		if err != nil {
			return invalid(err.Error())
		}
		type row struct {
			Name      string `json:"name"`
			Project   string `json:"project"`
			APIKeyEnv string `json:"api_key_env"`
			Default   bool   `json:"default"`
		}
		names := make([]string, 0, len(cfg.Profiles))
		for n := range cfg.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		rows := []row{}
		for _, n := range names {
			p := cfg.Profiles[n]
			rows = append(rows, row{n, p.Project, p.APIKeyEnv, n == cfg.DefaultProfile})
		}
		return a.write(rows, map[string]any{"command": "config profile list"})
	}))
	profiles.AddCommand(command("use NAME", "Select the default profile", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		return a.changeConfig(c, args[0], nil, func(cfg *config.Config) error {
			if _, ok := cfg.Profiles[args[0]]; !ok {
				return invalid("profile was not found")
			}
			cfg.DefaultProfile = args[0]
			return nil
		})
	}))
	profiles.AddCommand(command("remove NAME", "Remove a profile reference", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		return a.changeConfig(c, args[0], nil, func(cfg *config.Config) error {
			if _, ok := cfg.Profiles[args[0]]; !ok {
				return invalid("profile was not found")
			}
			delete(cfg.Profiles, args[0])
			if cfg.DefaultProfile == args[0] {
				cfg.DefaultProfile = ""
			}
			return nil
		})
	}))
	return root
}
func (a *app) changeConfig(c *cobra.Command, target string, values any, edit func(*config.Config) error) error {
	if err := a.settings(); err != nil {
		return err
	}
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return invalid(err.Error())
	}
	if err = edit(&cfg); err != nil {
		return err
	}
	a.meta = map[string]any{"command": strings.TrimPrefix(c.CommandPath(), "elg ")}
	if a.dryRun {
		return a.plan(c.Name(), target, values)
	}
	if err = config.Save(a.configPath, cfg); err != nil {
		return invalid("cannot save configuration: " + err.Error())
	}
	return a.operation(c.Name(), target)
}
