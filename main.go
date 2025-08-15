package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"google.golang.org/api/option"

	serviceusage "cloud.google.com/go/serviceusage/apiv1"
	"cloud.google.com/go/serviceusage/apiv1/serviceusagepb"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/googleapi"
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

type ProjectManager struct {
	config *Config

	crmService *cloudresourcemanager.Service
}

func NewProjectManager(config *Config) *ProjectManager {
	return &ProjectManager{config: config}
}

func (p *ProjectManager) getCloudResourceManagerClient(ctx context.Context) (*cloudresourcemanager.Service, error) {
	if p.crmService != nil {
		return p.crmService, nil
	}
	crmService, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating cloudresourcemanager client: %w", err)
	}
	p.crmService = crmService
	return crmService, nil
}

func (p *ProjectManager) EnsureProjectExists(ctx context.Context, projectName string) error {
	log := klog.FromContext(ctx)

	project, err := p.getProject(ctx, projectName)
	if err != nil {
		return err
	}
	if project == nil {
		log.Info("project does not exist, creating", "name", projectName)
		if err := p.createProject(ctx, projectName); err != nil {
			return err
		}
	} else {
		log.Info("project already exists", "name", projectName)
	}

	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	klog.InitFlags(nil)

	configPath := ""
	flag.StringVar(&configPath, "config", configPath, "Path to the configuration file")
	flag.Parse()

	logger := klog.NewKlogr()
	ctx = klog.NewContext(ctx, logger)

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

	log := klog.FromContext(ctx)
	log.Info("Project name", "name", projectName)

	projectManager := NewProjectManager(config)
	if err := projectManager.EnsureProjectExists(ctx, projectName); err != nil {
		return err
	}

	// Ensure cloudbilling.googleapis.com is enabled first so we can set up billing
	if err := projectManager.EnableProjectServices(ctx, projectName, []string{"cloudbilling.googleapis.com"}); err != nil {
		return err
	}

	if err := projectManager.LinkProjectToBillingAccount(ctx, projectName); err != nil {
		return err
	}

	if err := projectManager.EnableProjectServices(ctx, projectName, config.Services); err != nil {
		return err
	}

	if err := projectManager.RunSetupCommands(ctx, projectName); err != nil {
		return err
	}

	return nil
}

func (p *ProjectManager) createProject(ctx context.Context, projectName string) error {
	crmService, err := p.getCloudResourceManagerClient(ctx)
	if err != nil {
		return err
	}
	project := &cloudresourcemanager.Project{
		ProjectId:   projectName,
		DisplayName: projectName,
		Parent:      p.config.Parent,
	}
	op, err := crmService.Projects.Create(project).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("error creating project: %w", err)
	}

	for !op.Done {
		time.Sleep(2 * time.Second)
		op, err = crmService.Operations.Get(op.Name).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("error getting operation status: %w", err)
		}
	}

	if op.Error != nil {
		return fmt.Errorf("error from project creation operation: %v", op.Error)
	}
	log := klog.FromContext(ctx)
	log.Info("project created", "name", projectName)
	return nil
}

// getProject gets the project, returning nil if it does not exist
func (p *ProjectManager) getProject(ctx context.Context, projectName string) (*cloudresourcemanager.Project, error) {
	crmService, err := p.getCloudResourceManagerClient(ctx)
	if err != nil {
		return nil, err
	}
	// TODO: Search instead of get
	resp, err := crmService.Projects.Get("projects/" + projectName).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) || isPermissionDenied(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting project: %w", err)
	}
	return resp, nil
}

func (p *ProjectManager) LinkProjectToBillingAccount(ctx context.Context, projectName string) error {
	log := klog.FromContext(ctx)

	billingService, err := cloudbilling.NewService(ctx, option.WithQuotaProject(projectName))
	if err != nil {
		return fmt.Errorf("error creating cloudbilling client: %w", err)
	}

	// Check if already linked
	currentBillingInfo, err := billingService.Projects.GetBillingInfo("projects/" + projectName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("error getting current billing info for project %q: %w", projectName, err)
	}

	if currentBillingInfo.BillingAccountName == p.config.BillingAccount && currentBillingInfo.BillingEnabled {
		log.Info("project already linked to billing account", "project", projectName, "billingAccount", p.config.BillingAccount)
		return nil
	}

	log.Info("linking project to billing account", "project", projectName, "billingAccount", p.config.BillingAccount)

	projectBillingInfo := &cloudbilling.ProjectBillingInfo{
		BillingAccountName: p.config.BillingAccount,
		BillingEnabled:     true,
	}

	_, err = billingService.Projects.UpdateBillingInfo("projects/"+projectName, projectBillingInfo).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("error linking project %q to billing account %q: %w", projectName, p.config.BillingAccount, err)
	}

	log.Info("project linked to billing account", "project", projectName, "billingAccount", p.config.BillingAccount)
	return nil
}

func (p *ProjectManager) EnableProjectServices(ctx context.Context, projectName string, servicesToEnable []string) error {
	log := klog.FromContext(ctx)

	suClient, err := serviceusage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("error creating serviceusage client: %w", err)
	}
	defer suClient.Close()

	if len(servicesToEnable) == 0 {
		log.Info("no services to enable", "project", projectName)
		return nil
	}

	log.Info("enabling services", "services", servicesToEnable, "project", projectName)
	req := &serviceusagepb.BatchEnableServicesRequest{
		Parent:     fmt.Sprintf("projects/%s", projectName),
		ServiceIds: servicesToEnable,
	}

	op, err := suClient.BatchEnableServices(ctx, req)
	if err != nil {
		return fmt.Errorf("error starting batch enable services operation: %w", err)
	}

	_, err = op.Wait(ctx)
	if err != nil {
		return fmt.Errorf("error waiting for batch enable services operation: %w", err)
	}

	log.Info("services enabled", "services", servicesToEnable, "project", projectName)
	return nil
}

func (p *ProjectManager) RunSetupCommands(ctx context.Context, projectName string) error {
	log := klog.FromContext(ctx)

	if len(p.config.SetupCommands) == 0 {
		log.Info("no setup commands to run", "project", projectName)
		return nil
	}

	log.Info("running setup commands", "project", projectName)
	for _, command := range p.config.SetupCommands {
		expandedCommand := strings.ReplaceAll(command, "${PROJECT_ID}", projectName)
		log.Info("running command", "command", expandedCommand, "project", projectName)
		cmd := exec.Command("bash", "-c", expandedCommand)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error running setup command %q: %w", expandedCommand, err)
		}
	}
	log.Info("setup commands completed", "project", projectName)
	return nil
}

func isNotFound(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
		return true
	}
	return false
}

func isPermissionDenied(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusForbidden {
		return true
	}
	return false
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
