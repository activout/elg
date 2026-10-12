package cli

import (
	"sort"
	"strconv"
	"strings"

	glesys "github.com/glesys/glesys-go/v8"
	"github.com/spf13/cobra"
)

func (a *app) dnsCommands() *cobra.Command {
	root := &cobra.Command{Use: "dns", Short: "Manage DNS hosting, independently of domain registration"}
	domain := &cobra.Command{Use: "domain", Short: "Manage hosted zones"}
	record := &cobra.Command{Use: "record", Short: "Manage DNS records"}
	root.AddCommand(domain, record)
	domain.AddCommand(command("list", "List hosted domains", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.DNSDomains.List(a.ctx)
		if err != nil {
			return err
		}
		rows := nonNil(*v)
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
		return a.result(rows)
	}))
	for _, verb := range []string{"get", "export", "delete"} {
		domain.AddCommand(command(verb+" DOMAIN", verb+" a hosted zone", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
			if err := validateDomain(args[0]); err != nil {
				return err
			}
			if err := a.connect(c); err != nil {
				return err
			}
			switch verb {
			case "get":
				v, err := a.client.DNSDomains.Details(a.ctx, args[0])
				if err != nil {
					return err
				}
				return a.result(v)
			case "export":
				v, err := a.client.DNSDomains.Export(a.ctx, args[0])
				if err != nil {
					return err
				}
				return a.result(map[string]any{"domainname": args[0], "zonefile": v})
			default:
				if a.dryRun {
					return a.plan("delete", args[0], map[string]any{"forcedeleteemail": "no"})
				}
				if err := a.confirm(c, args[0], "delete DNS hosting; email is never force-deleted"); err != nil {
					return err
				}
				if err := a.client.DNSDomains.Delete(a.ctx, glesys.DeleteDNSDomainParams{Name: args[0], ForceDeleteEmail: "no"}); err != nil {
					return mutationError(err)
				}
				return a.operation("delete", args[0])
			}
		}))
	}
	var createRecords bool
	add := command("add DOMAIN", "Add DNS hosting without registering a domain", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateDomain(args[0]); err != nil {
			return err
		}
		setting := "no"
		if createRecords {
			setting = "yes"
		}
		p := glesys.AddDNSDomainParams{Name: args[0], CreateRecords: setting}
		if err := a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan("add", args[0], p)
		}
		v, err := a.client.DNSDomains.AddDNSDomain(a.ctx, p)
		if err != nil {
			return mutationError(err)
		}
		return a.result(v)
	})
	add.Flags().BoolVar(&createRecords, "create-records", false, "Create provider default records (default: none)")
	domain.AddCommand(add)
	record.AddCommand(command("list DOMAIN", "List records ordered by ID", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateDomain(args[0]); err != nil {
			return err
		}
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.DNSDomains.ListRecords(a.ctx, args[0])
		if err != nil {
			return err
		}
		rows := nonNil(*v)
		sort.Slice(rows, func(i, j int) bool { return rows[i].RecordID < rows[j].RecordID })
		return a.result(rows)
	}))
	record.AddCommand(a.recordWrite("create"), a.recordWrite("update"))
	record.AddCommand(command("delete RECORD_ID", "Delete a record after confirmation", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		id, err := recordID(args[0])
		if err != nil {
			return err
		}
		if err = a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan("delete", args[0], nil)
		}
		if err = a.confirm(c, args[0], "delete this DNS record"); err != nil {
			return err
		}
		if err = a.client.DNSDomains.DeleteRecord(a.ctx, id); err != nil {
			return mutationError(err)
		}
		return a.operation("delete", args[0])
	}))
	return root
}
func recordID(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, invalid("record ID must be a positive integer")
	}
	return n, nil
}
func (a *app) recordWrite(verb string) *cobra.Command {
	var kind, host, data string
	var ttl int
	use := verb + " DOMAIN"
	if verb == "update" {
		use = verb + " RECORD_ID"
	}
	c := command(use, verb+" a DNS record using provider data syntax", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if c.Flags().Changed("ttl") && ttl < 1 {
			return invalid("ttl must be positive, in seconds")
		}
		if c.Flags().Changed("type") {
			kind = strings.ToUpper(kind)
			if !idPattern.MatchString(kind) {
				return invalid("invalid record type")
			}
		}
		if verb == "create" {
			if err := validateDomain(args[0]); err != nil {
				return err
			}
			if data == "" {
				return invalid("record data must not be empty")
			}
			p := glesys.AddRecordParams{DomainName: args[0], Type: kind, Host: host, Data: data, TTL: ttl}
			if err := a.connect(c); err != nil {
				return err
			}
			if a.dryRun {
				return a.plan(verb, args[0], p)
			}
			v, err := a.client.DNSDomains.AddRecord(a.ctx, p)
			if err != nil {
				return mutationError(err)
			}
			return a.result(v)
		}
		id, err := recordID(args[0])
		if err != nil {
			return err
		}
		p := glesys.DNSRecordPatch{RecordID: id}
		if c.Flags().Changed("type") {
			p.Type = &kind
		}
		if c.Flags().Changed("host") {
			p.Host = &host
		}
		if c.Flags().Changed("data") {
			p.Data = &data
		}
		if c.Flags().Changed("ttl") {
			p.TTL = &ttl
		}
		if p.Type == nil && p.Host == nil && p.Data == nil && p.TTL == nil {
			return invalid("supply at least one record setting")
		}
		if err = a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan(verb, args[0], p)
		}
		v, err := a.client.DNSDomains.UpdateRecordPatch(a.ctx, p)
		if err != nil {
			return mutationError(err)
		}
		return a.result(v)
	})
	c.Flags().StringVar(&kind, "type", "", "Record type, e.g. A, AAAA, MX, TXT or SRV")
	c.Flags().StringVar(&host, "host", "", "Host/name within the zone (empty for apex)")
	c.Flags().StringVar(&data, "data", "", "Provider record data; include priority for MX and SRV components")
	c.Flags().IntVar(&ttl, "ttl", 0, "TTL in seconds; omitted uses the provider default")
	if verb == "create" {
		_ = c.MarkFlagRequired("type")
		_ = c.MarkFlagRequired("host")
		_ = c.MarkFlagRequired("data")
	}
	return c
}
