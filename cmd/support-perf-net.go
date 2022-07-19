// Copyright (c) 2022 MinIO, Inc.
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
	"github.com/minio/cli"
	json "github.com/minio/colorjson"
	"github.com/minio/madmin-go"
	"github.com/minio/mc/pkg/probe"
)

const (
	perfNetDurationDefault = 10 * time.Second
)

var supportPerfNetFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "duration",
		Usage: "duration for each perf test is run",
		Value: fmt.Sprintf("%ds", perfNetDurationDefault/time.Second),
	},
}

var supportPerfNetCmd = cli.Command{
	Name:            "net",
	Usage:           "analyze network performance",
	Action:          mainSupportPerfNet,
	OnUsageError:    onUsageError,
	Before:          setGlobalsFromContext,
	Flags:           append(supportPerfNetFlags, globalFlags...),
	HideHelpCommand: true,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [COMMAND] [FLAGS] TARGET

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}

EXAMPLES:
  1. Run drive speed measurements on all drive on all nodes (with default blockSize of 4MiB):
       {{.Prompt}} {{.HelpName}} drive myminio/
  2. Run drive speed measurements with blocksize of 64KiB, and 2GiB of data read/written from each drive:
       {{.Prompt}} {{.HelpName}} drive myminio/ --blocksize 64KiB --filesize 2GiB
`,
}

type netperfResult madmin.NetperfResult

func (m netperfResult) String() (msg string) {
	// string version is handled by banner.
	return ""
}

func (m netperfResult) JSON() string {
	JSONBytes, e := json.MarshalIndent(m, "", "    ")
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")
	return string(JSONBytes)
}

func doPerfNet(ctx context.Context, aliasedURL string, duration time.Duration) error {
	client, perr := newAdminClient(aliasedURL)
	if perr != nil {
		fatalIf(perr.Trace(aliasedURL), "Unable to initialize admin client.")
		return nil
	}

	ctxt, cancel := context.WithCancel(globalContext)
	defer cancel()

	resultCh := make(chan madmin.NetperfResult)
	go func() {
		result, err := client.Netperf(ctxt, duration)
		fatalIf(probe.NewError(err), "Unable to capture network perf results")

		resultCh <- result
		close(resultCh)
	}()

	if globalJSON {
		for {
			select {
			case result := <-resultCh:
				printMsg(netperfResult(result))
				return nil
			}
		}
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
		for {
			select {
			case result := <-resultCh:
				p.Send(speedTestResult{
					nresult: &result,
					final:   true,
				})
				return
			default:
				p.Send(speedTestResult{
					nresult: &madmin.NetperfResult{},
				})
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	<-done

	return nil
}

func mainSupportPerfNet(ctx *cli.Context) error {
	if len(ctx.Args()) != 1 {
		cli.ShowCommandHelpAndExit(ctx, "net", 1) // last argument is exit code
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

	return doPerfNet(globalContext, ctx.Args().Get(0), duration)
}
