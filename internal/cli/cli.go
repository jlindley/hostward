package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"hostward/internal/build"
	"hostward/internal/config"
	"hostward/internal/launchd"
	"hostward/internal/monitor"
	"hostward/internal/scaffold"
	"hostward/internal/scheduler"
	"hostward/internal/service"
	"hostward/internal/shell"
	"hostward/internal/state"
)

type Runner struct {
	stdout io.Writer
	stderr io.Writer
	paths  config.Paths
}

func New() (*Runner, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}

	return &Runner{
		stdout: os.Stdout,
		stderr: os.Stderr,
		paths:  paths,
	}, nil
}

func (r *Runner) Run(args []string) error {
	if len(args) == 0 {
		return r.help()
	}

	switch args[0] {
	case "banner":
		return r.runBanner(args[1:])
	case "add":
		return r.runAdd(args[1:])
	case "build":
		return r.runBuild(args[1:])
	case "doctor":
		return r.runDoctor(args[1:])
	case "disable":
		return r.runEnableDisable(args[1:], true)
	case "env":
		return r.runEnv(args[1:])
	case "enable":
		return r.runEnableDisable(args[1:], false)
	case "help", "-h", "--help":
		return r.help()
	case "launchd":
		return r.runLaunchd(args[1:])
	case "monitor":
		return r.runMonitor(args[1:])
	case "monitors":
		return r.runMonitors(args[1:])
	case "notify":
		return r.runNotify(args[1:])
	case "poke":
		return r.runPoke(args[1:])
	case "scheduler":
		return r.runScheduler(args[1:])
	case "shell-init":
		return r.runShellInit(args[1:])
	case "status":
		return r.runStatus(args[1:])
	case "version", "-v", "--version":
		_, err := fmt.Fprintln(r.stdout, "hostward dev")
		return err
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], helpText)
	}
}

func (r *Runner) help() error {
	_, err := fmt.Fprint(r.stdout, helpText)
	return err
}

func (r *Runner) runBanner(args []string) error {
	mode := shell.BannerCount
	if len(args) > 1 {
		return fmt.Errorf("usage: hostward banner [count|list]")
	}
	if len(args) == 1 {
		switch args[0] {
		case string(shell.BannerCount):
			mode = shell.BannerCount
		case string(shell.BannerList):
			mode = shell.BannerList
		default:
			return fmt.Errorf("usage: hostward banner [count|list]")
		}
	}

	snapshot, err := state.LoadSnapshot(r.paths.CurrentStatePath)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(r.stdout, shell.Banner(snapshot, mode))
	return err
}

func (r *Runner) runNotify(args []string) error {
	if len(args) != 1 || args[0] != "test" {
		return fmt.Errorf("usage: hostward notify test")
	}

	bundle, err := config.Load(r.paths)
	if err != nil {
		return err
	}
	if !bundle.Global.Notifications.Enabled {
		_, err := fmt.Fprintln(r.stdout, "notifications disabled in config")
		return err
	}

	svc := service.New(r.paths)
	if err := svc.NotifyTest(); err != nil {
		return err
	}

	_, err = fmt.Fprintln(r.stdout, "notification requested")
	return err
}

func (r *Runner) runAdd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward add <script|deadman> ...")
	}

	switch args[0] {
	case "script":
		return r.runAddScript(args[1:])
	case "deadman":
		return r.runAddDeadman(args[1:])
	case "dir-exists":
		return r.runAddDirExists(args[1:])
	case "file-exists":
		return r.runAddFileExists(args[1:])
	case "file-freshness":
		return r.runAddFileFreshness(args[1:])
	case "free-space":
		return r.runAddFreeSpace(args[1:])
	case "process-running":
		return r.runAddProcessRunning(args[1:])
	case "dir-size":
		return r.runAddDirSize(args[1:])
	default:
		return fmt.Errorf("usage: hostward add <script|deadman|file-exists|dir-exists|file-freshness|free-space|dir-size|process-running> ...")
	}
}

func (r *Runner) runBuild(args []string) error {
	flags := flag.NewFlagSet("hostward build", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", filepath.Join(r.paths.Home, ".local", "bin", "hostward"), "")
	if err := flags.Parse(args); err != nil || len(flags.Args()) != 0 {
		return fmt.Errorf("usage: hostward build [--output <path>]")
	}

	outputPath, err := expandHome(*output, r.paths.Home)
	if err != nil {
		return err
	}
	if err := build.Build(outputPath); err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "built %s\n", outputPath)
	return err
}

func (r *Runner) runAddScript(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward add script <name> --every <duration> [--timeout <duration>] [--display-name <name>] [--working-dir <dir>] [--no-inherit-env] [--max-output-bytes <n>] -- <command...>")
	}

	id := args[0]
	flags := flag.NewFlagSet("hostward add script", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	every := flags.String("every", "", "")
	timeout := flags.String("timeout", config.DefaultScriptTimeout.String(), "")
	displayName := flags.String("display-name", "", "")
	workingDir := flags.String("working-dir", "", "")
	noInheritEnv := flags.Bool("no-inherit-env", false, "")
	maxOutputBytes := flags.Int("max-output-bytes", config.DefaultMaxOutputBytes, "")

	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("usage: hostward add script <name> --every <duration> [--timeout <duration>] [--display-name <name>] [--working-dir <dir>] [--no-inherit-env] [--max-output-bytes <n>] -- <command...>")
	}
	rest := flags.Args()
	if len(rest) < 1 {
		return fmt.Errorf("usage: hostward add script <name> --every <duration> [--timeout <duration>] [--display-name <name>] [--working-dir <dir>] [--no-inherit-env] [--max-output-bytes <n>] -- <command...>")
	}

	path, err := scaffold.AddScript(r.paths, scaffold.ScriptOptions{
		ID:             id,
		DisplayName:    *displayName,
		Every:          *every,
		Timeout:        *timeout,
		Command:        rest,
		WorkingDir:     *workingDir,
		NoInheritEnv:   *noInheritEnv,
		MaxOutputBytes: *maxOutputBytes,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", path)
	return err
}

func (r *Runner) runAddDeadman(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward add deadman <name> --every <duration> --grace <duration> [--display-name <name>]")
	}

	id := args[0]
	flags := flag.NewFlagSet("hostward add deadman", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	every := flags.String("every", "", "")
	grace := flags.String("grace", "", "")
	displayName := flags.String("display-name", "", "")

	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("usage: hostward add deadman <name> --every <duration> --grace <duration> [--display-name <name>]")
	}
	rest := flags.Args()
	if len(rest) != 0 {
		return fmt.Errorf("usage: hostward add deadman <name> --every <duration> --grace <duration> [--display-name <name>]")
	}

	path, err := scaffold.AddDeadman(r.paths, scaffold.DeadmanOptions{
		ID:          id,
		DisplayName: *displayName,
		Every:       *every,
		Grace:       *grace,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", path)
	return err
}

func (r *Runner) runAddFileExists(args []string) error {
	pathValue, common, err := r.parseBuiltinAdd(args, "usage: hostward add file-exists <name> --path <path> --every <duration> [--timeout <duration>] [--display-name <name>]")
	if err != nil {
		return err
	}

	written, err := scaffold.AddFileExists(r.paths, scaffold.FileExistsOptions{
		ID:          common.id,
		DisplayName: common.displayName,
		Every:       common.every,
		Timeout:     common.timeout,
		Path:        pathValue,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", written)
	return err
}

func (r *Runner) runAddDirExists(args []string) error {
	pathValue, common, err := r.parseBuiltinAdd(args, "usage: hostward add dir-exists <name> --path <path> --every <duration> [--timeout <duration>] [--display-name <name>]")
	if err != nil {
		return err
	}

	written, err := scaffold.AddDirectoryExists(r.paths, scaffold.DirectoryExistsOptions{
		ID:          common.id,
		DisplayName: common.displayName,
		Every:       common.every,
		Timeout:     common.timeout,
		Path:        pathValue,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", written)
	return err
}

func (r *Runner) runAddProcessRunning(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward add process-running <name> --match <pattern> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	id := args[0]
	flags := flag.NewFlagSet("hostward add process-running", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	every := flags.String("every", "", "")
	timeout := flags.String("timeout", config.DefaultScriptTimeout.String(), "")
	displayName := flags.String("display-name", "", "")
	match := flags.String("match", "", "")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("usage: hostward add process-running <name> --match <pattern> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}
	if len(flags.Args()) != 0 {
		return fmt.Errorf("usage: hostward add process-running <name> --match <pattern> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	written, err := scaffold.AddProcessRunning(r.paths, scaffold.ProcessRunningOptions{
		ID:          id,
		DisplayName: *displayName,
		Every:       *every,
		Timeout:     *timeout,
		Match:       *match,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", written)
	return err
}

func (r *Runner) runAddFileFreshness(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward add file-freshness <name> --path <path> --max-age <duration> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	id := args[0]
	flags := flag.NewFlagSet("hostward add file-freshness", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pathValue := flags.String("path", "", "")
	maxAge := flags.String("max-age", "", "")
	every := flags.String("every", "", "")
	timeout := flags.String("timeout", config.DefaultScriptTimeout.String(), "")
	displayName := flags.String("display-name", "", "")
	if err := flags.Parse(args[1:]); err != nil || len(flags.Args()) != 0 {
		return fmt.Errorf("usage: hostward add file-freshness <name> --path <path> --max-age <duration> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	written, err := scaffold.AddFileFreshness(r.paths, scaffold.FileFreshnessOptions{
		ID:          id,
		DisplayName: *displayName,
		Every:       *every,
		Timeout:     *timeout,
		Path:        *pathValue,
		MaxAge:      *maxAge,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", written)
	return err
}

func (r *Runner) runAddFreeSpace(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward add free-space <name> --path <path> --min-free-percent <n> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	id := args[0]
	flags := flag.NewFlagSet("hostward add free-space", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pathValue := flags.String("path", "", "")
	minFreePercent := flags.Int("min-free-percent", 0, "")
	every := flags.String("every", "", "")
	timeout := flags.String("timeout", config.DefaultScriptTimeout.String(), "")
	displayName := flags.String("display-name", "", "")
	if err := flags.Parse(args[1:]); err != nil || len(flags.Args()) != 0 {
		return fmt.Errorf("usage: hostward add free-space <name> --path <path> --min-free-percent <n> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	written, err := scaffold.AddFreeSpace(r.paths, scaffold.FreeSpaceOptions{
		ID:             id,
		DisplayName:    *displayName,
		Every:          *every,
		Timeout:        *timeout,
		Path:           *pathValue,
		MinFreePercent: *minFreePercent,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", written)
	return err
}

func (r *Runner) runAddDirSize(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward add dir-size <name> --path <path> --max-bytes <n> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	id := args[0]
	flags := flag.NewFlagSet("hostward add dir-size", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pathValue := flags.String("path", "", "")
	maxBytes := flags.Int64("max-bytes", 0, "")
	every := flags.String("every", "", "")
	timeout := flags.String("timeout", config.DefaultScriptTimeout.String(), "")
	displayName := flags.String("display-name", "", "")
	if err := flags.Parse(args[1:]); err != nil || len(flags.Args()) != 0 {
		return fmt.Errorf("usage: hostward add dir-size <name> --path <path> --max-bytes <n> --every <duration> [--timeout <duration>] [--display-name <name>]")
	}

	written, err := scaffold.AddDirectorySize(r.paths, scaffold.DirectorySizeOptions{
		ID:          id,
		DisplayName: *displayName,
		Every:       *every,
		Timeout:     *timeout,
		Path:        *pathValue,
		MaxBytes:    *maxBytes,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "created %s\n", written)
	return err
}

type builtinAddArgs struct {
	id          string
	every       string
	timeout     string
	displayName string
}

func (r *Runner) parseBuiltinAdd(args []string, usage string) (string, builtinAddArgs, error) {
	flags := flag.NewFlagSet("hostward add builtin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pathValue := flags.String("path", "", "")
	every := flags.String("every", "", "")
	timeout := flags.String("timeout", config.DefaultScriptTimeout.String(), "")
	displayName := flags.String("display-name", "", "")
	if len(args) < 1 {
		return "", builtinAddArgs{}, fmt.Errorf("%s", usage)
	}
	id := args[0]
	if err := flags.Parse(args[1:]); err != nil {
		return "", builtinAddArgs{}, fmt.Errorf("%s", usage)
	}
	if len(flags.Args()) != 0 {
		return "", builtinAddArgs{}, fmt.Errorf("%s", usage)
	}

	return *pathValue, builtinAddArgs{
		id:          id,
		every:       *every,
		timeout:     *timeout,
		displayName: *displayName,
	}, nil
}

func (r *Runner) runDoctor(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: hostward doctor")
	}

	report, err := buildDoctorReport(r.paths, time.Now().UTC())
	if _, writeErr := fmt.Fprint(r.stdout, report); writeErr != nil {
		return writeErr
	}
	return err
}

func (r *Runner) runEnv(args []string) error {
	if len(args) != 1 || args[0] != "failing-count" {
		return fmt.Errorf("usage: hostward env failing-count")
	}

	snapshot, err := state.LoadSnapshot(r.paths.CurrentStatePath)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(r.stdout, shell.FailingCount(snapshot))
	return err
}

func (r *Runner) runEnableDisable(args []string, disabled bool) error {
	if len(args) != 1 {
		if disabled {
			return fmt.Errorf("usage: hostward disable <name>")
		}
		return fmt.Errorf("usage: hostward enable <name>")
	}

	svc := service.New(r.paths)
	monitorSnapshot, err := svc.SetMonitorDisabled(args[0], disabled)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "%s: %s\n", monitorSnapshot.ID, monitorSnapshot.Status)
	return err
}

func (r *Runner) runMonitor(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward monitor <show|run> <name>")
	}

	switch args[0] {
	case "run":
		if len(args) != 2 {
			return fmt.Errorf("usage: hostward monitor run <name>")
		}
		return r.runMonitorRun(args[1])
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: hostward monitor show <name>")
		}
		return r.runMonitorShow(args[1])
	default:
		return fmt.Errorf("usage: hostward monitor <show|run> <name>")
	}
}

func (r *Runner) runLaunchd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward launchd <print-plist|install|uninstall|status>")
	}

	switch args[0] {
	case "print-plist":
		return r.runLaunchdPrintPlist(args[1:])
	case "install":
		return r.runLaunchdInstall(args[1:])
	case "uninstall":
		return r.runLaunchdUninstall(args[1:])
	case "status":
		return r.runLaunchdStatus(args[1:])
	default:
		return fmt.Errorf("usage: hostward launchd <print-plist|install|uninstall|status>")
	}
}

func (r *Runner) runLaunchdPrintPlist(args []string) error {
	agent, err := r.parseLaunchdAgent(args)
	if err != nil {
		return err
	}

	rendered, err := launchd.Render(agent)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(r.stdout, rendered)
	return err
}

func (r *Runner) runLaunchdInstall(args []string) error {
	agent, err := r.parseLaunchdAgent(args)
	if err != nil {
		return err
	}
	if launchd.LooksEphemeralBinary(agent.ProgramArguments[0]) {
		return fmt.Errorf("refusing to install launchd agent with unstable binary path %q; build a stable binary first with `hostward build --output ~/.local/bin/hostward` or pass --binary", agent.ProgramArguments[0])
	}

	if err := launchd.Install(r.paths, agent); err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "installed %s\n", r.paths.LaunchAgentPath)
	return err
}

func (r *Runner) runLaunchdUninstall(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: hostward launchd uninstall")
	}

	if err := launchd.Uninstall(r.paths, launchd.Label); err != nil {
		return err
	}

	_, err := fmt.Fprintln(r.stdout, "uninstalled launchd agent")
	return err
}

func (r *Runner) runLaunchdStatus(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: hostward launchd status")
	}

	status, err := launchd.LoadStatus(r.paths, launchd.Label)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(r.stdout, "plist: %s (%s)\n", r.paths.LaunchAgentPath, presence(r.paths.LaunchAgentPath)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.stdout, "loaded: %t\n", status.Loaded); err != nil {
		return err
	}
	if status.Details != "" {
		if _, err := fmt.Fprintln(r.stdout, status.Details); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) parseLaunchdAgent(args []string) (launchd.Agent, error) {
	flags := flag.NewFlagSet("hostward launchd", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	binary := flags.String("binary", "", "")
	tick := flags.String("tick", "1s", "")
	if err := flags.Parse(args); err != nil {
		return launchd.Agent{}, fmt.Errorf("usage: hostward launchd <print-plist|install> [--binary <path>] [--tick <duration>]")
	}

	binaryPath, err := launchd.ExecutablePath(*binary)
	if err != nil {
		return launchd.Agent{}, err
	}
	tickDuration, err := config.ParseDuration(*tick)
	if err != nil {
		return launchd.Agent{}, fmt.Errorf("parse --tick: %w", err)
	}

	return launchd.DefaultAgent(r.paths, binaryPath, tickDuration), nil
}

func (r *Runner) runMonitorRun(id string) error {
	svc := service.New(r.paths)
	monitorSnapshot, err := svc.RunMonitor(id)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "%s: %s\n", monitorSnapshot.ID, monitorSnapshot.Status)
	if err != nil {
		return err
	}
	if monitorSnapshot.Summary != "" {
		_, err = fmt.Fprintln(r.stdout, monitorSnapshot.Summary)
	}
	return err
}

func (r *Runner) runMonitorShow(id string) error {
	svc := service.New(r.paths)
	bundle, store, snapshot, err := svc.Snapshot()
	if err != nil {
		return err
	}

	definition, ok := findDefinition(bundle.Monitors, id)
	if !ok {
		return fmt.Errorf("unknown monitor %q", id)
	}

	monitorSnapshot, ok := findMonitorSnapshot(snapshot.Monitors, id)
	if !ok {
		return fmt.Errorf("monitor %q missing from snapshot", id)
	}
	record := store.Monitors[id]

	if _, err := fmt.Fprintf(r.stdout, "id: %s\ntype: %s\nname: %s\nstatus: %s\n", definition.ID, definition.Type, definition.DisplayName(), monitorSnapshot.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.stdout, "every: %s\nsource: %s\n", definition.Every, definition.SourcePath); err != nil {
		return err
	}
	if monitorSnapshot.Summary != "" {
		if _, err := fmt.Fprintf(r.stdout, "summary: %s\n", monitorSnapshot.Summary); err != nil {
			return err
		}
	}
	if definition.Script != nil {
		if _, err := fmt.Fprintf(r.stdout, "command: %s\ntimeout: %s\n", strings.Join(definition.Script.Command, " "), definition.Script.Timeout); err != nil {
			return err
		}
	}
	if definition.Deadman != nil {
		if _, err := fmt.Fprintf(r.stdout, "grace: %s\n", definition.Deadman.Grace); err != nil {
			return err
		}
	}
	if record.LastCheckAt != nil {
		if _, err := fmt.Fprintf(r.stdout, "last_check_at: %s\n", record.LastCheckAt.Format(time.RFC3339)); err != nil {
			return err
		}
	}
	if record.LastPokeAt != nil {
		if _, err := fmt.Fprintf(r.stdout, "last_poke_at: %s\n", record.LastPokeAt.Format(time.RFC3339)); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) runMonitors(args []string) error {
	if len(args) != 1 || args[0] != "list" {
		return fmt.Errorf("usage: hostward monitors list")
	}

	svc := service.New(r.paths)
	bundle, _, snapshot, err := svc.Snapshot()
	if err != nil {
		return err
	}

	writer := tabwriter.NewWriter(r.stdout, 0, 0, 2, ' ', 0)
	for _, definition := range bundle.Monitors {
		monitorSnapshot, _ := findMonitorSnapshot(snapshot.Monitors, definition.ID)
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", definition.ID, definition.Type, monitorSnapshot.Status, definition.DisplayName()); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func (r *Runner) runPoke(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: hostward poke <name>")
	}

	svc := service.New(r.paths)
	monitorSnapshot, err := svc.PokeMonitor(args[0])
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.stdout, "%s: %s\n", monitorSnapshot.ID, monitorSnapshot.Status)
	return err
}

func (r *Runner) runScheduler(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hostward scheduler <once|run>")
	}

	switch args[0] {
	case "once":
		return r.runSchedulerOnce(args[1:])
	case "run":
		return r.runSchedulerRun(args[1:])
	default:
		return fmt.Errorf("usage: hostward scheduler <once|run>")
	}
}

func (r *Runner) runSchedulerOnce(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: hostward scheduler once")
	}

	runner := scheduler.Runner{
		Service: service.New(r.paths),
		Tick:    time.Second,
	}
	snapshot, err := runner.RunOnce(time.Now().UTC())
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(r.stdout, shell.Banner(snapshot, shell.BannerList))
	return err
}

func (r *Runner) runSchedulerRun(args []string) error {
	flags := flag.NewFlagSet("hostward scheduler run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tick := flags.String("tick", "1s", "")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("usage: hostward scheduler run [--tick <duration>]")
	}

	tickDuration, err := config.ParseDuration(*tick)
	if err != nil {
		return fmt.Errorf("parse --tick: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := scheduler.Runner{
		Service: service.New(r.paths),
		Tick:    tickDuration,
	}

	return runner.RunLoop(ctx)
}

func (r *Runner) runShellInit(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: hostward shell-init <zsh|bash>")
	}

	snippet, err := shell.Snippet(args[0])
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(r.stdout, snippet)
	return err
}

func (r *Runner) runStatus(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: hostward status")
	}

	svc := service.New(r.paths)
	bundle, _, snapshot, err := svc.Snapshot()
	if err != nil {
		return err
	}
	if err := state.WriteSnapshot(r.paths.CurrentStatePath, snapshot); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(r.stdout, shell.Banner(snapshot, shell.BannerList)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.stdout, "configured monitors: %d\n", len(bundle.Monitors)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		r.stdout,
		"states: ok=%d failing=%d unknown=%d disabled=%d\n",
		snapshot.StatusCounts.OK,
		snapshot.StatusCounts.Failing,
		snapshot.StatusCounts.Unknown,
		snapshot.StatusCounts.Disabled,
	); err != nil {
		return err
	}
	if len(snapshot.Failing) > 0 {
		if _, err := fmt.Fprintf(r.stdout, "failing: %s\n", strings.Join(snapshot.Failing, ", ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(r.stdout, "snapshot: %s (%s)\n", r.paths.CurrentStatePath, presence(r.paths.CurrentStatePath)); err != nil {
		return err
	}

	return nil
}

func presence(path string) string {
	_, err := os.Stat(path)
	if err == nil {
		return "present"
	}
	if os.IsNotExist(err) {
		parent := filepath.Dir(path)
		if info, parentErr := os.Stat(parent); parentErr == nil && info.IsDir() {
			return "missing"
		}
		return "missing parent"
	}

	return "unreadable"
}

const helpText = `hostward manages local monitoring state for a single Mac.

Usage:
  hostward status
  hostward banner [count|list]
  hostward env failing-count
  hostward build [--output <path>]
  hostward add script <name> --every <duration> -- <command...>
  hostward add deadman <name> --every <duration> --grace <duration>
  hostward add file-exists <name> --path <path> --every <duration>
  hostward add dir-exists <name> --path <path> --every <duration>
  hostward add file-freshness <name> --path <path> --max-age <duration> --every <duration>
  hostward add free-space <name> --path <path> --min-free-percent <n> --every <duration>
  hostward add dir-size <name> --path <path> --max-bytes <n> --every <duration>
  hostward add process-running <name> --match <pattern> --every <duration>
  hostward enable <name>
  hostward disable <name>
  hostward monitors list
  hostward monitor show <name>
  hostward monitor run <name>
  hostward notify test
  hostward poke <name>
  hostward scheduler once
  hostward scheduler run [--tick <duration>]
  hostward launchd print-plist [--binary <path>] [--tick <duration>]
  hostward launchd install [--binary <path>] [--tick <duration>]
  hostward launchd uninstall
  hostward launchd status
  hostward shell-init <zsh|bash>
  hostward doctor
  hostward version
`

func findDefinition(definitions []monitor.Definition, id string) (monitor.Definition, bool) {
	for _, definition := range definitions {
		if definition.ID == id {
			return definition, true
		}
	}

	return monitor.Definition{}, false
}

func findMonitorSnapshot(monitors []state.MonitorSnapshot, id string) (state.MonitorSnapshot, bool) {
	for _, monitorSnapshot := range monitors {
		if monitorSnapshot.ID == id {
			return monitorSnapshot, true
		}
	}

	return state.MonitorSnapshot{}, false
}

func expandHome(path, home string) (string, error) {
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, path[2:])
	}
	return filepath.Abs(path)
}
