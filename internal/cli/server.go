package cli

import (
	"sort"
	"strings"

	glesys "github.com/glesys/glesys-go/v8"
	"github.com/spf13/cobra"
)

func (a *app) serverCommands() *cobra.Command {
	root := &cobra.Command{Use: "server", Short: "Manage server resources and lifecycle"}
	root.AddCommand(command("list", "List servers ordered by ID", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.Servers.List(a.ctx)
		if err != nil {
			return err
		}
		rows := nonNil(*v)
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
		return a.result(rows)
	}))
	root.AddCommand(command("templates", "List provider templates and minimum resource sizes", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.Servers.Templates(a.ctx)
		if err != nil {
			return err
		}
		v.KVM = nonNil(v.KVM)
		v.VMware = nonNil(v.VMware)
		sort.Slice(v.KVM, func(i, j int) bool { return v.KVM[i].ID < v.KVM[j].ID })
		sort.Slice(v.VMware, func(i, j int) bool { return v.VMware[i].ID < v.VMware[j].ID })
		return a.result(v)
	}))
	root.AddCommand(command("get ID", "Get a server including runtime state", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateID(args[0]); err != nil {
			return err
		}
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.Servers.Details(a.ctx, args[0])
		if err != nil {
			return err
		}
		v.IPList = nonNil(v.IPList)
		return a.result(v)
	}))
	for _, verb := range []string{"start", "stop", "delete"} {
		var stopType string
		var keepIP bool
		c := command(verb+" ID", verb+" a server", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
			if err := validateID(args[0]); err != nil {
				return err
			}
			if verb == "stop" && stopType != "soft" && stopType != "hard" {
				return invalid("stop type must be soft or hard")
			}
			if err := a.connect(c); err != nil {
				return err
			}
			values := map[string]any{"serverid": args[0]}
			if verb == "stop" {
				values["type"] = stopType
			}
			if verb == "delete" {
				values["keepip"] = keepIP
			}
			if a.dryRun {
				return a.plan(verb, args[0], values)
			}
			if verb != "start" {
				consequence := "stop the server"
				if verb == "delete" {
					consequence = "permanently destroy the server and all its files"
				}
				if err := a.confirm(c, args[0], consequence); err != nil {
					return err
				}
			}
			var err error
			switch verb {
			case "start":
				err = a.client.Servers.Start(a.ctx, args[0])
			case "stop":
				err = a.client.Servers.Stop(a.ctx, args[0], glesys.StopServerParams{Type: stopType})
			case "delete":
				err = a.client.Servers.Destroy(a.ctx, args[0], glesys.DestroyServerParams{KeepIP: keepIP})
			}
			if err != nil {
				return mutationError(err)
			}
			return a.operation(verb, args[0])
		})
		if verb == "stop" {
			c.Flags().StringVar(&stopType, "type", "soft", "Shutdown type: soft or hard")
		}
		if verb == "delete" {
			c.Flags().BoolVar(&keepIP, "keep-ip", false, "Retain IP allocations instead of releasing them")
		}
		root.AddCommand(c)
	}
	root.AddCommand(a.serverCreate(), a.serverUpdate())
	return root
}
func (a *app) serverCreate() *cobra.Command {
	p := glesys.CreateServerParams{}
	var cloudFile, keysFile, passwordPath, usersFile, cloudParamsFile, backupsFile, username string
	c := command("create", "Create a server with explicit sizing and provisioning", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if p.CPU < 1 || p.Memory < 1 || p.Storage < 1 || p.Bandwidth < 1 {
			return invalid("cpu-cores, memory-mib, disk-gib and bandwidth-mbps must be positive")
		}
		for _, s := range []string{p.Hostname, p.DataCenter, p.Platform, p.Template} {
			if strings.TrimSpace(s) == "" {
				return invalid("hostname, datacenter, platform and template must not be empty")
			}
		}
		if cloudFile != "" {
			v, err := textFile(cloudFile)
			if err != nil {
				return err
			}
			p.CloudConfig = v
		}
		if cloudParamsFile != "" {
			if err := decodeFile(cloudParamsFile, nil, &p.CloudConfigParams); err != nil {
				return err
			}
		}
		if keysFile != "" {
			v, err := textFile(keysFile)
			if err != nil {
				return err
			}
			if strings.Contains(v, "PRIVATE KEY") {
				return invalid("public-key-file must contain public keys, not a private key")
			}
			p.PublicKey = strings.TrimSpace(v)
			if p.PublicKey == "" {
				return invalid("public-key-file is empty")
			}
		}
		if passwordPath != "" {
			v, err := passwordFile(passwordPath)
			if err != nil {
				return err
			}
			p.Password = v
		}
		if usersFile != "" {
			if err := decodeFile(usersFile, nil, &p.Users); err != nil {
				return err
			}
		}
		if username != "" {
			if len(p.Users) > 0 {
				return invalid("username and users-file are mutually exclusive")
			}
			if p.PublicKey == "" && p.Password == "" {
				return invalid("username requires a public-key-file or root-password-file")
			}
			p.Users = []glesys.User{{Username: username, PublicKeys: []string{}, Password: p.Password}}
			if p.PublicKey != "" {
				p.Users[0].PublicKeys = strings.Split(p.PublicKey, "\n")
			}
			p.Password = ""
			p.PublicKey = ""
		}
		for _, u := range p.Users {
			if u.Username == "" || (len(u.PublicKeys) == 0 && u.Password == "") {
				return invalid("each provisioning user requires a username and password or public SSH key")
			}
			for _, k := range u.PublicKeys {
				if strings.Contains(k, "PRIVATE KEY") {
					return invalid("users-file contains a private key in sshkeys")
				}
			}
		}
		if strings.EqualFold(p.Platform, "KVM") && p.CloudConfig == "" && len(p.Users) == 0 {
			return invalid("KVM creation requires users-file, username with credentials, or cloud-config-file")
		}
		if backupsFile != "" {
			if err := decodeFile(backupsFile, nil, &p.Backup); err != nil {
				return err
			}
			if err := validateBackups(p.Backup); err != nil {
				return err
			}
		}
		a.requestSecrets = append(a.requestSecrets, p.Password)
		for _, u := range p.Users {
			a.requestSecrets = append(a.requestSecrets, u.Password)
		}
		if err := a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan("create", p.Hostname, p)
		}
		v, err := a.client.Servers.Create(a.ctx, p)
		if err != nil {
			return mutationError(err)
		}
		v.IPList = nonNil(v.IPList)
		return a.result(v)
	})
	f := c.Flags()
	f.StringVar(&p.Hostname, "hostname", "", "Server hostname")
	f.StringVar(&p.DataCenter, "datacenter", "", "Explicit provider datacenter")
	f.StringVar(&p.Platform, "platform", "", "Explicit platform, e.g. KVM")
	f.StringVar(&p.Template, "template", "", "Template ID (recommended), tag or name")
	f.IntVar(&p.CPU, "cpu-cores", 0, "CPU cores")
	f.IntVar(&p.Memory, "memory-mib", 0, "Memory in MiB")
	f.IntVar(&p.Storage, "disk-gib", 0, "System disk in GiB")
	f.IntVar(&p.Bandwidth, "bandwidth-mbps", 100, "Bandwidth in Mbps (visible in dry-run)")
	f.StringVar(&p.IPv4, "ipv4", "any", "IPv4 allocation: any or a provider-supported value")
	f.StringVar(&p.IPv6, "ipv6", "any", "IPv6 allocation: any or a provider-supported value")
	f.StringVar(&p.Description, "description", "", "Server description")
	f.StringVar(&cloudFile, "cloud-config-file", "", "UTF-8 cloud-config template file")
	f.StringVar(&cloudParamsFile, "cloud-config-params-file", "", "JSON template parameters")
	f.StringVar(&keysFile, "public-key-file", "", "Public SSH key file")
	f.StringVar(&passwordPath, "root-password-file", "", "Root/provisioning password file; strips one line ending")
	f.StringVar(&usersFile, "users-file", "", "JSON array of provisioning users")
	f.StringVar(&username, "username", "", "Provisioning user for supplied public key or password")
	f.StringVar(&backupsFile, "backup-schedules-file", "", "JSON backup schedules (frequency and numberofimagestokeep)")
	for _, name := range []string{"hostname", "datacenter", "platform", "template", "cpu-cores", "memory-mib", "disk-gib"} {
		_ = c.MarkFlagRequired(name)
	}
	c.MarkFlagsMutuallyExclusive("username", "users-file")
	return c
}
func validateBackups(s []glesys.ServerBackupSchedule) error {
	for _, b := range s {
		if (b.Frequency != "daily" && b.Frequency != "weekly") || b.Numberofimagestokeep < 1 {
			return invalid("backup schedules require daily/weekly frequency and positive numberofimagestokeep")
		}
	}
	return nil
}
func (a *app) serverUpdate() *cobra.Command {
	var cpu, memory, disk int
	var hostname, description, backupsFile string
	c := command("update ID", "Patch explicitly supplied server settings", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateID(args[0]); err != nil {
			return err
		}
		p := glesys.ServerPatch{}
		for _, f := range []struct {
			name  string
			v     *int
			field **int
		}{{"cpu-cores", &cpu, &p.CPU}, {"memory-mib", &memory, &p.Memory}, {"disk-gib", &disk, &p.Storage}} {
			if c.Flags().Changed(f.name) {
				if *f.v < 1 {
					return invalid(f.name + " must be positive")
				}
				*f.field = f.v
			}
		}
		if c.Flags().Changed("hostname") {
			if hostname == "" {
				return invalid("hostname must not be empty")
			}
			p.Hostname = &hostname
		}
		if c.Flags().Changed("description") {
			p.Description = &description
		}
		if c.Flags().Changed("backup-schedules-file") {
			var s []glesys.ServerBackupSchedule
			if err := decodeFile(backupsFile, nil, &s); err != nil {
				return err
			}
			if err := validateBackups(s); err != nil {
				return err
			}
			s = nonNil(s)
			p.Backup = &s
		}
		if p == (glesys.ServerPatch{}) {
			return invalid("supply at least one server setting")
		}
		if err := a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan("update", args[0], p)
		}
		v, err := a.client.Servers.EditPatch(a.ctx, args[0], p)
		if err != nil {
			return mutationError(err)
		}
		v.IPList = nonNil(v.IPList)
		return a.result(v)
	})
	f := c.Flags()
	f.IntVar(&cpu, "cpu-cores", 0, "CPU cores")
	f.IntVar(&memory, "memory-mib", 0, "Memory in MiB")
	f.IntVar(&disk, "disk-gib", 0, "System disk in GiB")
	f.StringVar(&hostname, "hostname", "", "Server hostname")
	f.StringVar(&description, "description", "", "Description; empty explicitly clears it")
	f.StringVar(&backupsFile, "backup-schedules-file", "", "JSON schedules; [] clears them")
	return c
}
