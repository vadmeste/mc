// Copyright (c) 2015-2021 MinIO, Inc.
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
	"time"

	"github.com/minio/cli"
	"github.com/minio/mc/pkg/probe"
)

var globalPerfTestVerbose bool

var supportPerfFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "duration",
		Usage: "duration for each perf test is run",
		Value: "10s",
	},
	cli.BoolFlag{
		Name:  "verbose, v",
		Usage: "display per-server stats",
	},
	cli.StringFlag{
		Name:   "size",
		Usage:  "size of the object used for uploads/downloads",
		Value:  "64MiB",
		Hidden: true,
	},
	cli.IntFlag{
		Name:   "concurrent",
		Usage:  "number of concurrent requests per server",
		Value:  32,
		Hidden: true,
	},
	cli.StringFlag{
		Name:   "bucket",
		Usage:  "provide a custom bucket name to use (NOTE: bucket must be created prior)",
		Hidden: true, // Hidden for now.
	},
	// Drive test specific flags.
	cli.StringFlag{
		Name:   "filesize",
		Usage:  "total amount of data read/written to each drive",
		Value:  "1GiB",
		Hidden: true,
	},
	cli.StringFlag{
		Name:   "blocksize",
		Usage:  "read/write block size",
		Value:  "4MiB",
		Hidden: true,
	},
	cli.BoolFlag{
		Name:   "serial",
		Usage:  "run tests on drive(s) one-by-one",
		Hidden: true,
	},
}

var supportPerfCmd = cli.Command{
	Name:            "perf",
	Usage:           "analyze object, network and drive performance",
	Action:          mainSupportPerf,
	OnUsageError:    onUsageError,
	Before:          setGlobalsFromContext,
	Flags:           append(supportPerfFlags, globalFlags...),
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

func checkSupportPerfSyntax(ctx *cli.Context) {
	duration, e := time.ParseDuration(ctx.String("duration"))
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to parse duration")
	}
	if duration <= 0 {
		fatalIf(errInvalidArgument(), "duration cannot be 0 or negative")
	}
	if len(ctx.Args()) == 0 || len(ctx.Args()) > 2 {
		cli.ShowCommandHelpAndExit(ctx, "perf", 1) // last argument is exit code
	}
}

func mainSupportPerf(ctx *cli.Context) error {
	checkSupportPerfSyntax(ctx)

	args := ctx.Args()
	switch len(args) {
	case 1:
		aliasedURL := args.Get(0)
		mainSpeedTestNetperf(ctx, aliasedURL)
		mainSpeedTestDrive(ctx, aliasedURL)
		return mainSpeedTestObject(ctx, aliasedURL)
	case 2:
		aliasedURL := args.Get(1)
		switch args[0] {
		case "drive":
			return mainSpeedTestDrive(ctx, aliasedURL)
		case "object":
			return mainSpeedTestObject(ctx, aliasedURL)
		case "net":
			return mainSpeedTestNetperf(ctx, aliasedURL)
		default:
			cli.ShowCommandHelpAndExit(ctx, "perf", 1)
		}
	}

	cli.ShowCommandHelpAndExit(ctx, "perf", 1)
	return nil
}
