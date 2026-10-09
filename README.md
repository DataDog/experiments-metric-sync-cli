# Datadog Experiments Metric Sync CLI

Datadog Experiments Metric Sync CLI prepares metric sync definitions, submits them to Datadog's Metric Sync API, polls the async operation, and prints the results.


## Installation

Install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/DataDog/experiments-metric-sync-cli/main/install.sh | sh
```

The install script detects macOS or Linux, downloads the matching GitHub Release
archive, verifies it against the published SHA-256 checksum, and installs
`metric-sync` to `/usr/local/bin`.

To install a specific version or install into a different directory:

```sh
curl -fsSL https://raw.githubusercontent.com/DataDog/experiments-metric-sync-cli/main/install.sh | env VERSION=v0.1.0 INSTALL_DIR="$HOME/.local/bin" sh
```

## Authentication

Set Datadog API credentials through environment variables:

```sh
export DD_API_KEY=...
export DD_APP_KEY=...
export DD_SITE=datadoghq.com (optional)
```

## Usage

By default, the CLI sends requests to `datadoghq.com`.

In CI, run `plan` first to validate the metric definitions and preview the diff:

```sh
metric-sync plan ./metrics
```

Then run `execute` when you are ready to apply the changes:

```sh
metric-sync execute ./metrics
```

`execute` discovers metric sync YAML files, validates them locally, submits a
write operation to the Metric Sync API, polls until the operation reaches a
terminal state, and prints the created, updated, deleted, upgraded, blocked, and
error counts.

Other commands:

```sh
metric-sync validate ./metrics
metric-sync status <metric_sync_id>
metric-sync result <metric_sync_id>
metric-sync version
```

`validate` only performs local validation and does not call Datadog. `status`
checks an operation by ID. `result` fetches the terminal plan or execute result.
`version` prints build metadata.

## Idempotency

The CLI sends an `Idempotency-Key` for `plan` and `execute` requests. In CI, the
generated key includes the CI run identity so retries in the same run dedupe
naturally. In local/manual runs, the generated key includes a fresh nonce so
re-running the same file starts a new operation instead of replaying an old one.

Use `--idempotency-key` only when you intentionally want to replay or debug a
specific request.

## Preserve existing metric settings

Copy the current settings into YAML before you adopt an existing source or metric.
Omitted fields use API defaults; they do not preserve the current UI settings.

On a source, `additional_timestamp_columns` lists extra timestamp columns:

```yaml
timestamp_column: BOOKING_DATETIME_UTC
additional_timestamp_columns:
  - FIRST_TRANSACTION_DATETIME_UTC
```

The source SQL must return these columns. The list can contain at most four
unique, nonblank names, each at most 255 characters. It cannot include the
primary `timestamp_column`. Omission or an empty list means no extra timestamps.

Set `winsorization_strategy` inside `simple_metric_aggregation`, or separately
inside each ratio `numerator_aggregation` and `denominator_aggregation`:

```yaml
winsor_upper_percentile: 0.99
winsorization_strategy: nonzero
```

`nonzero` calculates percentile cutoffs from nonzero values. The other strategy,
`all_assigned_subjects`, is the default when the field is omitted. These values
are examples; use the actual strategy and bounds from your metric. Percentiles
must be between 0 and 1, and the lower percentile must be less than the upper
percentile. For `threshold`, omit all winsorization bounds and either omit the
strategy or set it to `all_assigned_subjects`.

See [settings.yaml](examples/settings.yaml) for a complete example with a simple
metric and a ratio metric. Run `validate`, then review `plan` before `execute`.
