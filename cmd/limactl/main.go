package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AkihiroSuda/lima/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	if err := newApp().Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func newApp() *cli.App {
	app := cli.NewApp()
	app.Name = "limactl"
	app.Usage = `Lima: Linux-on-Mac ("macOS subsystem for Linux", "containerd for Mac")`
	app.UseShortOptionHandling = true
	app.EnableBashCompletion = true
	app.BashComplete = appBashComplete
	app.Version = strings.TrimPrefix(version.Version, "v")
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "debug mode",
		},
		&cli.BoolFlag{
			Name:  "macvirt",
			Usage: "Use Mac OS Virtualization framework instead qemu",
		},
	}
	app.Before = func(clicontext *cli.Context) error {
		if clicontext.Bool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		if clicontext.Bool("macvirt") {
			logrus.Debug("Use macvirt as virtualization")
		}
		if os.Geteuid() == 0 {
			return errors.New("must not run as the root")
		}
		return nil
	}
	app.Commands = []*cli.Command{
		startCommand,
		// TODO: add stopCommand (stops an instance without deletion)
		shellCommand,
		lsCommand,
		deleteCommand,
		completionCommand,
	}
	return app
}

const (
	DefaultInstanceName = "default"
)

func appBashComplete(clicontext *cli.Context) {
	w := clicontext.App.Writer
	cli.DefaultAppComplete(clicontext)
	for _, subcomm := range clicontext.App.Commands {
		fmt.Fprintln(w, subcomm.Name)
	}
}
