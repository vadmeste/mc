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
	"bytes"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cheggaaa/pb"
	humanize "github.com/dustin/go-humanize"
	"github.com/minio/cli"
	"github.com/minio/madmin-go"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/pkg/console"
	"github.com/minio/pkg/trie"
	"github.com/minio/pkg/words"
	"github.com/pkg/profile"

	completeinstall "github.com/posener/complete/cmd/install"
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

var j = `
{"ScannedItemsCount":11,"HealDisks":null,"sets":[{"id":"0-0","pool_index":0,"set_index":0,"heal_status":"","heal_priority":"","total_objects":0,"disks":[{"endpoint":"http://localhost:9001/tmp/xl/1","rootDisk":true,"path":"/tmp/xl/1","state":"ok","uuid":"49d9d5f2-6201-4c21-8d09-7164bfe4e4ea","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{"apiLatencies":{"AppendFile":"0s","CheckParts":"0s","CreateFile":"0s","Delete":"218.966µs","DeleteVersion":"0s","DeleteVersions":"0s","DeleteVol":"0s","ListDir":"0s","ListVols":"221.722µs","MakeVol":"0s","MakeVolBulk":"77.049µs","ReadAll":"18.521µs","ReadFile":"0s","ReadFileStream":"0s","ReadVersion":"49.611µs","RenameData":"27.54442ms","RenameFile":"40.646µs","StatVol":"37µs","UpdateMetadata":"0s","VerifyFile":"0s","WalkDir":"910.137µs","WriteAll":"7.06607ms","WriteMetadata":"0s","storageStatInfoFile":"0s"},"apiCalls":{"AppendFile":0,"CheckParts":0,"CreateFile":0,"Delete":1,"DeleteVersion":0,"DeleteVersions":0,"DeleteVol":0,"ListDir":0,"ListVols":2601,"MakeVol":0,"MakeVolBulk":1,"ReadAll":1,"ReadFile":0,"ReadFileStream":0,"ReadVersion":40,"RenameData":744,"RenameFile":1,"StatVol":1946,"UpdateMetadata":0,"VerifyFile":0,"WalkDir":213,"WriteAll":1,"WriteMetadata":0,"storageStatInfoFile":0}},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":0},{"endpoint":"http://localhost:9002/tmp/xl/2","rootDisk":true,"path":"/tmp/xl/2","state":"ok","uuid":"4705e398-5493-43d1-8d99-fe4e173429df","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":1},{"endpoint":"http://localhost:9003/tmp/xl/3","rootDisk":true,"path":"/tmp/xl/3","state":"ok","uuid":"e5002524-ff20-4d0f-8e79-49a62c8f9190","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":2},{"endpoint":"http://localhost:9004/tmp/xl/4","rootDisk":true,"path":"/tmp/xl/4","state":"ok","uuid":"55e88871-81c9-4bd3-a699-3ef6cb6c7e96","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":3},{"endpoint":"http://localhost:9005/tmp/xl/5","rootDisk":true,"path":"/tmp/xl/5","state":"ok","uuid":"74b393f6-e9af-42ec-b4a1-9adf667e503a","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":4},{"endpoint":"http://localhost:9006/tmp/xl/6","rootDisk":true,"path":"/tmp/xl/6","state":"ok","uuid":"b4a2f0ac-b25a-477c-8cbe-25b54a068f44","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":5},{"endpoint":"http://localhost:9007/tmp/xl/7","rootDisk":true,"path":"/tmp/xl/7","state":"ok","uuid":"06056200-96fc-4ac6-bfd5-f1a812b1be58","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":6},{"endpoint":"http://localhost:9008/tmp/xl/8","rootDisk":true,"path":"/tmp/xl/8","state":"ok","uuid":"f6dfac66-9437-4ea1-9ee8-77b7c3981731","totalspace":101187674112,"usedspace":27540430848,"availspace":73647243264,"metrics":{},"free_inodes":6165803,"pool_index":0,"set_index":0,"disk_index":7}]}],"mrf":{"localhost:9001":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"},"localhost:9002":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"},"localhost:9003":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"},"localhost:9004":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"},"localhost:9005":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"},"localhost:9006":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"},"localhost:9007":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"},"localhost:9008":{"bytes_healed":0,"items_healed":0,"total_items":0,"total_bytes":0,"started":"0001-01-01T00:00:00Z"}},"sc_parity":{"REDUCED_REDUNDANCY":2,"STANDARD":4}}
`

// String colorized to show background heal status message.
func testJSON() string {

	var msg strings.Builder

	var healInfo madmin.BgHealState
	_ = stdjson.Unmarshal([]byte(j), &healInfo)

	parity, showFailure := healInfo.SCParity["STANDARD"]

	allDisks := getAllDisks(healInfo.Sets)

	setsStatus := generateSetsStatus(allDisks)
	serversStatus := generateServersStatus(allDisks)

	var poolsTolerance = make(map[int]poolStatus)
	pools := getPoolsIndexes(allDisks)
	for _, pool := range pools {
		tolerance, endpoints := computePoolTolerance(pool, parity, serversStatus, setsStatus)
		poolsTolerance[pool] = poolStatus{tolerance: tolerance, endpoints: endpoints}
	}

	for endpoint, serverStatus := range serversStatus {
		fmt.Fprintf(&msg, "%s:\n", endpoint)
		fmt.Fprintf(&msg, "  + Pool : %d\n", serverStatus.pool+1)
		if showFailure {
			fmt.Fprintf(&msg, "  + Tolerance : %d\n", poolsTolerance[serverStatus.pool].tolerance)
		}
		for _, d := range serverStatus.disks {
			state := d.state
			if state == "ok" && d.healing {
				state = "healing"
			}
			fmt.Fprintf(&msg, "  |_ %s : %s\n", d.path, state)
			if d.healing {
				fmt.Fprintf(&msg, "    |_ Estimated : %s\n", setsStatus[d.set].healing.ETA())
			}
			fmt.Fprintf(&msg, "    |_ Capacity : %s/%s\n", humanize.IBytes(d.used), humanize.IBytes(d.total))
			if showFailure {
				fmt.Fprintf(&msg, "    |_ Tolerance : %d\n", parity-setsStatus[d.set].incapableDisks)
			}
		}

		fmt.Fprintf(&msg, "\n")
	}

	if showFailure {
		fmt.Fprintf(&msg, "Server Failure Tolerance:\n")
		fmt.Fprintf(&msg, "========================\n")
		for _, pool := range poolsTolerance {
			fmt.Fprintf(&msg, "Pool 1:\n")
			fmt.Fprintf(&msg, "   Tolerance : %d server(s)\n", pool.tolerance)
			fmt.Fprintf(&msg, "       Nodes :")
			for _, endpoint := range pool.endpoints {
				fmt.Fprintf(&msg, " %s", endpoint)
			}
			fmt.Fprintf(&msg, "\n")
		}
	}

	return msg.String()
}

// Main starts mc application
func Main(args []string) {

	testJSON()
	return

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

// Function invoked when invalid flag is passed
func onUsageError(ctx *cli.Context, err error, subcommand bool) error {
	type subCommandHelp struct {
		flagName string
		usage    string
	}

	// Calculate the maximum width of the flag name field
	// for a good looking printing
	var help = make([]subCommandHelp, len(ctx.Command.Flags))
	maxWidth := 0
	for i, f := range ctx.Command.Flags {
		s := strings.Split(f.String(), "\t")
		if len(s[0]) > maxWidth {
			maxWidth = len(s[0])
		}

		help[i] = subCommandHelp{flagName: s[0], usage: s[1]}
	}
	maxWidth += 2

	var errMsg strings.Builder

	// Do the good-looking printing now
	fmt.Fprintln(&errMsg, "Invalid command usage,", err.Error())
	fmt.Fprintln(&errMsg, "")
	fmt.Fprintln(&errMsg, "SUPPORTED FLAGS:")
	for _, h := range help {
		spaces := string(bytes.Repeat([]byte{' '}, maxWidth-len(h.flagName)))
		fmt.Fprintf(&errMsg, "   %s%s%s\n", h.flagName, spaces, h.usage)
	}
	console.Fatal(errMsg.String())
	return err
}

// Function invoked when invalid command is passed.
func commandNotFound(ctx *cli.Context, cmds []cli.Command) {
	command := ctx.Args().First()
	if command == "" {
		cli.ShowCommandHelp(ctx, command)
		return
	}
	msg := fmt.Sprintf("`%s` is not a recognized command. Get help using `--help` flag.", command)
	var commandsTree = trie.NewTrie()
	for _, cmd := range cmds {
		commandsTree.Insert(cmd.Name)
	}
	closestCommands := findClosestCommands(commandsTree, command)
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

func installAutoCompletion() {
	if runtime.GOOS == "windows" {
		console.Infoln("autocompletion feature is not available for this operating system")
		return
	}

	shellName := os.Getenv("SHELL")
	if shellName == "" {
		ppid := os.Getppid()
		cmd := exec.Command("ps", "-p", strconv.Itoa(ppid), "-o", "comm=")
		ppName, err := cmd.Output()
		if err != nil {
			fatalIf(probe.NewError(err), "Failed to enable autocompletion. Cannot determine shell type and"+
				"no SHELL environment variable found")
		}
		shellName = strings.TrimSpace(string(ppName))
		console.Infoln("No 'SHELL' env var. Your shell is auto determined as '" + shellName + "'.")
	} else {
		console.Infoln("Your shell is set to '" + shellName + "', by env var 'SHELL'.")
	}
	shellName = strings.ToLower(filepath.Base(shellName))

	supportedShells := map[string]bool{
		"bash": true,
		"zsh":  true,
		"fish": true,
	}

	if !supportedShells[shellName] {
		fatalIf(probe.NewError(errors.New("")),
			"'"+shellName+"' is not a supported shell. "+
				"Supported shells are: bash, zsh, fish")
	}

	err := completeinstall.Install(filepath.Base(os.Args[0]))
	var printMsg string
	if err != nil && strings.Contains(err.Error(), "* already installed") {
		errStr := err.Error()[strings.Index(err.Error(), "\n")+1:]
		re := regexp.MustCompile(`[::space::]*\*.*` + shellName + `.*`)
		relatedMsg := re.FindStringSubmatch(errStr)
		if len(relatedMsg) > 0 {
			printMsg = "\n" + relatedMsg[0]
		} else {
			printMsg = ""
		}
	}
	if printMsg != "" {
		if completeinstall.IsInstalled(filepath.Base(os.Args[0])) || completeinstall.IsInstalled("mc") {
			console.Infoln("autocompletion is enabled.", printMsg)
		} else {
			fatalIf(probe.NewError(err), "Unable to install auto-completion.")
		}
	} else {
		console.Infoln("enabled autocompletion in your '" + shellName + "' rc file. Please restart your shell.")
	}
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
func findClosestCommands(commandsTree *trie.Trie, command string) []string {
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
	ilmCmd,
	encryptCmd,
	eventCmd,
	watchCmd,
	undoCmd,
	anonymousCmd,
	policyCmd,
	tagCmd,
	replicateCmd,
	adminCmd,
	configCmd,
	updateCmd,
}

func registerApp(name string) *cli.App {
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

		if ctx.Args().First() != "" {
			commandNotFound(ctx, app.Commands)
		} else {
			cli.ShowAppHelp(ctx)
		}

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
	app.Commands = appCmds
	app.Author = "MinIO, Inc."
	app.Version = ReleaseTag
	app.Flags = append(mcFlags, globalFlags...)
	app.CustomAppHelpTemplate = mcHelpTemplate
	app.EnableBashCompletion = true
	app.OnUsageError = onUsageError

	return app
}

// mustGetProfilePath must get location that the profile will be written to.
func mustGetProfileDir() string {
	return filepath.Join(mustGetMcConfigDir(), globalProfileDir)
}
