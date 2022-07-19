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

	tea "github.com/charmbracelet/bubbletea"
	humanize "github.com/dustin/go-humanize"
	"github.com/minio/cli"
	json "github.com/minio/colorjson"
	"github.com/minio/madmin-go"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/pkg/console"
)

const (
	perfDriveFileSizeDefault  = 1024 * 1024 * 1024
	perfDriveBlockSizeDefault = 4 * 1024 * 1024
	perfDriveSerialDefault    = false
)

var supportPerfDriveFlags = []cli.Flag{
	// Drive test specific flags.
	cli.StringFlag{
		Name:  "filesize",
		Usage: "total amount of data read/written to each drive",
		Value: humanize.IBytes(perfDriveFileSizeDefault),
	},
	cli.StringFlag{
		Name:  "blocksize",
		Usage: "read/write block size",
		Value: humanize.IBytes(perfDriveBlockSizeDefault),
	},
	cli.BoolFlag{
		Name:  "serial",
		Usage: "run tests on drive(s) one-by-one",
	},
}

var supportPerfDriveCmd = cli.Command{
	Name:            "drive",
	Usage:           "analyze drive performance",
	Action:          mainSupportPerfDrive,
	OnUsageError:    onUsageError,
	Before:          setGlobalsFromContext,
	Flags:           append(supportPerfDriveFlags, globalFlags...),
	HideHelpCommand: true,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [COMMAND] [FLAGS] TARGET

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}

EXAMPLES:
  1. Run all speed measurement tests in 'myminio' cluster
     {{.Prompt}} {{.HelpName}} myminio/
`,
}

func doPerfDrive(ctx context.Context, aliasedURL string, blocksize, filesize uint64, serial bool) error {
	client, perr := newAdminClient(aliasedURL)
	if perr != nil {
		fatalIf(perr.Trace(aliasedURL), "Unable to initialize admin client.")
		return nil
	}

	ctxt, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh, e := client.DriveSpeedtest(ctxt, madmin.DriveSpeedTestOpts{
		Serial:    serial,
		BlockSize: uint64(blocksize),
		FileSize:  uint64(filesize),
	})
	fatalIf(probe.NewError(e), "Failed to execute drive speedtest")

	if globalJSON {
		for result := range resultCh {
			if result.Version != "" {
				jsonBytes, e := json.MarshalIndent(result, "", " ")
				fatalIf(probe.NewError(e), "Unable to marshal into JSON.")

				console.Println(string(jsonBytes))
			}
		}
		return nil
	}

	done := make(chan struct{})

	p := tea.NewProgram(initSpeedTestUI())
	go func() {
		if e := p.Start(); e != nil {
			os.Exit(1)
		}
		close(done)
	}()

	go func() {
		var results []madmin.DriveSpeedTestResult
		for result := range resultCh {
			if result.Version != "" {
				results = append(results, result)
			} else {
				p.Send(speedTestResult{
					dresult: []madmin.DriveSpeedTestResult{},
				})
			}
		}
		p.Send(speedTestResult{
			dresult: results,
			final:   true,
		})
	}()

	<-done

	return nil
}

func mainSupportPerfDrive(ctx *cli.Context) error {
	if len(ctx.Args()) != 1 {
		cli.ShowCommandHelpAndExit(ctx, "drive", 1) // last argument is exit code
	}

	blocksize, e := humanize.ParseBytes(ctx.String("blocksize"))
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to parse blocksize")
		return nil
	}
	if blocksize < 0 {
		fatalIf(errInvalidArgument(), "blocksize cannot be <= 0")
		return nil
	}

	filesize, e := humanize.ParseBytes(ctx.String("filesize"))
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to parse filesize")
		return nil
	}
	if filesize < 0 {
		fatalIf(errInvalidArgument(), "filesize cannot be <= 0")
		return nil
	}

	serial := ctx.Bool("serial")

	return doPerfDrive(globalContext, ctx.Args().Get(0), blocksize, filesize, serial)
}
