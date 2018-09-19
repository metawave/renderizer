package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/imdario/mergo"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
)

var (
	version  = "2.0.0"
	commit   = "unknown"
	date     = "20060102T150405"
	selfn, _ = osext.Executable()
	selfz    = filepath.Base(selfn)
	semver   = version + "-" + date + "." + commit[:7]
	appver   = selfz + "/" + semver
)

//
type Settings struct {
	// Capitalization is a positional toggles. The following variable names are capitalized (title-case).
	Capitalize bool
	// Set the Missing Key template option. Defaults to "error".
	MissingKey string
	// Configuration yaml
	ConfigFiles cli.StringSlice
	Defaulted   bool
	Config      map[string]interface{}
	//
	Arguments []string
	//
	Templates []string
	// Add the environment map to the variables.
	Environment string
	//
	OutputExtension string
	//
	TimeFormat string
	//
	Stdin bool
	//
	Debugging bool
	//
	Verbose bool
}

//
var settings = Settings{
	Capitalize:  true,
	MissingKey:  "error",
	TimeFormat:  "20060102T150405",
	Environment: "env",
	Config:      map[string]interface{}{},
	ConfigFiles: []string{},
	Arguments:   []string{},
	Templates:   []string{},
}

//
func main() {
	app := cli.NewApp()
	app.Name = "renderizer"
	app.Usage = "renderizer [options] [--name=value...] template..."
	app.UsageText = "Template renderer"
	app.Version = appver
	app.EnableBashCompletion = true

	configs := cli.StringSlice{}

	app.Commands = []cli.Command{
		{
			Name:  "version",
			Usage: "Shows the app version",
			Action: func(ctx *cli.Context) error {
				fmt.Println(ctx.App.Version)
				return nil
			},
		},
	}

	app.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name:   "settings, S, s",
			Usage:  `load the settings from the provided YAMLs (default: ".renderizer.yaml")`,
			Value:  &configs,
			EnvVar: "RENDERIZER",
		},
		cli.StringFlag{
			Name:        "missing, M, m",
			Usage:       "the 'missingkey' template option (default|zero|error)",
			Value:       "error",
			EnvVar:      "RENDERIZER_MISSINGKEY",
			Destination: &settings.MissingKey,
		},
		cli.StringFlag{
			Name:   "environment, env, E, e",
			Usage:  "load the environment into the variable name instead of as 'env'",
			Value:  settings.Environment,
			EnvVar: "RENDERIZER_ENVIRONMENT",
		},
		cli.BoolFlag{
			Name:        "stdin, c",
			Usage:       "read from stdin",
			Destination: &settings.Stdin,
		},
		cli.BoolFlag{
			Name:        "debugging, debug, D",
			Usage:       "enable debugging server",
			Destination: &settings.Debugging,
		},
		cli.BoolFlag{
			Name:        "verbose, V",
			Usage:       "enable verbose output",
			Destination: &settings.Verbose,
		},
	}

	app.Before = func(ctx *cli.Context) error {

		fi, _ := os.Stdin.Stat()

		settings.Stdin = settings.Stdin || (fi.Mode()&os.ModeCharDevice) == 0

		settings.Arguments = append(settings.Arguments, ctx.Args()...)

		if len(settings.Templates) == 0 && !settings.Stdin {
			// Try default the template name
			folderName, err := os.Getwd()
			if err != nil {
				log.Println(err)
				folderName = "renderizer"
			} else {
				folderName = filepath.Base(folderName)
			}

			name := func() string {
				for _, base := range []string{folderName, "renderizer"} {
					for _, ext := range []string{".tmpl", ""} {
						for _, try := range []string{"yaml", "json", "html", "txt", "xml", ""} {
							name := fmt.Sprintf("%s.%s%s", base, try, ext)
							if _, err := os.Stat(name); err == nil {
								if settings.Verbose {
									log.Printf("using template: %+v", name)
								}
								return name
							}
						}
					}
				}
				return ""
			}()
			if name != "" {
				settings.Templates = append(settings.Templates, name)
			}
		}

		if len(settings.Templates) == 0 {
			return cli.NewExitError("missing template name", 1)
		}

		mainName := strings.Split(strings.TrimLeft(filepath.Base(settings.Templates[0]), "."), ".")[0]

		switch settings.MissingKey {
		case "zero", "error", "default", "invalid":
		default:
			fmt.Fprintf(os.Stderr, "ERROR: Resetting invalid missingkey: %+v", settings.MissingKey)
			settings.MissingKey = "error"
		}

		if len(configs) == 0 {
			settings.Defaulted = true
			settings.ConfigFiles = []string{"." + mainName + ".yaml"}
		} else {
			settings.ConfigFiles = configs
		}

		for _, config := range settings.ConfigFiles {
			in, err := ioutil.ReadFile(config)
			if err != nil {
				if !settings.Defaulted {
					return err
				}
			} else {
				loaded := map[string]interface{}{}
				err := yaml.Unmarshal(in, &loaded)
				if err != nil {
					return err
				}
				if settings.Debugging || settings.Verbose {
					log.Printf("using settings: %+v", settings.ConfigFiles)
				}
				loaded = retyper(loaded)
				if settings.Debugging {
					log.Printf("loaded: %s = %#v", config, loaded)
				} else if settings.Verbose {
					log.Printf("loaded: %s = %+v", config, loaded)
				}
				mergo.Merge(&settings.Config, loaded)
			}
		}

		if settings.Debugging {
			log.Printf("--settings:%#v", settings)
		} else if settings.Verbose {
			log.Printf("--settings:%+v", settings)
		}

		return nil
	}

	// Remove args that are not processed by urfave/cli
	args := []string{os.Args[0]}
	if len(os.Args) > 1 {
		next := false
		for _, arg := range os.Args[1:] {
			larg := strings.ToLower(arg)
			if next {
				args = append(args, arg)
				next = false
				continue
			}
			// TODO convert all '--name value' parameters to --name=value
			if strings.HasPrefix(larg, "--") {
				flag := larg
				parts := strings.SplitN(larg, "=", 2)
				if len(parts) == 2 {
					flag = parts[0]
				}
				switch flag[2:] {
				case "settings", "missing":
					// If the flag requires a parameter but it is not specified with an =, grab the next argument too.
					if !strings.Contains(larg, "=") {
						next = true
					}
					fallthrough
				case "debug", "verbose", "version", "stdin", "help":
					args = append(args, arg)
					continue
				}
			} else if strings.HasPrefix(larg, "-") {
				switch arg[1:] {
				case "C":
				case "S":
					if !strings.Contains(arg, "=") {
						next = true
					}
					fallthrough
				default:
					args = append(args, arg)
					continue
				}
			} else {
				settings.Templates = append(settings.Templates, arg)
				continue
			}

			settings.Arguments = append(settings.Arguments, arg)
		}
	}

	app.Action = renderizer
	app.Run(args)
}
