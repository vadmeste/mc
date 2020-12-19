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
	"fmt"
	"net/url"
	"regexp"
	"strings"

	humanize "github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/minio/cli"
	json "github.com/minio/mc/pkg/colorjson"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/minio/pkg/auth"
	"github.com/minio/minio/pkg/console"
	"github.com/minio/minio/pkg/madmin"
)

var adminBucketRemoteAddFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "path",
		Value: "auto",
		Usage: "bucket path lookup supported by the server. Valid options are '[on,off,auto]'",
	},
	cli.StringFlag{
		Name:  "service",
		Usage: "type of service. Valid options are '[replication, ilm]'",
	},
	cli.StringFlag{
		Name:  "region",
		Usage: "region of the destination bucket (optional)",
	},
	cli.StringFlag{
		Name:  "label",
		Usage: "set a label to identify this target (optional)",
	},
	cli.StringFlag{
		Name:  "bandwidth",
		Usage: "Set bandwidth limit in bits per second (K,B,G,T for metric and Ki,Bi,Gi,Ti for IEC units)",
	},
}
var adminBucketRemoteAddCmd = cli.Command{
	Name:         "add",
	Usage:        "add a new remote target",
	Action:       mainAdminBucketRemoteAdd,
	OnUsageError: onUsageError,
	Before:       setGlobalsFromContext,
	Flags:        append(globalFlags, adminBucketRemoteAddFlags...),
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} TARGET http(s)://ACCESSKEY:SECRETKEY@DEST_URL/DEST_BUCKET [--path | --region | --label| --bandwidth] --service

TARGET:
  Also called as alias/sourcebucketname

DEST_BUCKET:
  Also called as remote target bucket.

DEST_URL:
  Also called as remote endpoint.

ACCESSKEY:
  Also called as username.

SECRETKEY:
  Also called as password.

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
  1. Set a new remote replication target "targetbucket" in region "us-west-1" on https://minio.siteb.example.com for bucket 'sourcebucket'.
     {{.Prompt}} {{.HelpName}} sitea/sourcebucket https://foobar:foo12345@minio.siteb.example.com/targetbucket \
         --service "replication" --region "us-west-1" --label "hdd-tier"

  2. Set a new remote replication target 'targetbucket' in region "us-west-1" on https://minio.siteb.example.com for
     bucket 'sourcebucket' with bandwidth set to 2 gigabits (2*10^9) per second.
     {{.Prompt}} {{.HelpName}} sitea/sourcebucket https://foobar:foo12345@minio.siteb.example.com/targetbucket \
         --service "replication" --region "us-west-1 --bandwidth "2G"

  3. Set a new remote transition target 'srcbucket' in region "us-west-1" on https://minio2:9000 for bucket 'srcbucket' on MinIO server.
     {{.DisableHistory}}
     {{.Prompt}} {{.HelpName}} myminio/srcbucket https://foobar:foo12345@minio2:9000/srcbucket --service "ilm" --region "us-west-1" --label "hdd-tier"
     {{.EnableHistory}}
`,
}

// checkAdminBucketRemoteAddSyntax - validate all the passed arguments
func checkAdminBucketRemoteAddSyntax(ctx *cli.Context) {
	argsNr := len(ctx.Args())
	if argsNr < 2 {
		showCommandHelpAndExit(ctx, ctx.Command.Name, 1) // last argument is exit code
	}
	if argsNr > 2 {
		fatalIf(errInvalidArgument().Trace(ctx.Args().Tail()...),
			"Incorrect number of arguments for remote add command.")
	}
}

// RemoteMessage container for content message structure
type RemoteMessage struct {
	op           string
	Status       string `json:"status"`
	AccessKey    string `json:"accessKey,omitempty"`
	SecretKey    string `json:"secretKey,omitempty"`
	SourceBucket string `json:"sourceBucket"`
	TargetURL    string `json:"TargetURL,omitempty"`
	TargetBucket string `json:"TargetBucket,omitempty"`
	RemoteARN    string `json:"RemoteARN,omitempty"`
	Path         string `json:"path,omitempty"`
	Region       string `json:"region,omitempty"`
	ServiceType  string `json:"service"`
	TargetLabel  string `json:"TargetLabel"`
	Bandwidth    int64  `json:"bandwidth"`
}

func (r RemoteMessage) String() string {
	switch r.op {
	case "ls":
		message := console.Colorize("TargetURL", fmt.Sprintf("%s ", r.TargetURL))
		if r.TargetLabel != "" {
			message += console.Colorize("TargetLabel", fmt.Sprintf("%s ", r.TargetLabel))
		}

		message += console.Colorize("SourceBucket", r.SourceBucket)
		message += console.Colorize("Arrow", "->")
		message += console.Colorize("TargetBucket", r.TargetBucket)
		message += " "
		message += console.Colorize("ARN", r.RemoteARN)
		return message
	case "rm":
		return console.Colorize("RemoteMessage", "Removed remote target for `"+r.SourceBucket+"` bucket successfully.")
	case "add":
		return console.Colorize("RemoteMessage", "Remote ARN = `"+r.RemoteARN+"`.")
	case "edit":
		return console.Colorize("RemoteMessage", "Credentials updated successfully for target with ARN:`"+r.RemoteARN+"`.")
	}
	return ""
}

// JSON returns jsonified message
func (r RemoteMessage) JSON() string {
	r.Status = "success"
	jsonMessageBytes, e := json.MarshalIndent(r, "", " ")
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")

	return string(jsonMessageBytes)
}

var targetKeys = regexp.MustCompile("^(https?://)(.*?):(.*?)@(.*?)/(.*?)$")

// fetchRemoteTarget - returns the dest bucket, dest endpoint, access and secret key
func fetchRemoteTarget(cli *cli.Context) (sourceBucket string, bktTarget *madmin.BucketTarget) {
	args := cli.Args()
	argCount := len(args)
	if argCount < 2 {
		fatalIf(probe.NewError(fmt.Errorf("Missing Remote target configuration")), "Unable to parse remote target")
	}
	_, sourceBucket = url2Alias(args[0])
	TargetURL := args[1]
	path := cli.String("path")
	if !isValidPath(path) {
		fatalIf(errInvalidArgument().Trace(path),
			"Unrecognized bucket path style. Valid options are `[on,off, auto]`.")
	}
	parts := targetKeys.FindStringSubmatch(TargetURL)
	if len(parts) != 6 {
		fatalIf(probe.NewError(fmt.Errorf("invalid url format")), "Malformed Remote target URL")
	}
	accessKey := parts[2]
	secretKey := parts[3]
	parsedURL := fmt.Sprintf("%s%s", parts[1], parts[4])
	TargetBucket := strings.TrimSuffix(parts[5], slashSeperator)
	TargetBucket = strings.TrimPrefix(TargetBucket, slashSeperator)
	u, cerr := url.Parse(parsedURL)
	if cerr != nil {
		fatalIf(probe.NewError(cerr), "Malformed Remote target URL")
	}

	serviceType := cli.String("service")
	if !madmin.ServiceType(serviceType).IsValid() {
		fatalIf(errInvalidArgument().Trace(serviceType), "Invalid service type. Valid option is `[replication]`.")
	}
	bandwidthStr := cli.String("bandwidth")
	bandwidth, err := getBandwidthInBytes(bandwidthStr)
	if err != nil {
		fatalIf(errInvalidArgument().Trace(bandwidthStr), "Invalid bandwidth number")
	}
	console.SetColor(cred, color.New(color.FgYellow, color.Italic))
	creds := &auth.Credentials{AccessKey: accessKey, SecretKey: secretKey}
	bktTarget = &madmin.BucketTarget{
		TargetBucket:   TargetBucket,
		Secure:         u.Scheme == "https",
		Credentials:    creds,
		Endpoint:       u.Host,
		Path:           path,
		API:            "s3v4",
		Type:           madmin.ServiceType(serviceType),
		Region:         cli.String("region"),
		BandwidthLimit: int64(bandwidth),
		Label:          strings.ToUpper(cli.String("label")),
	}
	return sourceBucket, bktTarget
}

func getBandwidthInBytes(bandwidthStr string) (bandwidth uint64, err error) {
	if bandwidthStr != "" {
		bandwidth, err = humanize.ParseBytes(bandwidthStr)
		if err != nil {
			return
		}
	}
	bandwidth = bandwidth / 8
	return
}

// mainAdminBucketRemoteAdd is the handle for "mc admin bucket remote set" command.
func mainAdminBucketRemoteAdd(ctx *cli.Context) error {
	checkAdminBucketRemoteAddSyntax(ctx)

	console.SetColor("RemoteMessage", color.New(color.FgGreen))

	// Get the alias parameter from cli
	args := ctx.Args()
	aliasedURL := args.Get(0)
	// Create a new MinIO Admin Client
	client, cerr := newAdminClient(aliasedURL)
	fatalIf(cerr, "Unable to initialize admin connection.")

	sourceBucket, bktTarget := fetchRemoteTarget(ctx)
	if bktTarget.Type == madmin.ILMService && !ctx.IsSet("label") {
		fatalIf(errInvalidArgument().Trace(args...), "--label flag is required for ilm target")
	}
	arn, e := client.SetRemoteTarget(globalContext, sourceBucket, bktTarget)
	if e != nil {
		fatalIf(probe.NewError(e).Trace(args...), "Unable to configure remote target")
	}

	printMsg(RemoteMessage{
		op:           ctx.Command.Name,
		TargetURL:    bktTarget.URL().String(),
		TargetBucket: bktTarget.TargetBucket,
		AccessKey:    bktTarget.Credentials.AccessKey,
		SourceBucket: sourceBucket,
		RemoteARN:    arn,
	})

	return nil
}
