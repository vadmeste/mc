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

var supportPerfSubcommands = []cli.Command{
	supportPerfNetCmd,
	supportPerfDriveCmd,
	supportPerfObjectCmd,
}

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
}

var supportPerfCmd = cli.Command{
	Name:            "perf",
	Usage:           "analyze object, network and drive performance",
	Action:          mainSupportPerf,
	Subcommands:     supportPerfSubcommands,
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

func mainSupportPerf(ctx *cli.Context) error {
	if len(ctx.Args()) != 1 {
		cli.ShowCommandHelpAndExit(ctx, "perf", 1) // last argument is exit code
	}

	duration, e := time.ParseDuration(ctx.String("duration"))
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to parse duration")
	}
	if duration <= 0 {
		fatalIf(errInvalidArgument(), "duration cannot be 0 or negative")
	}

	verbose := ctx.Bool("verbose")

	args := ctx.Args()
	aliasedURL := args.Get(0)

	doPerfNet(globalContext, aliasedURL, duration)

	doPerfDrive(globalContext, aliasedURL, perfDriveBlockSizeDefault,
		perfDriveFileSizeDefault, perfDriveSerialDefault)

	doPerfObject(globalContext, aliasedURL, duration, perfObjectSizeDefault,
		perfObjectConcurrentDefault, perfObjectBucketDefault, false, verbose)

	return nil
}
