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

For YAML fields and measure references, see
[Sync Experiment Metrics from YAML](https://docs.datadoghq.com/experiments/defining_metrics/metric_sync/#yaml-reference).
See [examples](examples/) for complete YAML files.

### Properties and filters

Source properties use `property_name`, `property_column`, and `property_type`
in both YAML and API requests. Rename the previous YAML fields `name`,
`column_name`, and `column_type` on properties to these API names. The old
property field names are rejected. Measure fields remain `name`, `column_name`,
and `column_type`. Use `property_sync_id` and `warehouse_metric_source_sync_id`
to reference a property in a filter.

`property_filters` accepts a single filter, a flat list of filters (AND), or
nested groups (OR between groups, AND within each group). The CLI converts the
single filter and flat list forms to one group in the API request.

For one condition, no list markers are needed:

```yaml
property_filters:
  property:
    warehouse_metric_source_sync_id: source
    property_sync_id: country
  operation: IS
  values: [US, CA]
```

For multiple AND conditions, use a flat list:

```yaml
property_filters:
  - property:
      warehouse_metric_source_sync_id: source
      property_sync_id: country
    operation: IS
    values: [US, CA]
  - property:
      warehouse_metric_source_sync_id: source
      property_sync_id: country
    operation: IS_NOT
    values: [CA]
```

Use explicit nested groups for OR. Do not mix filters and groups in one list.
Use `operation: IS` or `operation: IS_NOT` with a nonempty list of values.
Simple and ratio metrics also support numeric measure filters and null checks
when enabled by the API. Percentile metrics support property filters with `IS`
and `IS_NOT` only. Filters must use the same source as the aggregation.
See [the filter example](examples/filters.yaml).

The `operator` field remains an alias for `operation`. These changes require a release newer than
v0.1.1; they are available when building this branch from source.

Use one YAML document per file. To split definitions, use multiple files in the
same directory with the same sync tag and options. Additional documents after a
`---` separator are rejected before validation or submission.

## Idempotency

The CLI sends an `Idempotency-Key` for `plan` and `execute` requests. In CI, the
generated key includes the CI run identity so retries in the same run dedupe
naturally. In local/manual runs, the generated key includes a fresh nonce so
re-running the same file starts a new operation instead of replaying an old one.

Use `--idempotency-key` only when you intentionally want to replay or debug a
specific request.
