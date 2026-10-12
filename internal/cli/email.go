package cli

import (
	"fmt"
	"sort"
	"strings"

	glesys "github.com/glesys/glesys-go/v8"
	"github.com/spf13/cobra"
)

func (a *app) emailCommands() *cobra.Command {
	root := &cobra.Command{Use: "email", Short: "Administer email hosting, accounts, aliases and quotas"}
	var filter string
	var page int
	overview := command("overview", "List all email domains and project limits", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if c.Flags().Changed("page") && page < 1 {
			return invalid("page must be positive")
		}
		if err := a.connect(c); err != nil {
			return err
		}
		return a.overview(filter, page, c.Flags().Changed("page"))
	})
	overview.Flags().StringVar(&filter, "filter", "", "API domain filter retained on every page")
	overview.Flags().IntVar(&page, "page", 0, "Fetch only this page instead of all pages")
	root.AddCommand(overview)
	root.AddCommand(a.emailList("list DOMAIN", "all"))
	root.AddCommand(command("costs", "Show prices with currency and units", cobra.NoArgs, func(c *cobra.Command, args []string) error {
		if err := a.connect(c); err != nil {
			return err
		}
		v, err := a.client.EmailDomains.CostsDetails(a.ctx)
		if err != nil {
			return err
		}
		return a.result(v)
	}))
	for _, kind := range []string{"account", "alias"} {
		group := &cobra.Command{Use: kind, Short: "Manage email " + kind + "s"}
		root.AddCommand(group)
		group.AddCommand(a.emailList("list DOMAIN", kind))
		group.AddCommand(command("get ADDRESS", "Get one exact "+kind, cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
			if _, err := addressDomain(args[0]); err != nil {
				return err
			}
			if err := a.connect(c); err != nil {
				return err
			}
			v, err := a.exactEmail(kind, args[0])
			if err != nil {
				return err
			}
			return a.result(v)
		}))
		group.AddCommand(command("delete ADDRESS", "Delete an exact "+kind+" after confirmation", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
			if _, err := addressDomain(args[0]); err != nil {
				return err
			}
			if err := a.connect(c); err != nil {
				return err
			}
			if _, err := a.exactEmail(kind, args[0]); err != nil {
				return err
			}
			if a.dryRun {
				return a.plan("delete", args[0], nil)
			}
			if err := a.confirm(c, args[0], "permanently delete this email "+kind); err != nil {
				return err
			}
			if err := a.client.EmailDomains.Delete(a.ctx, args[0]); err != nil {
				return mutationError(err)
			}
			return a.operation("delete", args[0])
		}))
		if kind == "alias" {
			group.AddCommand(a.aliasWrite("create"), a.aliasWrite("update"))
		} else {
			group.AddCommand(a.accountWrite("create"), a.accountWrite("update"), a.resetPassword())
			group.AddCommand(command("quota ADDRESS", "Show used MiB and allocated GiB", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
				if _, err := addressDomain(args[0]); err != nil {
					return err
				}
				if err := a.connect(c); err != nil {
					return err
				}
				v, err := a.client.EmailDomains.Quota(a.ctx, args[0])
				if err != nil {
					return err
				}
				return a.result(v)
			}))
		}
	}
	return root
}
func (a *app) emailList(use, kind string) *cobra.Command {
	var filter string
	c := command(use, "List email "+kind+" resources", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := validateDomain(args[0]); err != nil {
			return err
		}
		if err := a.connect(c); err != nil {
			return err
		}
		list, err := a.client.EmailDomains.List(a.ctx, args[0], glesys.ListEmailsParams{Filter: filter})
		if err != nil {
			return err
		}
		sort.Slice(list.EmailAccounts, func(i, j int) bool { return list.EmailAccounts[i].EmailAccount < list.EmailAccounts[j].EmailAccount })
		sort.Slice(list.EmailAliases, func(i, j int) bool { return list.EmailAliases[i].EmailAlias < list.EmailAliases[j].EmailAlias })
		list.EmailAccounts = nonNil(list.EmailAccounts)
		list.EmailAliases = nonNil(list.EmailAliases)
		switch kind {
		case "account":
			return a.result(list.EmailAccounts)
		case "alias":
			return a.result(list.EmailAliases)
		default:
			return a.result(list)
		}
	})
	c.Flags().StringVar(&filter, "filter", "", "Optional provider filter")
	return c
}
func equalAddress(x, y string) bool {
	i, j := strings.LastIndexByte(x, '@'), strings.LastIndexByte(y, '@')
	return i >= 0 && j >= 0 && x[:i] == y[:j] && strings.EqualFold(x[i+1:], y[j+1:])
}
func (a *app) exactEmail(kind, address string) (any, error) {
	domain, err := addressDomain(address)
	if err != nil {
		return nil, err
	}
	list, err := a.client.EmailDomains.List(a.ctx, domain, glesys.ListEmailsParams{Filter: address})
	if err != nil {
		return nil, err
	}
	if kind == "account" {
		for _, v := range list.EmailAccounts {
			if equalAddress(v.EmailAccount, address) {
				return v, nil
			}
		}
	} else {
		for _, v := range list.EmailAliases {
			if equalAddress(v.EmailAlias, address) {
				return v, nil
			}
		}
	}
	return nil, notFound("Email " + kind + " was not found")
}

type accountFlags struct {
	quota, spam                      int
	virus, reject, respond           bool
	message, messageFile, secretFile string
}

func (f *accountFlags) add(c *cobra.Command, create bool) {
	fs := c.Flags()
	fs.IntVar(&f.quota, "quota-gib", 0, "Allocated quota in GiB")
	fs.IntVar(&f.spam, "antispam-level", 0, "Spam filtering level (0–5)")
	fs.BoolVar(&f.virus, "antivirus", false, "Enable antivirus; explicit false is transmitted")
	fs.BoolVar(&f.reject, "reject-spam", false, "Reject spam")
	fs.BoolVar(&f.respond, "autorespond", false, "Enable autoresponse")
	fs.StringVar(&f.message, "autorespond-message", "", "Autoresponse text; empty clears it")
	fs.StringVar(&f.messageFile, "autorespond-message-file", "", "Read autoresponse text from a UTF-8 file")
	c.MarkFlagsMutuallyExclusive("autorespond-message", "autorespond-message-file")
	if create {
		fs.StringVar(&f.secretFile, "secret-output-file", "", "New file for a returned password (no overwrite)")
	}
}
func (f *accountFlags) patch(c *cobra.Command) (glesys.EmailAccountPatch, error) {
	p := glesys.EmailAccountPatch{}
	if c.Flags().Changed("quota-gib") {
		if f.quota < 0 {
			return p, invalid("quota-gib must not be negative")
		}
		p.QuotaInGiB = &f.quota
	}
	if c.Flags().Changed("antispam-level") {
		if f.spam < 0 || f.spam > 5 {
			return p, invalid("antispam-level must be between 0 and 5")
		}
		p.AntiSpamLevel = &f.spam
	}
	for _, flag := range []struct {
		name  string
		value bool
		field **string
	}{{"antivirus", f.virus, &p.AntiVirus}, {"reject-spam", f.reject, &p.RejectSpam}, {"autorespond", f.respond, &p.AutoRespond}} {
		if c.Flags().Changed(flag.name) {
			s := "no"
			if flag.value {
				s = "yes"
			}
			*flag.field = &s
		}
	}
	if c.Flags().Changed("autorespond-message") {
		p.AutoRespondMessage = &f.message
	}
	if c.Flags().Changed("autorespond-message-file") {
		message, err := textFile(f.messageFile)
		if err != nil {
			return p, invalid(err.Error())
		}
		f.message = message
		p.AutoRespondMessage = &f.message
	}
	return p, nil
}
func (a *app) accountWrite(verb string) *cobra.Command {
	f := &accountFlags{}
	c := command(verb+" ADDRESS", verb+" an account with only supplied settings", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if _, err := addressDomain(args[0]); err != nil {
			return err
		}
		p, err := f.patch(c)
		if err != nil {
			return err
		}
		if verb == "update" && p == (glesys.EmailAccountPatch{}) {
			return invalid("supply at least one account setting")
		}
		if err = a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan(verb, args[0], p)
		}
		if verb == "update" {
			v, err := a.client.EmailDomains.EditAccountPatch(a.ctx, args[0], p)
			if err != nil {
				return mutationError(err)
			}
			return a.result(v)
		}
		d, err := a.secretDestination(c, f.secretFile)
		if err != nil {
			return err
		}
		if d != nil {
			defer d.Close()
		}
		v, err := a.client.EmailDomains.CreateAccountWithSettings(a.ctx, args[0], p)
		if err != nil {
			return mutationError(err)
		}
		if err = deliver(d, v.Password); err != nil {
			return err
		}
		return a.result(v)
	})
	f.add(c, verb == "create")
	return c
}
func (a *app) aliasWrite(verb string) *cobra.Command {
	var to string
	c := command(verb+" ADDRESS", verb+" an alias forwarding destination", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if _, err := addressDomain(args[0]); err != nil {
			return err
		}
		// shortcut: only a single documented destination, add multi-destination syntax after provider verification.
		if _, err := addressDomain(to); err != nil {
			return invalid("to must be one destination email address")
		}
		if strings.ContainsAny(to, ",;") {
			return invalid("multiple destinations are not verified; supply one address")
		}
		p := glesys.EmailAliasParams{EmailAlias: args[0], GoTo: to}
		if err := a.connect(c); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan(verb, args[0], p)
		}
		var v *glesys.EmailAlias
		var err error
		if verb == "create" {
			v, err = a.client.EmailDomains.CreateAlias(a.ctx, p)
		} else {
			v, err = a.client.EmailDomains.EditAlias(a.ctx, p)
		}
		if err != nil {
			return mutationError(err)
		}
		return a.result(v)
	})
	c.Flags().StringVar(&to, "to", "", "One forwarding email address")
	_ = c.MarkFlagRequired("to")
	return c
}
func (a *app) resetPassword() *cobra.Command {
	var secretFile string
	c := command("reset-password ADDRESS", "Generate a new password after confirmation", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if _, err := addressDomain(args[0]); err != nil {
			return err
		}
		if err := a.connect(c); err != nil {
			return err
		}
		if _, err := a.exactEmail("account", args[0]); err != nil {
			return err
		}
		if a.dryRun {
			return a.plan("reset-password", args[0], nil)
		}
		if err := a.confirm(c, args[0], "invalidate the existing password"); err != nil {
			return err
		}
		d, err := a.secretDestination(c, secretFile)
		if err != nil {
			return err
		}
		if d != nil {
			defer d.Close()
		}
		secret, err := a.client.EmailDomains.ResetPassword(a.ctx, args[0])
		if err != nil {
			return mutationError(err)
		}
		if err = deliver(d, secret); err != nil {
			return err
		}
		return a.result(map[string]any{"emailaccount": args[0], "password": secret})
	})
	c.Flags().StringVar(&secretFile, "secret-output-file", "", "New file for the password (exact bytes; no newline added)")
	return c
}
func (a *app) overview(filter string, page int, explicit bool) error {
	if !explicit {
		page = 1
	}
	result := glesys.EmailOverview{Domains: []glesys.EmailOverviewDomain{}}
	seen := map[string]bool{}
	fetched := 0
	total := -1
	perPage := 0
	for {
		v, err := a.client.EmailDomains.Overview(a.ctx, glesys.OverviewParams{Filter: filter, Page: page})
		if err != nil {
			return err
		}
		m := v.Meta
		if m.Page != page || m.Total < 0 || m.PerPage < 1 || len(v.Domains) > m.PerPage {
			return fmt.Errorf("invalid overview pagination metadata")
		}
		if total == -1 {
			total = m.Total
			perPage = m.PerPage
			result.Summary = v.Summary
		} else if m.Total != total || m.PerPage != perPage {
			return fmt.Errorf("overview pagination changed during fetch; retry the read")
		}
		for _, d := range v.Domains {
			if seen[d.DomainName] {
				return fmt.Errorf("overview repeated a domain across pages")
			}
			seen[d.DomainName] = true
			result.Domains = append(result.Domains, d)
		}
		fetched++
		result.Meta = m
		complete := int64(m.Page)*int64(m.PerPage) >= int64(m.Total)
		if explicit || complete {
			a.meta["pages_fetched"] = fetched
			a.meta["complete"] = !explicit || m.Total <= m.PerPage
			break
		}
		if len(v.Domains) == 0 {
			return fmt.Errorf("overview returned an empty page before completion")
		}
		page++
	}
	if !explicit && len(result.Domains) != total {
		return fmt.Errorf("overview domain count disagrees with pagination total")
	}
	sort.Slice(result.Domains, func(i, j int) bool { return result.Domains[i].DomainName < result.Domains[j].DomainName })
	return a.result(result)
}
