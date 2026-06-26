# Datadog Experiment Metric Sync CLI

Datadog Experiment Metric Sync CLI prepares metric sync definitions, submits them to Datadog's Metric Sync API, polls the async operation, and prints CI-friendly results.


## Authentication

Set Datadog API credentials through environment variables:

```sh
export DD_API_KEY=...
export DD_APP_KEY=...
export DD_SITE=datadoghq.com (optional)
```

## Usage

```sh
metric-sync plan examples/
metric-sync execute examples/
metric-sync status <metric_sync_id>
metric-sync result <metric_sync_id>
```

`plan` and `execute` both discover and validate YAML before calling Datadog. Use
`validate` only when you want a local validation check without submitting an
operation.

## Idempotency

The CLI sends an `Idempotency-Key` for `plan` and `execute` requests. In CI, the
generated key includes the CI run identity so retries in the same run dedupe
naturally. In local/manual runs, the generated key includes a fresh nonce so
re-running the same file starts a new operation instead of replaying an old one.

Use `--idempotency-key` only when you intentionally want to replay or debug a
specific request.
