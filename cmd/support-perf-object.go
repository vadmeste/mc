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
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	humanize "github.com/dustin/go-humanize"
	"github.com/minio/cli"
	json "github.com/minio/colorjson"
	"github.com/minio/madmin-go"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/pkg/console"
)

// Deprecated June 2022
var adminSpeedtestCmd = cli.Command{
	Name:               "speedtest",
	Usage:              "Run server side speed test",
	Action:             mainAdminSpeedtest,
	OnUsageError:       onUsageError,
	Before:             setGlobalsFromContext,
	HideHelpCommand:    true,
	Hidden:             true,
	CustomHelpTemplate: "Please use 'mc support perf'",
}

// Deprecated June 2022
func mainAdminSpeedtest(ctx *cli.Context) error {
	console.Infoln("Please use 'mc support perf'")
	return nil
}

const (
	perfObjectDurationDefault   = 10 * time.Second
	perfObjectVerboseDefault    = false
	perfObjectSizeDefault       = 64 * 1024 * 1024
	perfObjectConcurrentDefault = 32
	perfObjectBucketDefault     = ""
)

var supportPerfObjectFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "duration",
		Usage: "duration for each perf test is run",
		Value: fmt.Sprintf("%ds", perfObjectDurationDefault/time.Second),
	},
	cli.BoolFlag{
		Name:  "verbose, v",
		Usage: "display per-server stats",
	},
	cli.StringFlag{
		Name:  "size",
		Usage: "size of the object used for uploads/downloads",
		Value: humanize.IBytes(perfObjectSizeDefault),
	},
	cli.IntFlag{
		Name:  "concurrent",
		Usage: "number of concurrent requests per server",
		Value: perfObjectConcurrentDefault,
	},
	cli.StringFlag{
		Name:   "bucket",
		Usage:  "provide a custom bucket name to use (NOTE: bucket must be created prior)",
		Value:  perfObjectBucketDefault,
		Hidden: true, // Hidden for now.
	},
}

var supportPerfObjectCmd = cli.Command{
	Name:            "object",
	Usage:           "analyze object performance",
	Action:          mainSupportPerfObject,
	OnUsageError:    onUsageError,
	Before:          setGlobalsFromContext,
	Flags:           append(supportPerfObjectFlags, globalFlags...),
	HideHelpCommand: true,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [COMMAND] [FLAGS] TARGET

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}

EXAMPLES:
  1. Run object speed measurement with autotuning the concurrency to obtain maximum throughput and IOPs:
     {{.Prompt}} {{.HelpName}} object myminio/
  2. Run object speed measurement for 20 seconds with object size of 128MiB with autotuning the concurrency to obtain maximum throughput:
     {{.Prompt}} {{.HelpName}} object myminio/ --duration 20s --size 128MiB
`,
}

// Object speed test

func (s speedTestResult) StringVerbose() (msg string) {
	result := s.result
	if s.verbose {
		msg += "\n\n"
		msg += "PUT:\n"
		for _, node := range result.PUTStats.Servers {
			msg += fmt.Sprintf("   * %s: %s/s %s objs/s", node.Endpoint, humanize.IBytes(node.ThroughputPerSec), humanize.Comma(int64(node.ObjectsPerSec)))
			if node.Err != "" {
				msg += " Err: " + node.Err
			}
			msg += "\n"
		}

		msg += "GET:\n"
		for _, node := range result.GETStats.Servers {
			msg += fmt.Sprintf("   * %s: %s/s %s objs/s", node.Endpoint, humanize.IBytes(node.ThroughputPerSec), humanize.Comma(int64(node.ObjectsPerSec)))
			if node.Err != "" {
				msg += " Err: " + node.Err
			}
			msg += "\n"
		}

	}
	return msg
}

func (s speedTestResult) String() (msg string) {
	result := s.result
	msg += fmt.Sprintf("MinIO %s, %d servers, %d drives, %s objects, %d threads",
		result.Version, result.Servers, result.Disks,
		humanize.IBytes(uint64(result.Size)), result.Concurrent)

	return msg
}

func (s speedTestResult) JSON() string {
	JSONBytes, e := json.MarshalIndent(s.result, "", "    ")
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")
	return string(JSONBytes)
}

func doPerfObject(ctx context.Context, aliasedURL string, duration time.Duration, size uint64, concurrent int, bucket string, autotune, verbose bool) error {
	client, perr := newAdminClient(aliasedURL)
	if perr != nil {
		fatalIf(perr.Trace(aliasedURL), "Unable to initialize admin client.")
		return nil
	}

	ctxt, cancel := context.WithCancel(globalContext)
	defer cancel()

	resultCh, err := client.Speedtest(ctxt, madmin.SpeedtestOpts{
		Size:        int(size),
		Duration:    duration,
		Concurrency: concurrent,
		Autotune:    autotune,
		Bucket:      bucket, // This is a hidden flag.
	})
	fatalIf(probe.NewError(err), "Failed to execute performance test")

	if globalJSON {
		for result := range resultCh {
			if result.Version == "" {
				continue
			}
			printMsg(speedTestResult{
				result: &result,
			})
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
		var result madmin.SpeedTestResult
		for result = range resultCh {
			p.Send(speedTestResult{
				result:  &result,
				verbose: verbose,
			})
		}
		p.Send(speedTestResult{
			result:  &result,
			verbose: verbose,
			final:   true,
		})
	}()

	<-done
	return nil
}

func mainSupportPerfObject(ctx *cli.Context) error {
	if len(ctx.Args()) != 1 {
		cli.ShowCommandHelpAndExit(ctx, "object", 1) // last argument is exit code
	}

	duration, e := time.ParseDuration(ctx.String("duration"))
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to parse duration")
		return nil
	}
	if duration <= 0 {
		fatalIf(errInvalidArgument(), "duration cannot be 0 or negative")
		return nil
	}
	size, e := humanize.ParseBytes(ctx.String("size"))
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to parse object size")
		return nil
	}
	if size < 0 {
		fatalIf(errInvalidArgument(), "size is expected to be atleast 0 bytes")
		return nil
	}
	concurrent := ctx.Int("concurrent")
	if concurrent <= 0 {
		fatalIf(errInvalidArgument(), "concurrency cannot be '0' or negative")
		return nil
	}

	// Turn-off autotuning only when "concurrent" is specified
	// in all other scenarios keep auto-tuning on.
	autotune := !ctx.IsSet("concurrent")

	verbose := ctx.Bool("verbose")
	bucket := ctx.String("bucket")

	return doPerfObject(globalContext, ctx.Args().Get(0), duration, size, concurrent, bucket, autotune, verbose)
}
