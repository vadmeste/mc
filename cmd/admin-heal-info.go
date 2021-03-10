/*
 * MinIO Client (C) 2021 MinIO, Inc.
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
	"fmt"
	"path/filepath"
	"strings"

	humanize "github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/minio/cli"
	json "github.com/minio/mc/pkg/colorjson"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/minio/pkg/console"
	"github.com/minio/minio/pkg/madmin"
)

var adminHealInfoCmd = cli.Command{
	Name:            "info",
	Usage:           "show healing information",
	Action:          mainAdminHealInfo,
	OnUsageError:    onUsageError,
	Before:          setGlobalsFromContext,
	Flags:           globalFlags,
	HideHelpCommand: true,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [FLAGS] TARGET

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}

`,
}

func checkAdminHealInfoSyntax(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		cli.ShowCommandHelpAndExit(ctx, "info", 1) // last argument is exit code
	}
}

// backgroundHealStatusMessage is container for stop heal success and failure messages.
type backgroundHealStatusMessage struct {
	Status   string `json:"status"`
	HealInfo madmin.BgHealState
}

func printSetHealInfo(set madmin.SetStatus) string {
	var out strings.Builder
	health := set.HealStatus
	if health == "" {
		health = "healthy"
	}

	fmt.Fprintf(&out, "Pool %v, Set %v: %s\n", set.PoolIndex+1, set.SetIndex+1, health)
	for _, disk := range set.Disks {
		diskState := disk.State
		if disk.Healing {
			diskState = "healing"
		}
		fmt.Fprintf(&out, "   %s (Status: %s, Capacity: %s/%s)\n", disk.Endpoint, diskState, humanize.IBytes(disk.UsedSpace), humanize.IBytes(disk.TotalSpace))
	}

	var healInfo *madmin.HealingDisk
	for _, disk := range set.Disks {
		if disk.HealInfo != nil {
			healInfo = disk.HealInfo
			break
		}
	}

	if healInfo == nil {
		return out.String()
	}

	fmt.Fprintf(&out, "       Start Time: %s\n", healInfo.Started)
	fmt.Fprintf(&out, "      Current Dir: %s/%s\n", healInfo.Bucket+"/"+healInfo.Object)
	fmt.Fprintf(&out, "   Objects healed: %d objects, %s data\n", healInfo.ObjectsHealed, humanize.IBytes((healInfo.BytesDone)))
	fmt.Fprintf(&out, "         Priority: %s\n", set.HealPriority)

	return out.String()
}

// String colorized to show background heal status message.
func (s backgroundHealStatusMessage) String() (output string) {
	for _, set := range s.HealInfo.Sets {
		output += printSetHealInfo(set)
	}
	return
}

// JSON jsonified stop heal message.
func (s backgroundHealStatusMessage) JSON() string {
	healJSONBytes, e := json.MarshalIndent(s, "", " ")
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")

	return string(healJSONBytes)
}

// mainAdminHealInfo - the entry function of heal info command
func mainAdminHealInfo(ctx *cli.Context) error {
	// Check for command syntax
	checkAdminHealInfoSyntax(ctx)

	// Get the alias parameter from cli
	args := ctx.Args()
	aliasedURL := args.Get(0)

	console.SetColor("Heal", color.New(color.FgGreen, color.Bold))
	console.SetColor("Dot", color.New(color.FgGreen, color.Bold))
	console.SetColor("HealBackgroundTitle", color.New(color.FgGreen, color.Bold))
	console.SetColor("HealBackground", color.New(color.Bold))

	// Create a new MinIO Admin Client
	client, err := newAdminClient(aliasedURL)
	if err != nil {
		fatalIf(err.Trace(aliasedURL), "Unable to initialize admin client.")
		return nil
	}

	// Compute bucket and object from the aliased URL
	aliasedURL = filepath.ToSlash(aliasedURL)
	clnt, err := newClient(aliasedURL)
	if err != nil {
		fatalIf(err.Trace(clnt.GetURL().String()), "Unable to create client for URL ", aliasedURL)
		return nil
	}

	bgHealStatus, berr := client.BackgroundHealStatus(globalContext)
	fatalIf(probe.NewError(berr), "Failed to get the status of the background heal.")
	printMsg(backgroundHealStatusMessage{Status: "success", HealInfo: bgHealStatus})
	return nil
}
