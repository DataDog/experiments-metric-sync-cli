# Contributing

Thanks for your interest in contributing to Datadog Experiments Metric Sync CLI.

## Getting Started

1. Fork the repository or create a branch.
2. Make focused changes with tests where practical.
3. Run the local checks:

```sh
go test ./...
go run github.com/goreleaser/goreleaser/v2@v2.16.0 check
```

4. Open a pull request with a clear description of the change and testing.

## Reporting Issues

Use GitHub issues for bug reports and feature requests. Include enough detail for maintainers to reproduce the issue, including the CLI version, operating system, command, expected behavior, actual behavior, and relevant sanitized output.

## Security Issues

Do not report security vulnerabilities through public GitHub issues. Follow Datadog's vulnerability disclosure process at https://www.datadoghq.com/security/.

## License

By contributing, you agree that your contributions will be licensed under this repository's Apache License, Version 2.0.
