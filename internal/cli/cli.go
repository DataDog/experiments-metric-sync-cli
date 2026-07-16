// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/DataDog/experiments-metric-sync-cli/internal/api"
	"github.com/DataDog/experiments-metric-sync-cli/internal/config"
	"github.com/DataDog/experiments-metric-sync-cli/internal/idempotency"
	"github.com/DataDog/experiments-metric-sync-cli/internal/logfile"
	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
	"github.com/DataDog/experiments-metric-sync-cli/internal/output"
	"github.com/DataDog/experiments-metric-sync-cli/internal/payload"
	"github.com/DataDog/experiments-metric-sync-cli/internal/poll"
	"github.com/DataDog/experiments-metric-sync-cli/internal/validate"
	"github.com/DataDog/experiments-metric-sync-cli/internal/yamlutil"
	"github.com/spf13/cobra"
)

type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

type app struct {
	stdout  io.Writer
	stderr  io.Writer
	version VersionInfo
	logger  *logfile.Logger
	opts    commandOptions
}

type commandOptions struct {
	site           string
	timeout        time.Duration
	logFile        string
	pollInterval   time.Duration
	idempotencyKey string
	noPoll         bool
	files          []string
}

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e exitError) Unwrap() error {
	return e.err
}

func Run(args []string, version VersionInfo) int {
	return runWithIO(args, version, os.Stdout, os.Stderr)
}

func runWithIO(args []string, version VersionInfo, stdout io.Writer, stderr io.Writer) int {
	cobra.EnableCommandSorting = false

	start := time.Now()
	exitCode := 0
	logPath := logFilePathFromArgs(args)
	logger, err := logfile.New(logPath, args, logfile.VersionInfo{
		Version: version.Version,
		Commit:  version.Commit,
		Date:    version.Date,
	})
	if err != nil {
		_ = output.PrintLogWarning(stderr, logPath, err)
	} else {
		defer func() {
			logger.Complete(exitCode, time.Since(start))
			_ = logger.Close()
		}()
	}

	a := &app{
		stdout:  stdout,
		stderr:  stderr,
		version: version,
		logger:  logger,
		opts: commandOptions{
			timeout:      10 * time.Minute,
			logFile:      logPath,
			pollInterval: 2 * time.Second,
		},
	}

	cmd := a.newRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		exitCode = a.handleCommandError(err)
		return exitCode
	}
	return exitCode
}

func (a *app) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "metric-sync",
		Short:         "Prepare and run Datadog Metric Sync operations",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().StringVar(&a.opts.site, "site", "", "Datadog site or base URL, such as datadoghq.com")
	cmd.PersistentFlags().DurationVar(&a.opts.timeout, "timeout", a.opts.timeout, "overall command timeout")
	cmd.PersistentFlags().StringVar(&a.opts.logFile, "log-file", a.opts.logFile, "debug log file path")

	cmd.AddCommand(
		a.newValidateCommand(),
		a.newSubmitCommand("plan", "Submit an async metric sync plan operation"),
		a.newSubmitCommand("execute", "Validate and apply metric sync definitions"),
		a.newStatusCommand(),
		a.newResultCommand(),
		a.newVersionCommand(),
	)
	return cmd
}

func (a *app) newValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [path...]",
		Short: "Validate metric sync YAML files without calling Datadog",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runValidate(args)
		},
	}
	cmd.Flags().StringArrayVar(&a.opts.files, "file", nil, "YAML file or directory to process; may be repeated")
	return cmd
}

func (a *app) newSubmitCommand(operation string, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   operation + " [path...]",
		Short: short,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSubmit(cmd.Context(), operation, args)
		},
	}
	cmd.Flags().StringArrayVar(&a.opts.files, "file", nil, "YAML file or directory to process; may be repeated")
	cmd.Flags().DurationVar(&a.opts.pollInterval, "poll-interval", a.opts.pollInterval, "poll interval while waiting for terminal status")
	cmd.Flags().StringVar(&a.opts.idempotencyKey, "idempotency-key", "", "override derived idempotency key for replay, resume, or debugging")
	cmd.Flags().BoolVar(&a.opts.noPoll, "no-poll", false, "submit the operation and print its ID without polling")
	return cmd
}

func (a *app) newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status <metric_sync_id>",
		Short: "Fetch metric sync operation status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runStatus(cmd.Context(), args[0])
		},
	}
}

func (a *app) newResultCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "result <metric_sync_id>",
		Short: "Fetch terminal metric sync result output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runResult(cmd.Context(), args[0])
		},
	}
}

func (a *app) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return output.PrintVersion(a.stdout, a.version.Version, a.version.Commit, a.version.Date)
		},
	}
}

func (a *app) runValidate(positional []string) error {
	files, fileConfigs, issues, err := loadAndValidate(positional, a.opts.files)
	if err != nil {
		return a.fail(1, err)
	}
	a.info("discovered files", slog.Any("files", files))
	_ = fileConfigs
	if err := output.PrintDiscovered(a.stdout, files); err != nil {
		return a.fail(1, err)
	}
	for _, issue := range issues {
		a.info("validation issue", slog.String("issue", issue.Error()))
	}
	if err := output.PrintValidation(a.stdout, issues); err != nil {
		return a.fail(1, err)
	}
	if len(issues) > 0 {
		return exitError{code: 1}
	}
	return nil
}

func (a *app) runSubmit(ctx context.Context, operation string, positional []string) error {
	files, fileConfigs, issues, err := loadAndValidate(positional, a.opts.files)
	if err != nil {
		return a.fail(1, err)
	}
	a.info("discovered files", slog.Any("files", files))
	if err := output.PrintDiscovered(a.stdout, files); err != nil {
		return a.fail(1, err)
	}
	if len(issues) > 0 {
		for _, issue := range issues {
			a.info("validation issue", slog.String("issue", issue.Error()))
		}
		_ = output.PrintValidation(a.stdout, issues)
		return exitError{code: 1}
	}

	request, submitOptions, err := payload.Build(fileConfigs)
	if err != nil {
		return a.fail(1, err)
	}
	cfg, err := config.Load(config.Options{SiteOverride: a.opts.site})
	if err != nil {
		return a.fail(1, err)
	}
	a.info("loaded config", slog.Any("config", config.Redacted(cfg)))
	client := api.NewClientWithLogger(cfg, a.logger)

	submitCtx, cancel := context.WithTimeout(ctx, a.opts.timeout)
	defer cancel()

	if err := a.resolveWarehouseConnection(submitCtx, client, &request); err != nil {
		return a.fail(1, err)
	}

	idempotencyKey := a.opts.idempotencyKey
	if idempotencyKey == "" {
		idempotencyKey, err = idempotency.Derive(idempotency.Input{
			Operation: operation,
			Payload:   request,
			Options:   submitOptions,
		})
		if err != nil {
			return a.fail(1, err)
		}
	}
	a.info("prepared operation",
		slog.String("operation", operation),
		slog.String("sync_tag", request.SyncTag),
		slog.String("idempotency_key", idempotencyKey),
		slog.String("warehouse_connection_id", request.WarehouseConnectionID),
		slog.Any("options", submitOptions),
	)

	operationResponse, err := client.Submit(submitCtx, operation, request, submitOptions, idempotencyKey)
	if err != nil {
		return a.fail(1, err)
	}
	a.info("submit response", slog.Any("operation", *operationResponse))
	if a.opts.noPoll {
		if err := output.PrintSubmittedOperation(a.stdout, operationResponse, idempotencyKey); err != nil {
			return a.fail(1, err)
		}
		return nil
	}

	if err := output.PrintSubmittedOperation(a.stderr, operationResponse, idempotencyKey); err != nil {
		return a.fail(1, err)
	}
	if err := output.PrintPollingStart(a.stderr); err != nil {
		return a.fail(1, err)
	}
	terminal, err := poll.UntilTerminal(submitCtx, client, operationResponse.MetricSyncID, a.opts.pollInterval, a.pollObserver())
	if err != nil {
		return a.fail(1, err)
	}
	a.info("terminal operation", slog.Any("operation", *terminal))
	if api.IsFailureStatus(terminal.Status) {
		_ = output.PrintOperation(a.stdout, terminal)
		return exitError{code: 1}
	}
	result, err := client.GetResult(submitCtx, terminal.MetricSyncID)
	if err != nil {
		return a.fail(1, err)
	}
	if err := output.PrintResult(a.stdout, result); err != nil {
		return a.fail(1, err)
	}
	if operation == "execute" && api.CountBucket(result.Blocked) > 0 {
		return exitError{code: 1}
	}
	return nil
}

func (a *app) resolveWarehouseConnection(ctx context.Context, client *api.Client, request *model.SyncConfig) error {
	if !needsWarehouseConnectionResolution(*request) {
		return nil
	}

	connections, err := client.ListWarehouseConnections(ctx)
	if err != nil {
		return fmt.Errorf("resolve warehouse connection: %w", err)
	}
	switch len(connections) {
	case 0:
		return fmt.Errorf("no warehouse connection is configured for this organization")
	case 1:
		request.WarehouseConnectionID = connections[0].ID
		a.info("resolved warehouse connection",
			slog.String("warehouse_connection_id", connections[0].ID),
			slog.String("name", connections[0].Name),
			slog.String("engine", connections[0].Engine),
		)
		return nil
	default:
		return fmt.Errorf("multiple warehouse connections are configured for this organization; set warehouse_connection_id in the YAML to choose one")
	}
}

func needsWarehouseConnectionResolution(request model.SyncConfig) bool {
	if request.WarehouseConnectionID != "" {
		return false
	}
	for _, source := range request.WarehouseMetricSources {
		if source.WarehouseConnectionID == "" {
			return true
		}
	}
	return false
}

func (a *app) runStatus(ctx context.Context, metricSyncID string) error {
	cfg, err := config.Load(config.Options{SiteOverride: a.opts.site})
	if err != nil {
		return a.fail(1, err)
	}
	a.info("loaded config", slog.Any("config", config.Redacted(cfg)))
	client := api.NewClientWithLogger(cfg, a.logger)
	statusCtx, cancel := context.WithTimeout(ctx, a.opts.timeout)
	defer cancel()

	operationResponse, err := client.GetStatus(statusCtx, metricSyncID)
	if err != nil {
		return a.fail(1, err)
	}
	if err := output.PrintOperation(a.stdout, operationResponse); err != nil {
		return a.fail(1, err)
	}
	return nil
}

func (a *app) runResult(ctx context.Context, metricSyncID string) error {
	cfg, err := config.Load(config.Options{SiteOverride: a.opts.site})
	if err != nil {
		return a.fail(1, err)
	}
	a.info("loaded config", slog.Any("config", config.Redacted(cfg)))
	client := api.NewClientWithLogger(cfg, a.logger)
	resultCtx, cancel := context.WithTimeout(ctx, a.opts.timeout)
	defer cancel()

	result, err := client.GetResult(resultCtx, metricSyncID)
	if err != nil {
		var notReady *api.ResultNotReadyError
		if errors.As(err, &notReady) {
			return a.fail(2, notReady)
		}
		return a.fail(1, err)
	}
	if err := output.PrintResult(a.stdout, result); err != nil {
		return a.fail(1, err)
	}
	return nil
}

func loadAndValidate(positional []string, fileFlags []string) ([]string, []model.FileConfig, []validate.Issue, error) {
	files, err := yamlutil.Discover(positional, fileFlags)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, nil, fmt.Errorf("no .yaml or .yml files found")
	}
	configs, err := yamlutil.LoadFiles(files)
	if err != nil {
		return files, nil, nil, err
	}
	issues := validate.ValidateFiles(configs)
	return files, configs, issues, nil
}

func (a *app) handleCommandError(err error) int {
	var exitErr exitError
	if errors.As(err, &exitErr) {
		if exitErr.err != nil {
			a.printError(exitErr.err)
		}
		return exitErr.code
	}
	a.printError(err)
	return 2
}

func (a *app) fail(code int, err error) error {
	a.error("command failed", err)
	return exitError{code: code, err: err}
}

func (a *app) printError(err error) {
	if err == nil {
		return
	}
	logPath := ""
	if a.logger != nil {
		logPath = a.logger.Path()
	}
	_ = output.PrintCommandError(a.stderr, err, logPath)
}

func (a *app) info(message string, attrs ...slog.Attr) {
	if a.logger == nil {
		return
	}
	a.logger.Info(message, attrs...)
}

func (a *app) error(message string, err error, attrs ...slog.Attr) {
	if a.logger == nil {
		return
	}
	allAttrs := append([]slog.Attr{slog.Any("error", err)}, attrs...)
	a.logger.Error(message, allAttrs...)
}

func (a *app) pollObserver() poll.Observer {
	return func(event poll.Event) {
		switch event.Kind {
		case poll.EventPollResult:
			a.logPollResult(event)
			_ = output.PrintPollResult(a.stderr, event.Attempt, event.Operation, event.Terminal)
		case poll.EventWait:
			a.logPollWait(event)
			_ = output.PrintPollWait(a.stderr, event.Interval, event.Elapsed)
		}
	}
}

func (a *app) logPollResult(event poll.Event) {
	if event.Operation == nil {
		return
	}
	a.info("poll result",
		slog.Int("attempt", event.Attempt),
		slog.String("metric_sync_id", event.Operation.MetricSyncID),
		slog.String("operation", event.Operation.OperationType),
		slog.String("status", event.Operation.Status),
		slog.String("sync_tag", event.Operation.SyncTag),
		slog.Bool("terminal", event.Terminal),
		slog.Duration("elapsed", event.Elapsed),
	)
}

func (a *app) logPollWait(event poll.Event) {
	a.info("poll wait",
		slog.Int("attempt", event.Attempt),
		slog.Duration("interval", event.Interval),
		slog.Duration("elapsed", event.Elapsed),
	)
}

func logFilePathFromArgs(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return logfile.DefaultPath
		}
		if arg == "--log-file" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--log-file=") {
			return strings.TrimPrefix(arg, "--log-file=")
		}
	}
	return logfile.DefaultPath
}
