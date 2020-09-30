/*
 * MinIO Client (C) 2014-2019 MinIO, Inc.
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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/minio/cli"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/minio/pkg/console"
	"github.com/minio/minio/pkg/words"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/profile"
)

var (
	// global flags for mc.
	mcFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "autocompletion",
			Usage: "install auto-completion for your shell",
		},
	}
)

// Help template for mc
var mcHelpTemplate = `NAME:
  {{.Name}} - {{.Usage}}

USAGE:
  {{.Name}} {{if .VisibleFlags}}[FLAGS] {{end}}COMMAND{{if .VisibleFlags}} [COMMAND FLAGS | -h]{{end}} [ARGUMENTS...]

COMMANDS:
  {{range .VisibleCommands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
  {{end}}{{if .VisibleFlags}}
GLOBAL FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}{{end}}
TIP:
  Use '{{.Name}} --autocompletion' to enable shell autocompletion

VERSION:
  ` + ReleaseTag +
	`{{ "\n"}}{{range $key, $value := ExtraInfo}}
{{$key}}:
  {{$value}}
{{end}}`

// Main starts mc application
func Main(args []string) {

	if len(args) > 1 {
		switch args[1] {
		case "mc", filepath.Base(args[0]):
			mainComplete()
			return
		}
	}

	// Enable profiling supported modes are [cpu, mem, block].
	// ``MC_PROFILER`` supported options are [cpu, mem, block].
	switch os.Getenv("MC_PROFILER") {
	case "cpu":
		defer profile.Start(profile.CPUProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
	case "mem":
		defer profile.Start(profile.MemProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
	case "block":
		defer profile.Start(profile.BlockProfile, profile.ProfilePath(mustGetProfileDir())).Stop()
	}

	probe.Init() // Set project's root source path.
	probe.SetAppInfo("Release-Tag", ReleaseTag)
	probe.SetAppInfo("Commit", ShortCommitID)

	// Fetch terminal size, if not available, automatically
	// set globalQuiet to true.
	if w, e := pb.GetTerminalWidth(); e != nil {
		globalQuiet = true
	} else {
		globalTermWidth = w
	}

	// Set the mc app name.
	appName := filepath.Base(args[0])
	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(appName), ".exe") {
		// Trim ".exe" from Windows executable.
		appName = appName[:strings.LastIndex(appName, ".")]
	}

	// Monitor OS exit signals and cancel the global context in such case
	go trapSignals(os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)

	// Run the app - exit on error.
	if err := registerApp(appName).Run(args); err != nil {
		os.Exit(1)
	}
}

// Function invoked when invalid command is passed.
func commandNotFound(ctx *cli.Context, command string) {
	msg := fmt.Sprintf("`%s` is not a mc command. See `mc --help`.", command)
	closestCommands := findClosestCommands(command)
	if len(closestCommands) > 0 {
		msg += "\n\nDid you mean one of these?\n"
		if len(closestCommands) == 1 {
			cmd := closestCommands[0]
			msg += fmt.Sprintf("        `%s`", cmd)
		} else {
			for _, cmd := range closestCommands {
				msg += fmt.Sprintf("        `%s`\n", cmd)
			}
		}
	}
	fatalIf(errDummy().Trace(), msg)
}

// Check for sane config environment early on and gracefully report.
func checkConfig() {
	// Refresh the config once.
	loadMcConfig = loadMcConfigFactory()
	// Ensures config file is sane.
	config, err := loadMcConfig()
	// Verify if the path is accesible before validating the config
	fatalIf(err.Trace(mustGetMcConfigPath()), "Unable to access configuration file.")

	// Validate and print error messges
	ok, errMsgs := validateConfigFile(config)
	if !ok {
		var errorMsg bytes.Buffer
		for index, errMsg := range errMsgs {
			// Print atmost 10 errors
			if index > 10 {
				break
			}
			errorMsg.WriteString(errMsg + "\n")
		}
		console.Fatal(errorMsg.String())
	}
}

func migrate() {
	// Fix broken config files if any.
	fixConfig()

	// Migrate config files if any.
	migrateConfig()

	// Migrate session files if any.
	migrateSession()

	// Migrate shared urls if any.
	migrateShare()
}

// Get os/arch/platform specific information.
// Returns a map of current os/arch/platform/memstats.
func getSystemData() map[string]string {
	host, e := os.Hostname()
	fatalIf(probe.NewError(e), "Unable to determine the hostname.")

	memstats := &runtime.MemStats{}
	runtime.ReadMemStats(memstats)
	mem := fmt.Sprintf("Used: %s | Allocated: %s | UsedHeap: %s | AllocatedHeap: %s",
		pb.Format(int64(memstats.Alloc)).To(pb.U_BYTES),
		pb.Format(int64(memstats.TotalAlloc)).To(pb.U_BYTES),
		pb.Format(int64(memstats.HeapAlloc)).To(pb.U_BYTES),
		pb.Format(int64(memstats.HeapSys)).To(pb.U_BYTES))
	platform := fmt.Sprintf("Host: %s | OS: %s | Arch: %s", host, runtime.GOOS, runtime.GOARCH)
	goruntime := fmt.Sprintf("Version: %s | CPUs: %s", runtime.Version(), strconv.Itoa(runtime.NumCPU()))
	return map[string]string{
		"PLATFORM": platform,
		"RUNTIME":  goruntime,
		"MEM":      mem,
	}
}

// initMC - initialize 'mc'.
func initMC() {
	// Check if mc config exists.
	if !isMcConfigExists() {
		err := saveMcConfig(newMcConfig())
		fatalIf(err.Trace(), "Unable to save new mc config.")

		if !globalQuiet && !globalJSON {
			console.Infoln("Configuration written to `" + mustGetMcConfigPath() + "`. Please update your access credentials.")
		}
	}

	// Check if mc session directory exists.
	if !isSessionDirExists() {
		fatalIf(createSessionDir().Trace(), "Unable to create session config directory.")
	}

	// Check if mc share directory exists.
	if !isShareDirExists() {
		initShareConfig()
	}

	// Check if certs dir exists
	if !isCertsDirExists() {
		fatalIf(createCertsDir().Trace(), "Unable to create `CAs` directory.")
	}

	// Check if CAs dir exists
	if !isCAsDirExists() {
		fatalIf(createCAsDir().Trace(), "Unable to create `CAs` directory.")
	}

	// Load all authority certificates present in CAs dir
	loadRootCAs()

}

const bashAutoCompleteTemplate = `

#! /bin/bash

_cli_bash_autocomplete() {
  if [[ "${COMP_WORDS[0]}" != "source" ]]; then
      local cur opts base
      COMPREPLY=()
      cur="${COMP_WORDS[COMP_CWORD]}"
      if [[ "$cur" == "-"* ]]; then
        opts=$( ${COMP_WORDS[@]:0:$COMP_CWORD} ${cur} --generate-bash-completion )
      else
        opts=$( ${COMP_WORDS[@]:0:$COMP_CWORD} --generate-bash-completion )
      fi
      COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
      return 0
  fi
}

complete -o bashdefault -o default -o nospace -F _cli_bash_autocomplete ${MC_PROG}

`

const shellCompletionFName = ".shell_completion"

func installAutoCompletion() {
	if runtime.GOOS == "windows" {
		console.Infoln("autocompletion feature is not available for this operating system")
		return
	}

	if os.Getenv("SHELL") != "/bin/bash" {
		return
	}

	mcDir, err := getMcConfigDir()
	if err != nil {
	}

	homeDir, e := homedir.Dir()
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to install auto-completion.")
	}

	completionScriptFPath := filepath.Join(mcDir, shellCompletionFName)
	binaryName := filepath.Base(os.Args[0])

	completionScript := strings.Replace(bashAutoCompleteTemplate, "${MC_PROG}", binaryName, -1)
	e = ioutil.WriteFile(completionScriptFPath, []byte(completionScript), 0600)
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to install auto-completion.")
	}

	f, e := os.OpenFile(filepath.Join(homeDir, ".bashrc"), os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		fatalIf(probe.NewError(e), "Unable to install auto-completion.")
	}

	defer f.Close()

	fmt.Fprintf(f, "\n")
	fmt.Fprintf(f, "source %s\n", completionScriptFPath)
	fmt.Fprintf(f, "\n")

	console.Infoln("enabled autocompletion in '$SHELLRC'. Please restart your shell.")
}

func registerBefore(ctx *cli.Context) error {
	if ctx.IsSet("config-dir") {
		// Set the config directory.
		setMcConfigDir(ctx.String("config-dir"))
	} else if ctx.GlobalIsSet("config-dir") {
		// Set the config directory.
		setMcConfigDir(ctx.GlobalString("config-dir"))
	}

	// Set global flags.
	setGlobalsFromContext(ctx)

	// Migrate any old version of config / state files to newer format.
	migrate()

	// Initialize default config files.
	initMC()

	// Check if config can be read.
	checkConfig()

	return nil
}

// findClosestCommands to match a given string with commands trie tree.
func findClosestCommands(command string) []string {
	closestCommands := commandsTree.PrefixMatch(command)
	sort.Strings(closestCommands)
	// Suggest other close commands - allow missed, wrongly added and even transposed characters
	for _, value := range commandsTree.Walk(commandsTree.Root()) {
		if sort.SearchStrings(closestCommands, value) < len(closestCommands) {
			continue
		}
		// 2 is arbitrary and represents the max allowed number of typed errors
		if words.DamerauLevenshteinDistance(command, value) < 2 {
			closestCommands = append(closestCommands, value)
		}
	}
	return closestCommands
}

// Check for updates and print a notification message
func checkUpdate(ctx *cli.Context) {
	// Do not print update messages, if quiet flag is set.
	if ctx.Bool("quiet") || ctx.GlobalBool("quiet") {
		// Its OK to ignore any errors during doUpdate() here.
		if updateMsg, _, currentReleaseTime, latestReleaseTime, err := getUpdateInfo(2 * time.Second); err == nil {
			printMsg(updateMessage{
				Status:  "success",
				Message: updateMsg,
			})
		} else {
			printMsg(updateMessage{
				Status:  "success",
				Message: prepareUpdateMessage("Run `mc update`", latestReleaseTime.Sub(currentReleaseTime)),
			})
		}
	}
}

var appCmds = []cli.Command{
	aliasCmd,
	lsCmd,
	mbCmd,
	rbCmd,
	cpCmd,
	mirrorCmd,
	catCmd,
	headCmd,
	pipeCmd,
	shareCmd,
	findCmd,
	sqlCmd,
	statCmd,
	mvCmd,
	treeCmd,
	duCmd,
	retentionCmd,
	legalHoldCmd,
	diffCmd,
	rmCmd,
	versionCmd,
	bucketILMCmd,
	encryptCmd,
	eventCmd,
	watchCmd,
	undoCmd,
	policyCmd,
	tagCmd,
	replicateCmd,
	adminCmd,
	configCmd,
	updateCmd,
}

func registerApp(name string) *cli.App {
	for _, cmd := range appCmds {
		registerCmd(cmd)
	}

	cli.HelpFlag = cli.BoolFlag{
		Name:  "help, h",
		Usage: "show help",
	}

	app := cli.NewApp()
	app.Name = name
	app.Action = func(ctx *cli.Context) error {
		if strings.HasPrefix(ReleaseTag, "RELEASE.") {
			// Check for new updates from dl.min.io.
			checkUpdate(ctx)
		}

		if ctx.Bool("autocompletion") || ctx.GlobalBool("autocompletion") {
			// Install shell completions
			installAutoCompletion()
			return nil
		}

		cli.ShowAppHelp(ctx)
		return exitStatus(globalErrorExitStatus)
	}

	app.Before = registerBefore
	app.ExtraInfo = func() map[string]string {
		if globalDebug {
			return getSystemData()
		}
		return make(map[string]string)
	}

	app.HideHelpCommand = true
	app.Usage = "MinIO Client for cloud storage and filesystems."
	app.Commands = commands
	app.Author = "MinIO, Inc."
	app.Version = ReleaseTag
	app.Flags = append(mcFlags, globalFlags...)
	app.CustomAppHelpTemplate = mcHelpTemplate
	app.CommandNotFound = commandNotFound // handler function declared above.
	app.EnableBashCompletion = true

	return app
}

// mustGetProfilePath must get location that the profile will be written to.
func mustGetProfileDir() string {
	return filepath.Join(mustGetMcConfigDir(), globalProfileDir)
}
