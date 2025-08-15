package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
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
	klog.InitFlags(nil)
	flag.Parse()

	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// TODO: Find configs in a well-known path
	configPath := "config/default.yaml"
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
	b, err := ioutil.ReadFile(path)
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
	now := time.Now()
	date := now.Format("20060102")
	s := strings.Replace(pattern, "YYYYMMDD", date, -1)

	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("error getting current user: %w", err)
	}
	username := u.Username
	// Sanitize username - gcloud projects must start with a letter and contain only letters, numbers, and hyphens
	sanitizedUsername := ""
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sanitizedUsername += string(r)
		}
	}
	if len(sanitizedUsername) == 0 {
		return "", fmt.Errorf("could not build sanitized username from %q", username)
	}

	s = strings.Replace(s, "${USER}", sanitizedUsername, -1)
	s = strings.ToLower(s)
	return s, nil
}
