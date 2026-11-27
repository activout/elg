package cli

import (
	"context"
	"sort"
	"strings"

	glesys "github.com/glesys/glesys-go/v8"
	"github.com/spf13/cobra"
)

func (a *app) volumeCommands() *cobra.Command {
	root := &cobra.Command{Use: "volume", Aliases: []string{"blockstorage"}, Short: "Manage Block Storage volumes"}
	root.AddCommand(command("list", "List volumes in the project ordered by ID", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.BlockStorageVolumes.List(a.ctx, a.project)
		if err != nil {
			return err
		}
		rows := nonNil(*v)
		sort.Slice(rows, func(i, j int) bool { return rows[i].VolumeID < rows[j].VolumeID })
		return a.result(rows)
	}))
	root.AddCommand(command("tiers", "List volume tiers with size limits and IOPS", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.BlockStorageVolumes.ListTiers(a.ctx)
		if err != nil {
			return err
		}
		return a.result(nonNil(*v))
	}))
	root.AddCommand(command("get ID", "Get a volume including attachments", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateID(args[0]); err != nil {
			return err
		}
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.BlockStorageVolumes.Details(a.ctx, args[0])
		if err != nil {
			return err
		}
		return a.result(v)
	}))
	root.AddCommand(a.volumeCost(), a.volumeCreate())

	var name string
	rename := command("rename ID", "Rename a volume", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateID(args[0]); err != nil {
			return err
		}
		if strings.TrimSpace(name) == "" {
			return invalid("name must not be empty")
		}
		p := glesys.EditBlockStorageParams{VolumeID: args[0], Name: name}
		return a.volumeEdit(c, "rename", p, (*glesys.BlockStorageService).UpdateName)
	})
	rename.Flags().StringVar(&name, "name", "", "New volume name")
	_ = rename.MarkFlagRequired("name")

	var size int
	resize := command("resize ID", "Grow an attached volume", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateID(args[0]); err != nil {
			return err
		}
		if size < 1 {
			return invalid("size-gib must be positive")
		}
		p := glesys.EditBlockStorageParams{VolumeID: args[0], SizeInGIB: size}
		return a.volumeEdit(c, "resize", p, (*glesys.BlockStorageService).Resize)
	})
	resize.Flags().IntVar(&size, "size-gib", 0, "New size in GiB; volumes can only grow")
	_ = resize.MarkFlagRequired("size-gib")

	var server string
	attach := command("attach ID", "Attach a volume to a server", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateID(args[0]); err != nil {
			return err
		}
		if err := validateID(server); err != nil {
			return err
		}
		if err := a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan("attach", args[0], map[string]any{"volumeid": args[0], "serverid": server})
		}
		v, err := a.client.BlockStorageVolumes.Attach(a.ctx, args[0], server)
		if err != nil {
			return mutationError(err)
		}
		return a.result(v)
	})
	attach.Flags().StringVar(&server, "server", "", "Server ID")
	_ = attach.MarkFlagRequired("server")
	root.AddCommand(rename, resize, attach)

	for _, verb := range []string{"detach", "delete"} {
		root.AddCommand(command(verb+" ID", verb+" a volume", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
			if err := validateID(args[0]); err != nil {
				return err
			}
			if err := a.connect(c); err != nil {
				return err
			}
			if a.dryRun {
				return a.plan(verb, args[0], map[string]any{"volumeid": args[0]})
			}
			consequence := "detach the volume from its server; unmount it first"
			if verb == "delete" {
				consequence = "permanently destroy the volume and its data"
			}
			if err := a.confirm(c, args[0], consequence); err != nil {
				return err
			}
			var err error
			if verb == "detach" {
				err = a.client.BlockStorageVolumes.Detach(a.ctx, args[0])
			} else {
				err = a.client.BlockStorageVolumes.Delete(a.ctx, args[0])
			}
			if err != nil {
				return mutationError(err)
			}
			return a.operation(verb, args[0])
		}))
	}
	return root
}

func (a *app) volumeEdit(c *cobra.Command, verb string, p glesys.EditBlockStorageParams, call func(*glesys.BlockStorageService, context.Context, glesys.EditBlockStorageParams) (*glesys.BlockStorageVolumeDetails, error)) error {
	if err := a.connect(c); err != nil {
		return err
	}
	if a.dryRun {
		return a.plan(verb, p.VolumeID, p)
	}
	v, err := call(a.client.BlockStorageVolumes, a.ctx, p)
	if err != nil {
		return mutationError(err)
	}
	return a.result(v)
}

func (a *app) volumeCreate() *cobra.Command {
	p := glesys.CreateBlockStorageParams{}
	c := command("create", "Create a volume", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		for _, s := range []string{p.Name, p.Datacenter, p.Tier} {
			if strings.TrimSpace(s) == "" {
				return invalid("name, datacenter and tier must not be empty")
			}
		}
		if p.SizeInGIB < 1 {
			return invalid("size-gib must be positive")
		}
		if err := a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan("create", p.Name, p)
		}
		v, err := a.client.BlockStorageVolumes.Create(a.ctx, p)
		if err != nil {
			return mutationError(err)
		}
		return a.result(v)
	})
	f := c.Flags()
	f.StringVar(&p.Name, "name", "", "Volume name")
	f.StringVar(&p.Datacenter, "datacenter", "", "Provider datacenter")
	f.StringVar(&p.Tier, "tier", "", "Tier slug from elg volume tiers, e.g. balanced")
	f.IntVar(&p.SizeInGIB, "size-gib", 0, "Size in GiB, within the tier limits")
	for _, n := range []string{"name", "datacenter", "tier", "size-gib"} {
		_ = c.MarkFlagRequired(n)
	}
	return c
}

func (a *app) volumeCost() *cobra.Command {
	var volume, tier string
	var size int
	c := command("cost", "Estimate the cost of a new or existing volume", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if volume != "" {
			if err := validateID(volume); err != nil {
				return err
			}
		}
		if volume == "" && (tier == "" || size < 1) {
			return invalid("supply --volume, or --tier and a positive --size-gib for a new volume")
		}
		if c.Flags().Changed("size-gib") && size < 1 {
			return invalid("size-gib must be positive")
		}
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.BlockStorageVolumes.EstimatedCost(a.ctx, glesys.EstimateCostBlockStorageParams{ProjectKey: a.project, VolumeID: volume, Tier: tier, SizeInGIB: size})
		if err != nil {
			return err
		}
		return a.result(v)
	})
	f := c.Flags()
	f.StringVar(&volume, "volume", "", "Existing volume ID")
	f.StringVar(&tier, "tier", "", "Tier slug")
	f.IntVar(&size, "size-gib", 0, "Size in GiB")
	return c
}
