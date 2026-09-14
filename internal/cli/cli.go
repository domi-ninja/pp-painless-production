package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"git.domi.ninja/domi-ninja/infra-meta-forgejo/internal/deploy"
)

func Main(name string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		args = []string{"deploy"}
	}

	command := args[0]
	flags := flag.NewFlagSet(name+" "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "deploy.yml", "deployment config, relative to the working directory")
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
	case "deploy":
		return runDeploy(*configPath, stdout, stderr)
	case "down":
		return runDown(*configPath, stdout, stderr)
	case "init":
		return runInit(*configPath, stdout, stderr)
	case "plan":
		return runPlan(*configPath, stdout, stderr)
	case "status":
		return runStatus(*configPath, stdout, stderr)
	case "rollback":
		return runRollback(*configPath, stdout, stderr)
	case "-h", "--help", "help":
		printHelp(name, stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printHelp(name, stderr)
		return 2
	}
}

func runDown(configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(wd, configPath, stdout, stderr).Down(); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runDeploy(configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(wd, configPath, stdout, stderr).Deploy(); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runInit(configPath string, stdout io.Writer, stderr io.Writer) int {
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

func runStatus(configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(wd, configPath, stdout, stderr).Status(); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runRollback(configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}
	if err := configuredDeployer(wd, configPath, stdout, stderr).Rollback(); err != nil {
		printError(stderr, err)
		return 1
	}
	return 0
}

func runPlan(configPath string, stdout io.Writer, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: read working directory: %v\n", err)
		return 1
	}

	plan, err := deploy.LoadPlan(wd, configPath)
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
	fmt.Fprintln(w, "  init      create a starter deploy.yml")
	fmt.Fprintln(w, "  plan      validate deploy.yml and print host/service placement")
	fmt.Fprintln(w, "  status    show local deployment state")
	fmt.Fprintln(w, "  rollback  restore previous code and DB state")
}

func configuredDeployer(root, configPath string, stdout, stderr io.Writer) deploy.Deployer {
	deployer := deploy.NewDeployer(root, stdout, stderr)
	deployer.ConfigPath = configPath
	return deployer
}
