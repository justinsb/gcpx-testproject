package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

type Config struct {
	NamePattern    string   `yaml:"namePattern"`
	Parent         string   `yaml:"parent"`
	BillingAccount string   `yaml:"billingAccount"`
	Services       []string `yaml:"services"`
	SetupCommands  []string `yaml:"setupCommands"`
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	configPath := ""
	flag.StringVar(&configPath, "config", configPath, "Path to the configuration file")
	klog.InitFlags(nil)
	flag.Parse()

	if configPath == "" {
		return fmt.Errorf("config file path must be specified with -config flag")
	}
	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("error loading config %q: %w", configPath, err)
	}

	projectName, err := expandProjectName(config.NamePattern)
	if err != nil {
		return fmt.Errorf("error expanding project name: %w", err)
	}

	klog.Infof("Project name: %s", projectName)
	// TODO: Create project
	// TODO: Enable services
	// TODO: Run setup commands
	return nil
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %q: %w", path, err)
	}
	c := &Config{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("error unmarshaling yaml from %q: %w", path, err)
	}
	return c, nil
}

func expandProjectName(pattern string) (string, error) {
	var out strings.Builder
	in := pattern
	for {
		i := strings.Index(in, "${")
		if i == -1 {
			out.WriteString(in)
			break
		}
		out.WriteString(in[:i])
		in = in[i+2:]

		j := strings.Index(in, "}")
		if j == -1 {
			return "", fmt.Errorf("unclosed substitution in pattern %q", pattern)
		}
		expr := in[:j]
		in = in[j+1:]

		var val string
		switch expr {
		case "today":
			val = time.Now().Format("20060102")
		default:
			if strings.HasPrefix(expr, "env.") {
				varName := strings.TrimPrefix(expr, "env.")
				val = os.Getenv(varName)
			} else {
				return "", fmt.Errorf("unrecognized expression %q in pattern %q", expr, pattern)
			}
		}
		out.WriteString(val)
	}

	s := out.String()
	if s == "" {
		return "", fmt.Errorf("project name pattern %q expanded to empty string", pattern)
	}

	// GCP project IDs must be lowercase.
	s = strings.ToLower(s)
	// Note: We are not fully sanitizing the project ID here.
	// The user is responsible for ensuring environment variables result in a valid GCP project ID.
	// A valid ID contains lowercase letters, numbers, and hyphens.
	return s, nil
}
