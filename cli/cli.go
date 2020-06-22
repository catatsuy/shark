package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"go.uber.org/multierr"
)

const (
	Version = "v0.0.1"

	ExitCodeOK   = 0
	ExitCodeFail = 1
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

type CLI struct {
	outStream, errStream io.Writer
}

// Config represents mackerel-agent's configuration file.
type Config struct {
	// This Plugin field is used to decode the toml file. After reading the
	// configuration from file, this field is set to nil.
	// Please consider using MetricPlugins and CheckPlugins.
	Plugin map[string]map[string]*PluginConfig
}

// PluginConfig represents a plugin configuration.
type PluginConfig struct {
	CommandConfig
}

type CommandConfig struct {
	Raw interface{} `toml:"command"`
}

func NewCLI(outStream, errStream io.Writer) *CLI {
	log.SetOutput(errStream)
	return &CLI{outStream: outStream, errStream: errStream}
}

func (c *CLI) Run(args []string) int {
	var (
		version    bool
		configPath string
	)

	flags := flag.NewFlagSet("shark", flag.ContinueOnError)
	flags.SetOutput(c.errStream)

	flags.StringVar(&configPath, "config-path", "", "config path")

	flags.BoolVar(&version, "version", false, "Print version information and quit")

	err := flags.Parse(args[1:])
	if err != nil {
		return ExitCodeFail
	}

	if version {
		fmt.Fprintf(c.errStream, "shark version %s; %s\n", Version, runtime.Version())
		return ExitCodeOK
	}

	err = c.run(configPath)
	if err != nil {
		fmt.Fprintf(c.errStream, "%+v\n", err)
		return ExitCodeFail
	}

	return ExitCodeOK
}

func (c *CLI) run(configPath string) error {
	if configPath == "" {
		return fmt.Errorf("must provide config path")
	}

	matches, err := filepath.Glob(configPath)
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return fmt.Errorf("not found files")
	}

	var conf Config
	var errs error

	for _, file := range matches {
		_, err := toml.DecodeFile(file, &conf)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()

		for _, v := range conf.Plugin {
			for _, com := range v {
				switch t := com.Raw.(type) {
				case string:
					err := c.commandRun(ctx, "sh", "-c", t)
					if err != nil {
						errs = multierr.Append(errs, err)
					}
				case []interface{}:
					if len(t) == 0 {
						return fmt.Errorf("failed to parse")
					}
					args := make([]string, 0, len(t))
					for _, vv := range t {
						str, ok := vv.(string)
						if !ok {
							return fmt.Errorf("not string")
						}
						args = append(args, str)
					}
					err := c.commandRun(ctx, args[0], args[1:]...)
					if err != nil {
						errs = multierr.Append(errs, err)
					}
				case []string:
					if len(t) == 0 {
						return fmt.Errorf("failed to parse")
					}
					err := c.commandRun(ctx, t[0], t[1:]...)
					if err != nil {
						errs = multierr.Append(errs, err)
					}
				case nil:
				// nothing
				default:
					return fmt.Errorf("failed to parse")
				}
			}
		}
	}

	return errs
}

func (c *CLI) commandRun(ctx context.Context, name string, arg ...string) error {
	cmd := exec.CommandContext(ctx, name, arg...)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(c.outStream, stdout.String())
		fmt.Fprintf(c.errStream, stderr.String())
		return fmt.Errorf("exec: %s %s; err: %+v", name, strings.Join(arg, " "), err)
	}

	return nil
}
