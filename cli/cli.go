package cli

import (
	"flag"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/catatsuy/shark/command"
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
		timeout    time.Duration
	)

	flags := flag.NewFlagSet("shark", flag.ContinueOnError)
	flags.SetOutput(c.errStream)

	flags.StringVar(&configPath, "config-path", "", "config path")
	flags.DurationVar(&timeout, "timeout", 1*time.Minute, "timeout (sig kill)")

	flags.BoolVar(&version, "version", false, "Print version information and quit")

	err := flags.Parse(args[1:])
	if err != nil {
		return ExitCodeFail
	}

	if version {
		fmt.Fprintf(c.errStream, "shark version %s; %s\n", Version, runtime.Version())
		return ExitCodeOK
	}

	err = c.run(configPath, timeout)
	if err != nil {
		fmt.Fprintf(c.errStream, "%+v\n", err)
		return ExitCodeFail
	}

	return ExitCodeOK
}

func (c *CLI) run(configPath string, timeout time.Duration) error {
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

		for _, v := range conf.Plugin {
			for _, com := range v {
				switch t := com.Raw.(type) {
				case string:
					cmd := command.NewCommand(c.outStream, c.errStream, t, timeout)
					err := cmd.Exec()
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
					cmd := command.NewCommands(c.outStream, c.errStream, args, timeout)
					err := cmd.Exec()
					if err != nil {
						errs = multierr.Append(errs, err)
					}
				case []string:
					if len(t) == 0 {
						return fmt.Errorf("failed to parse")
					}
					cmd := command.NewCommands(c.outStream, c.errStream, t, timeout)
					err := cmd.Exec()
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
