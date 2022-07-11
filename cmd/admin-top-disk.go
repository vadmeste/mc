// Copyright (c) 2015-2022 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"context"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/minio/cli"
)

var adminTopDiskFlags = []cli.Flag{
	/*
		cli.BoolFlag{
			Name:  "errors, e",
			Usage: "summarize current API calls throwing only errors",
		},
	*/
}

var adminTopDiskCmd = cli.Command{
	Name:            "disk",
	Usage:           "summarize storage events on MinIO server in real-time",
	Action:          mainAdminTopDisk,
	OnUsageError:    onUsageError,
	Before:          setGlobalsFromContext,
	Flags:           append(adminTopDiskFlags, globalFlags...),
	HideHelpCommand: true,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] TARGET

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
   1. Display storage operations all S3 API calls.
      {{.Prompt}} {{.HelpName}} myminio/
`,
}

// checkAdminTopDiskSyntax - validate all the passed arguments
func checkAdminTopDiskSyntax(ctx *cli.Context) {
	if len(ctx.Args()) == 0 || len(ctx.Args()) > 1 {
		cli.ShowCommandHelpAndExit(ctx, "disk", 1) // last argument is exit code
	}
}

func mainAdminTopDisk(ctx *cli.Context) error {
	checkAdminTopAPISyntax(ctx)

	aliasedURL := ctx.Args().Get(0)

	// Create a new MinIO Admin Client
	client, err := newAdminClient(aliasedURL)
	if err != nil {
		fatalIf(err.Trace(aliasedURL), "Unable to initialize admin client.")
		return nil
	}

	ctxt, cancel := context.WithCancel(globalContext)
	defer cancel()

	// Start listening on all trace activity.
	// fatalIf(probe.NewError())

	done := make(chan struct{})

	p := tea.NewProgram(initTopDiskUI())
	go func() {
		if e := p.Start(); e != nil {
			os.Exit(1)
		}
		close(done)
	}()

	go func() {
		t := time.NewTimer(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				info, _ := client.ServerInfo(ctxt)
				for _, server := range info.Servers {
					for _, disk := range server.Disks {
						m := disk.Metrics
						if m == nil {
							continue
						}
						for k, v := range m.LastMinute {
							p.Send(topDiskResult{
								diskName:    disk.Endpoint,
								diskAPIName: k,
								diskAPICall: v.Count,
								diskAPIAvg:  uint64(v.Avg()),
							})
						}
					}
				}
				p.Send(topAPIResult{final: true})

			}
		}
	}()

	<-done
	return nil
}
