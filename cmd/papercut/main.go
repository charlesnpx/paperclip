package main

import (
	"os"

	"github.com/charlesnpx/paperclip/internal/app"
	"github.com/charlesnpx/paperclip/internal/cli"
	repoctx "github.com/charlesnpx/paperclip/internal/context"
	"github.com/charlesnpx/paperclip/internal/ledger"
	"github.com/charlesnpx/paperclip/internal/policy"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "delegate-install" {
		os.Exit(cli.RunInstaller(os.Args[2:], os.Stdout, os.Stderr))
	}
	repo, err := ledger.NewDefault()
	if err != nil {
		_, _ = os.Stderr.WriteString("papercut: " + err.Error() + "\n")
		os.Exit(cli.ExitUnexpected)
	}
	application := app.New(repo, repoctx.NewResolver(""), policy.DefaultScanner())
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, application))
}
