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
	"os"
	"strings"

	"github.com/minio/cli"
	json "github.com/minio/mc/pkg/colorjson"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/minio/pkg/console"
	iampolicy "github.com/minio/minio/pkg/iam/policy"
	"github.com/minio/minio/pkg/madmin"
)

var adminUserSvcAcctAddFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "access-key",
		Usage: "set an access key for the service account",
	},
	cli.StringFlag{
		Name:  "secret-key",
		Usage: "set a secret key for the service account",
	},
	cli.StringFlag{
		Name:  "policy",
		Usage: "path to a JSON policy file",
	},
}

var adminUserSvcAcctAddCmd = cli.Command{
	Name:         "add",
	Usage:        "add a new service account",
	Action:       mainAdminUserSvcAcctAdd,
	OnUsageError: onUsageError,
	Before:       setGlobalsFromContext,
	Flags:        append(adminUserSvcAcctAddFlags, globalFlags...),
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} TARGET USER

USER:
  Also called as MinIO user.

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
  1. Add a new service account for user 'foobar' to MinIO server.
     {{.Prompt}} {{.HelpName}} myminio foobar
`,
}

// checkAdminUserSvcAcctAddSyntax - validate all the passed arguments
func checkAdminUserSvcAcctAddSyntax(ctx *cli.Context) {
	if len(ctx.Args()) != 1 {
		fatalIf(errInvalidArgument().Trace(ctx.Args().Tail()...),
			"Incorrect number of arguments for user svcacct add command.")
	}
}

// svcAcctMessage container for content message structure
type svcAcctMessage struct {
	op         string
	Status     string   `json:"status"`
	AccessKey  string   `json:"accessKey,omitempty"`
	SecretKey  string   `json:"secretKey,omitempty"`
	PolicyName string   `json:"policyName,omitempty"`
	UserStatus string   `json:"userStatus,omitempty"`
	MemberOf   []string `json:"memberOf,omitempty"`
}

func (u svcAcctMessage) String() string {
	switch u.op {
	case "list":
		userFieldMaxLen := 9
		accessFieldMaxLen := 20
		policyFieldMaxLen := 20

		// Create a new pretty table with cols configuration
		return newPrettyTable("  ",
			Field{"UserStatus", userFieldMaxLen},
			Field{"AccessKey", accessFieldMaxLen},
			Field{"PolicyName", policyFieldMaxLen},
		).buildRow(u.UserStatus, u.AccessKey, u.PolicyName)
	case "info":
		return console.Colorize("UserMessage", strings.Join(
			[]string{
				fmt.Sprintf("AccessKey: %s", u.AccessKey),
				fmt.Sprintf("Status: %s", u.UserStatus),
				fmt.Sprintf("PolicyName: %s", u.PolicyName),
				fmt.Sprintf("MemberOf: %s", strings.Join(u.MemberOf, ",")),
			}, "\n"))
	case "remove":
		return console.Colorize("UserMessage", "Removed user `"+u.AccessKey+"` successfully.")
	case "disable":
		return console.Colorize("UserMessage", "Disabled user `"+u.AccessKey+"` successfully.")
	case "enable":
		return console.Colorize("UserMessage", "Enabled user `"+u.AccessKey+"` successfully.")
	case "add":
		return console.Colorize("UserMessage", "Added user `"+u.AccessKey+"` successfully.")
	}
	return ""
}

func (u svcAcct) JSON() string {
	u.Status = "success"
	jsonMessageBytes, e := json.MarshalIndent(u, "", " ")
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")

	return string(jsonMessageBytes)
}

// mainAdminUserSvcAcctAdd is the handle for "mc admin user svcacct add" command.
func mainAdminUserSvcAcctAdd(ctx *cli.Context) error {
	checkAdminUserSvcAcctAddSyntax(ctx)

	// Get the alias parameter from cli
	args := ctx.Args()
	aliasedURL := args.Get(0)
	user := args.Get(1)

	accessKey := ctx.String("access-key")
	secretKey := ctx.String("secret-key")
	policyPath := ctx.String("policy")

	// Create a new MinIO Admin Client
	client, err := newAdminClient(aliasedURL)
	fatalIf(err, "Unable to initialize admin connection.")

	var policy *iampolicy.Policy

	if policyPath != "" {
		var err error
		f, err := os.Open(policyPath)
		fatalIf(err, "Unable to open the policy document.")
		policy, err = iampolicy.ParseConfig(f)
		fatalIf(err, "Unable to parse the policy document.")
	}

	opts := madmin.AddServiceAccountReq{
		Policy:     policy,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		ParentUser: user,
	}

	fatalIf(probe.NewError(client.AddServiceAccount(globalContext, opts)).Trace(args...), "Unable to add new user")

	printMsg(svcAcctMessage{
		op:         "add",
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		UserStatus: "enabled",
	})

	return nil
}
