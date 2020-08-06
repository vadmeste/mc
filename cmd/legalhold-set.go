/*
 * MinIO Client (C) 2020 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/minio/cli"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio/pkg/console"
)

var (
	lhSetFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "recursive, r",
			Usage: "apply legal hold recursively",
		},
		cli.StringFlag{
			Name:  "version-id",
			Usage: "apply legal hold to a specific object version",
		},
		cli.StringFlag{
			Name:  "rewind",
			Usage: "Move back in time",
		},
		cli.BoolFlag{
			Name:  "versions",
			Usage: "Pick earlier versions",
		},
	}
)
var legalHoldSetCmd = cli.Command{
	Name:   "set",
	Usage:  "set legal hold for object(s)",
	Action: mainLegalHoldSet,
	Before: setGlobalsFromContext,
	Flags:  append(lhSetFlags, globalFlags...),
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] TARGET

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}

EXAMPLES:
   1. Enable legal hold on a specific object
      $ {{.HelpName}} myminio/mybucket/prefix/obj.csv

   2. Enable legal hold on a specific object version
      $ {{.HelpName}} myminio/mybucket/prefix/obj.csv --version-id "HiMFUTOowG6ylfNi4LKxD3ieHbgfgrvC"

   3. Enable object legal hold recursively for all objects at a prefix
      $ {{.HelpName}} myminio/mybucket/prefix --recursive

   4. Enable object legal hold recursively for all objects versions older than one year
      $ {{.HelpName}} myminio/mybucket/prefix --recursive --rewind 365d --versions

 `,
}

// setLegalHold - Set legalhold for all objects within a given prefix.
func setLegalHold(urlStr, versionID string, timeRef time.Time, withOlderVersions, recursive bool, lhold minio.LegalHoldStatus) error {
	ctx, cancelLegalHold := context.WithCancel(globalContext)
	defer cancelLegalHold()

	clnt, err := newClient(urlStr)
	if err != nil {
		fatalIf(err.Trace(), "Cannot parse the provided url.")
	}
	if !recursive {
		err = clnt.PutObjectLegalHold(ctx, versionID, lhold)
		if err != nil {
			errorIf(err.Trace(urlStr), "Failed to set legal hold on `"+urlStr+"` successfully")
		} else {
			printMsg(legalHoldCmdMessage{
				LegalHold: lhold,
				Status:    "success",
				URLPath:   urlStr,
				VersionID: versionID,
			})
		}
		return nil
	}

	alias, _, _ := mustExpandAlias(urlStr)
	var cErr error
	errorsFound := false
	lstOptions := ListOptions{isRecursive: recursive, showDir: DirNone}
	if !timeRef.IsZero() {
		lstOptions.withOlderVersions = withOlderVersions
		lstOptions.withDeleteMarkers = true
		lstOptions.timeRef = timeRef
	}
	for content := range clnt.List(ctx, lstOptions) {
		if content.Err != nil {
			errorIf(content.Err.Trace(clnt.GetURL().String()), "Unable to list folder.")
			cErr = exitStatus(globalErrorExitStatus) // Set the exit status.
			continue
		}

		newClnt, perr := newClientFromAlias(alias, content.URL.String())
		if perr != nil {
			errorIf(content.Err.Trace(clnt.GetURL().String()), "Invalid URL")
			continue
		}
		probeErr := newClnt.PutObjectLegalHold(ctx, content.VersionID, lhold)
		if probeErr != nil {
			errorsFound = true
			errorIf(probeErr.Trace(content.URL.Path), "Failed to set legal hold on `"+content.URL.Path+"` successfully")
		} else {
			if globalJSON {
				printMsg(legalHoldCmdMessage{
					LegalHold: lhold,
					Status:    "success",
					URLPath:   content.URL.Path,
					VersionID: content.VersionID,
				})
			}
		}
	}
	if cErr == nil && !globalJSON {
		if errorsFound {
			console.Print(console.Colorize("LegalHoldPartialFailure", fmt.Sprintf("Errors found while setting legal hold status on objects with prefix `%s`. \n", urlStr)))
		} else {
			console.Print(console.Colorize("LegalHoldSuccess", fmt.Sprintf("Object legal hold successfully set for prefix `%s`.\n", urlStr)))
		}
	}
	return cErr
}

// main for retention command.
func mainLegalHoldSet(ctx *cli.Context) error {
	console.SetColor("LegalHoldSuccess", color.New(color.FgGreen, color.Bold))
	console.SetColor("LegalHoldPartialFailure", color.New(color.FgRed, color.Bold))
	console.SetColor("LegalHoldMessageFailure", color.New(color.FgYellow))

	args := ctx.Args()
	if len(args) != 1 {
		cli.ShowCommandHelpAndExit(ctx, "set", 1)
	}

	versionID := ctx.String("version-id")
	rewind := parseRewindFlag(ctx.String("rewind"))
	recursive := ctx.Bool("recursive")
	withVersions := ctx.Bool("versions")

	if versionID != "" && recursive {
		fatalIf(errInvalidArgument(), "Cannot pass --version-id and --recursive at the same time")
	}

	if rewind.IsZero() && withVersions {
		rewind = time.Now().UTC()
	}

	urlStr := args[0]
	return setLegalHold(urlStr, versionID, rewind, withVersions, recursive, minio.LegalHoldEnabled)
}
