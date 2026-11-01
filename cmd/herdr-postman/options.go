package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type startupOptions struct {
	root, bin                          string
	source, workspace, cloudCollection string
	environments                       []string
}

func parseStartupOptions(args []string, output io.Writer) (startupOptions, error) {
	var opts startupOptions
	flags := flag.NewFlagSet("herdr-postman", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&opts.source, "source", "local", "local, cloud, or choose")
	flags.StringVar(&opts.workspace, "workspace", "", "Postman Cloud workspace ID")
	flags.StringVar(&opts.cloudCollection, "cloud-collection", "", "Postman Cloud collection ID")
	flags.StringVar(&opts.root, "collection", "", "Postman Native Git collection directory")
	flags.StringVar(&opts.bin, "postman-bin", "postman", "Postman CLI executable")
	flags.Func("environment", "Postman Native environment file; repeat to layer files in order", func(file string) error {
		if strings.TrimSpace(file) == "" {
			return fmt.Errorf("environment file must not be empty")
		}
		opts.environments = append(opts.environments, file)
		return nil
	})
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments; use --collection and --environment")
	}
	if opts.source != "local" && opts.source != "cloud" && opts.source != "choose" {
		return opts, fmt.Errorf("source must be local, cloud, or choose")
	}
	if opts.source == "cloud" {
		if opts.workspace == "" || opts.cloudCollection == "" || opts.root != "" || len(opts.environments) != 0 {
			return opts, fmt.Errorf("cloud requires --workspace and --cloud-collection without local collection or environment flags")
		}
	} else if opts.workspace != "" || opts.cloudCollection != "" {
		return opts, fmt.Errorf("cloud IDs require --source cloud")
	}
	if opts.source == "local" && opts.root == "" {
		return opts, fmt.Errorf("use --collection <native-collection-directory>")
	}
	return opts, nil
}
