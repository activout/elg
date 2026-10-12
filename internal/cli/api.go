package cli

import (
	"encoding/json"
	glesys "github.com/glesys/glesys-go/v8"
	"github.com/spf13/cobra"
	"strings"
)

func (a *app) apiCommands() *cobra.Command {
	root := &cobra.Command{Use: "api", Short: "Call API operations through the SDK transport"}
	var method, file string
	c := command("call ENDPOINT", "Return a complete provider envelope, subject to secret redaction", cobra.ExactArgs(1), func(c *cobra.Command, args []string) error {
		if err := glesys.ValidateEndpoint(args[0]); err != nil {
			return invalid(err.Error())
		}
		method = strings.ToUpper(method)
		if method != "GET" && method != "POST" {
			return invalid("method must be GET or POST")
		}
		var params map[string]any
		if file != "" {
			if err := decodeFile(file, a.in, &params); err != nil {
				return err
			}
			if params == nil {
				return invalid("data-file must contain a JSON object")
			}
		}
		if method == "GET" {
			for k, v := range params {
				if !idPattern.MatchString(k) {
					return invalid("invalid GET parameter name")
				}
				switch x := v.(type) {
				case string:
					if x == "." || x == ".." {
						return invalid("invalid GET parameter value")
					}
				case json.Number, bool:
				default:
					return invalid("GET parameters must be strings, numbers or booleans")
				}
			}
		}
		if err := a.connect(c); err != nil {
			return err
		}
		a.meta["endpoint"] = args[0]
		a.meta["method"] = method
		readOnly := reads[args[0]]
		if a.dryRun && !readOnly {
			return a.plan("api call", args[0], params)
		}
		if !readOnly && !a.yes {
			return &failure{"confirmation_required", "unknown or mutating API operations require --yes, including GET", 8, 0}
		}
		if (args[0] == "email/createaccount" || args[0] == "email/resetpassword") && !a.showSecrets {
			return invalid("this API operation can generate a password; use the typed email command with --secret-output-file, or deliberately pass --show-secrets")
		}
		v, err := a.client.Request(a.ctx, method, args[0], params)
		if err != nil {
			if !readOnly {
				return mutationError(err)
			}
			return err
		}
		return a.result(v)
	})
	c.Flags().StringVar(&method, "method", "POST", "GET or POST (HTTP method alone does not establish safety)")
	c.Flags().StringVar(&file, "data-file", "", "JSON object file, or - for stdin")
	root.AddCommand(c)
	return root
}
