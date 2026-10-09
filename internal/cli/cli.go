package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/domi-ninja/pp-painless-production/internal/deploy"
)

func Main(name string, args []string, stdout, stderr io.Writer) int {
	return MainContext(context.Background(), name, args, stdout, stderr)
}
func MainContext(ctx context.Context, name string, args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 && args[0] == "help" {
		return MainContext(ctx, name, []string{args[1], "--help"}, stdout, stderr)
	}
	if len(args) == 0 {
		args = []string{"deploy"}
	}

	command := args[0]
	flags := flag.NewFlagSet(name+" "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { printCommandHelp(name, command, stderr); flags.PrintDefaults() }
	configPath := flags.String("config", "deploy.yml", "deployment config, relative to the working directory")
	var releasePorts bool
	if command == "down" {
		flags.BoolVar(&releasePorts, "release-ports", false, "retire owned port allocations after stopping containers")
	}
	var dryRun bool
	if command == "cleanup" {
		flags.BoolVar(&dryRun, "dry-run", false, "preview cleanup without deleting artifacts")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected arguments:", flags.Args())
		return 2
	}
	switch command {
	case "version", "--version":
		printVersion(name, stdout)
		return 0
	case "cleanup":
		wd, err := os.Getwd()
		if err == nil {
			err = configuredDeployer(ctx, wd, *configPath, stdout, stderr).Cleanup(dryRun)
		}
		if err != nil {
			printError(stderr, err)
			return 1
		}
		return 0
	case "deploy":
		return runDeploy(ctx, *configPath, stdout, stderr)
	case "down":
		return runDown(ctx, *configPath, releasePorts, stdout, stderr)
	case "init":
		return runInit(ctx, *configPath, stdout, stderr)
	case "plan":
		return runPlan(ctx, *configPath, stdout, stderr)
	case "status":
		return runStatus(ctx, *configPath, stdout, stderr)
	case "rollback":
		return runRollback(ctx, *configPath, stdout, stderr)
	case "-h", "--help", "help":
		printHelp(name, stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printHelp(name, stderr)
		return 2
	}
}

func runDown(ctx context.Context, configPath string, releasePorts bool, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(ctx, wd, configPath, stdout, stderr).DownWithPortRelease(releasePorts); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runDeploy(ctx context.Context, configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(ctx, wd, configPath, stdout, stderr).Deploy(); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runInit(ctx context.Context, configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}

	result, err := deploy.InitConfig(wd, configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "created %s\n", result.Path)
	fmt.Fprintf(stdout, "project: %s\n", result.ProjectName)
	return 0
}

func runStatus(ctx context.Context, configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(ctx, wd, configPath, stdout, stderr).Status(); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runRollback(ctx context.Context, configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(ctx, wd, configPath, stdout, stderr).Rollback(); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runPlan(ctx context.Context, configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}

	plan, err := deploy.LoadPlanContext(ctx, wd, configPath)
	if err != nil {
		var validationErr deploy.ValidationError
		if errors.As(err, &validationErr) {
			fmt.Fprintf(stderr, "invalid deploy config:\n%s", validationErr.Error())
			return 1
		}

		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	deploy.PrintPlan(stdout, plan)
	return 0
}

func printError(stderr io.Writer, err error) {
	var validationErr deploy.ValidationError
	if errors.As(err, &validationErr) {
		fmt.Fprintf(stderr, "invalid deploy config:\n%s", validationErr.Error())
		return
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
}

func printHelp(name string, w io.Writer) {
	fmt.Fprintf(w, "usage: %s <command> [--config deploy.yml]\n", name)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  deploy    build, transfer, and apply the release")
	fmt.Fprintln(w, "  down      remove remote containers for the selected project/environment")
	fmt.Fprintln(w, "  version   show compiler and source build identity")
	fmt.Fprintln(w, "  init      create a starter deploy.yml")
	fmt.Fprintln(w, "  plan      validate deploy.yml and print host/service placement")
	fmt.Fprintln(w, "  status    show local attempts and observed container state")
	fmt.Fprintln(w, "  rollback  recover the saved previous release; declared database recovery only")
	fmt.Fprintln(w, "  cleanup   remove expired releases and owned images (--dry-run to preview)")
}

func configuredDeployer(ctx context.Context, root, configPath string, stdout, stderr io.Writer) deploy.Deployer {
	deployer := deploy.NewDeployer(root, stdout, stderr)
	deployer.ConfigPath = configPath
	deployer.Runner.Context = ctx
	return deployer
}

func printVersion(name string, w io.Writer) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Fprintf(w, "%s build unknown\n", name)
		return
	}
	revision, dirty := "unknown", "unknown"
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value
		}
	}
	fmt.Fprintf(w, "%s %s\ncompiler: %s\nrevision: %s\nsource_modified: %s\n", name, info.Main.Version, info.GoVersion, revision, dirty)
}
func printCommandHelp(name, command string, w io.Writer) {
	fmt.Fprintf(w, "usage: %s %s [options]\n", name, command)
	switch command {
	case "deploy":
		fmt.Fprintln(w, "Build the working tree, apply services and routes, then run checks.")
	case "rollback":
		fmt.Fprintln(w, "Use saved release inputs. Database restore plans and application hooks need explicit recovery.")
	case "down":
		fmt.Fprintln(w, "Remove owned containers and routes. Keep volumes and, unless requested, ports.")
	case "status":
		fmt.Fprintln(w, "Show local records and inspect recorded hosts over SSH.")
	case "plan":
		fmt.Fprintln(w, "Validate inputs and preview images, placement, mounts, routes and recovery without changing hosts.")
	case "init":
		fmt.Fprintln(w, "Create a config with a localhost health endpoint and ignore deployment state and env files.")
	case "cleanup":
		fmt.Fprintln(w, "Remove expired owned artifacts. Preserve current and previous releases and data volumes.")
	}
}
