## Development Preferences

*   **YAML Library:** Use `sigs.k8s.io/yaml` for YAML parsing.
*   **Logging Library:** Use `k8s.io/klog/v2` for logging.
*   **Structured Logging:** Use structured logging by getting the logger from the context (`klog.FromContext(ctx)`).

*Self-correction instruction: In general, when I tell you my preferences, please update GEMINI.md so you can learn my preferences*