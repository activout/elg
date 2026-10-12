package main

import (
	"context"
	"github.com/activout/elg/internal/cli"
	"os"
	"os/signal"
	"runtime/debug"
)

var version = "dev"
var commit = "unknown"
var sdkVersion = ""

func main() {
	if info, ok := debug.ReadBuildInfo(); ok {
		if sdkVersion == "" {
			for _, module := range info.Deps {
				if module.Path == "github.com/glesys/glesys-go/v8" {
					sdkVersion = module.Version
					if module.Replace != nil {
						sdkVersion += "+fork." + module.Replace.Version
					}
				}
			}
		}
		if commit == "unknown" {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					commit = setting.Value
				}
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.BuildInfo{Version: version, Commit: commit, SDKVersion: sdkVersion}))
}
