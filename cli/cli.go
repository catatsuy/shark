package cli

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
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
		log.Fatal(err)
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

	for _, file := range matches {
		_, err := toml.DecodeFile(file, &conf)
		if err != nil {
			return err
		}

		for _, v := range conf.Plugin {
			for _, com := range v {
				switch t := com.Raw.(type) {
				case string:
					err := c.commandRun("sh", "-c", t)
					if err != nil {
						fmt.Fprintf(c.errStream, "%+v\n", err)
					}
				case []interface{}:
					if len(t) <= 1 {
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
					err := c.commandRun(args[0], args[1:]...)
					if err != nil {
						fmt.Printf("%+v\n", err)
					}
				case []string:
					if len(t) <= 1 {
						return fmt.Errorf("failed to parse")
					}
					err := c.commandRun(t[0], t[1:]...)
					if err != nil {
						fmt.Printf("%+v\n", err)
					}
				case nil:
				// nothing
				default:
					return fmt.Errorf("failed to parse")
				}
			}
		}
	}

	return nil
}

func (c *CLI) commandRun(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = c.outStream
	cmd.Stderr = c.errStream
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("exec: %s %s; err: %+v", name, strings.Join(arg, " "), err)
	}

	return nil
}
