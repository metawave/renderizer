package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/gomatic/funcmap"
	"github.com/imdario/mergo"
	"github.com/urfave/cli"
)

//
func renderizer(_ *cli.Context) error {

	globalContext := map[string]interface{}{}
	args := []string{}

	// Iterate the remaining arguments for variable overrides and file names.

	for a, arg := range settings.Arguments {
		if len(arg) == 0 {
			continue
		} else if arg[0] != '-' {
			args = append(args, arg)
			continue
		}

		switch arg[1:][0] {
		case 'c', 'C':
			settings.Capitalize = !settings.Capitalize
			if settings.Verbose {
				log.Printf("-capitalize:%v", settings.Capitalize)
			}
			continue
		}

		currentContext := map[string]interface{}{}

		nameValuePair := strings.SplitN(strings.TrimLeft(arg, "-"), "=", 2)
		currentName := nameValuePair[0]

		// Iterate dotted notation and construct a map from it.
		var (
			local = currentContext       // Iterative map-reference into globalContext
			leaf  map[string]interface{} // Keep a reference to the leaf node
			last  string                 // Keep the leaf node's map-key
		)
		for _, name := range strings.Split(currentName, ".") {
			if settings.Capitalize {
				name = fmt.Sprintf("%s%s", strings.ToTitle(name[:1]), strings.ToLower(name[1:]))
			}

			local[name] = map[string]interface{}{}
			leaf, last, local = local, name, local[name].(map[string]interface{})
		}

		var currentValue interface{}
		if len(nameValuePair) == 1 { // i.e. a boolean
			leaf[last] = []interface{}{true}
		} else {
			leaf[last] = []interface{}{typer(nameValuePair[1])}
		}

		if settings.Debugging {
			log.Printf("index:%d name:%s value:%+v", a, currentName, currentValue)
		}

		// log.Printf("currentContext: %[1]T %#[1]v", currentContext)
		mergo.Merge(&globalContext, currentContext, mergo.WithAppendSlice)
		// log.Printf("globalContext: %[1]T %#[1]v", globalContext)
	}

	retyper(globalContext)

	// Dump the settings

	if settings.Debugging {
		log.Printf("globalContext: %#v", globalContext)
	} else if settings.Verbose {
		log.Printf("globalContext: %+v", globalContext)
	}

	// If there's no files, read from stdin.
	files := args
	if len(args) == 0 {
		stat, _ := os.Stdin.Stat()
		isTTY := (stat.Mode() & os.ModeCharDevice) != 0
		if isTTY {
			log.Println("source: stdin")
		}
		files = []string{""}
	}

	// Copy any loaded keys into the globalContext unless they already exist, i.e. they were provided on the command line.
	for n, v := range settings.Config {
		if _, exists := globalContext[n]; !exists {
			switch x := v.(type) {
			case int8:
				globalContext[n] = int64(x)
			case int16:
				globalContext[n] = int64(x)
			case int32:
				globalContext[n] = int64(x)
			case int:
				globalContext[n] = int64(x)
			default:
				globalContext[n] = v
			}
		}
	}

	if settings.Environment != "" || len(args) == 0 {
		v := make(map[string]string)
		for _, item := range os.Environ() {
			splits := strings.Split(item, "=")
			v[splits[0]] = strings.Join(splits[1:], "=")
		}
		globalContext[settings.Environment] = v
	}

	// Dump the settings

	if settings.Debugging {
		log.Printf("globalContext: %#v", globalContext)
	} else if settings.Verbose {
		log.Printf("globalContext: %+v", globalContext)
	}

	// Execute each template

	status := 0
	for _, file := range files {
		status |= func() (status int) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("PANIC: %+v", r)
					status = 15
				}
			}()

			var err error
			var data []byte
			var r io.ReadCloser
			if file == "" {
				r = os.Stdin
			} else {
				r, err = os.Open(file)
				if err != nil {
					log.Println(err)
					return 1
				}
				defer r.Close()
			}
			f, err := ioutil.ReadAll(r)
			if err != nil {
				log.Println(err)
				return 2
			}
			data = f

			tmpl, err := template.New(file).
				Option(fmt.Sprintf("missingkey=%s", settings.MissingKey)).
				Funcs(funcmap.Map).
				Parse(string(data))
			if err != nil {
				log.Print(err)
				return 4
			}

			var b bytes.Buffer
			err = tmpl.Execute(&b, globalContext)
			if err != nil {
				log.Print(err)
				return 8
			}

			data = b.Bytes()
			fmt.Println(string(data))

			return 0
		}()
	}

	os.Exit(status)
	return nil
}