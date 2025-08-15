# Gemini - GCP Test Project Manager

This tool manages GCP test projects, creating a new project for each day with a configurable prefix.

## Features

*   **Daily Projects:** Automatically creates a new GCP project for each day, named with a configurable prefix, the user's name, and the date (e.g., `abc-${USER}-YYYYMMDD`).
*   **Pre-creation:** Supports pre-creating projects overnight so they are ready in the morning.
*   **Configuration:** Uses a YAML configuration file for each prefix, specifying:
    *   Project name pattern
    *   Parent folder
    *   Billing account
    *   Services to enable
    *   A list of bash commands to run for setup.

## Configuration

Configuration is managed through YAML files. Here is an example configuration:

```yaml
# config/abc.yaml
namePattern: "abc-${USER}-YYYYMMDD"
parent: "folders/1234567890"
billingAccount: "billingAccounts/012345-67890A-BCDEF0"
services:
  - "compute.googleapis.com"
  - "container.googleapis.com"
setupCommands:
  - "gcloud container clusters create-auto my-cluster --project=${PROJECT_ID} --region=us-central1"
  - "gcloud compute instances create my-instance --project=${PROJECT_ID} --zone=us-central1-a"
```

## Development Preferences

*   **YAML Library:** Use `sigs.k8s.io/yaml` for YAML parsing.
*   **Logging Library:** Use `k8s.io/klog/v2` for logging.
*   **Structured Logging:** Use structured logging by getting the logger from the context (`klog.FromContext(ctx)`).

*Self-correction instruction: In general, when I tell you my preferences, please update GEMINI.md so you can learn my preferences*
