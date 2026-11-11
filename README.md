# Datadog Experiments Metric Sync CLI

Datadog Experiments Metric Sync CLI prepares metric sync definitions, submits them to Datadog's public Experiments v2 Metric Sync API, polls the async operation, and prints the results.

Use one YAML document per file. To split definitions, use multiple files in the
same directory with the same sync tag and options. Additional documents after a
`---` separator are rejected before validation or submission.


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
error counts. It also shows object IDs, setting changes, blocked reasons, server
warnings, and available errors. Plans show proposed changes. A failed result does
not confirm that its proposed changes were applied.

A blocked plan or execute result exits nonzero, as does a failed operation. The
`result` command uses the same exit behavior. With `--no-poll`, exit zero confirms
only submission; use `status` and `result` to check completion.

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

### Adopting manually created metrics and sources

To adopt existing manual objects by name, set `options.upgrade_mode: by_name`
and add `upgrade_by_name: true` to each metric or warehouse metric source you
want to adopt. The global option alone does not opt objects into adoption.
Add these fields to your complete definitions:

```yaml
options:
  upgrade_mode: by_name
warehouse_metric_sources:
  - sync_id: orders
    name: Orders
    upgrade_by_name: true
    # Include the remaining source fields.
metrics:
  - sync_id: revenue
    name: Revenue
    upgrade_by_name: true
    # Include the remaining metric fields.
```

Names must match exactly. A single matching manual object is adopted; no match
creates a new object. Multiple matching manual objects are blocked: use
`options.upgrade_mode: by_id` and the intended object's `existing_id` instead.
Omitting `upgrade_by_name` or setting it to `false` leaves that object opted out
of name-based adoption. Run `plan` and review upgrades and definition changes
before `execute`, since adoption can update the existing object to match the YAML.

### Metric collections

Metric collections group metrics, in order, and are synced from the same YAML
as the metrics they contain. See `examples/collections.yaml`.

```yaml
metric_collections:
  - sync_id: checkout_core
    name: Checkout core
    description: Core checkout KPIs
    is_guardrail: false
    metrics:
      - metric_sync_id: checkout_revenue
      - metric_sync_tag: growth
        metric_sync_id: signups
```

- A collection is identified by its `sync_id` within the sync tag. Members are
  ordered; reordering them is a change.
- A member without `metric_sync_tag` refers to a metric under the file's
  `sync_tag`, and that metric must be defined in the files of the same
  operation. Set `metric_sync_tag` to reference a metric synced by another tag.
- Members must be metrics synced by Metric Sync. A member `existing_id` is
  rejected: adopt a UI metric first (see above), then reference it by
  `metric_sync_id`.
- Names must contain 3 to 40 characters after trimming whitespace. Collection
  `sync_id`s and names must be unique across the files of one operation. A
  collection holds at most 100 metrics, and a metric can appear only once in a
  collection.
- With multiple files, the `metric_collections` entries of all files are
  combined. A file that omits the key, or sets it to `[]` or `null`,
  contributes nothing and does not remove another file's collections.

The combined list is the complete desired set of collections for the sync tag.
If no file lists any collection (the key is omitted, `[]`, or `null` in every
file), every collection managed by that sync tag is deleted. Deletion is
permanent: re-adding the YAML later creates a new collection with a new ID. Run
`plan` and review every `Delete collection` line before `execute`; the output
also ends with a warning that lists every collection the operation deletes. An
empty list item (for example an entry whose fields are all commented out) in
`metric_collections`, a collection's `metrics`, `metrics`, or
`warehouse_metric_sources` is rejected, so a stray blank entry cannot delete a
collection or member.

A synced collection is locked in Datadog: editing or deleting it in the UI or
through the API returns `409 Conflict` with a message that the collection is
managed by metric sync and must be changed in source control. Change it in YAML
and sync again, or release its tag (see
[Releasing or resetting a sync tag](#releasing-or-resetting-a-sync-tag)) to
manage it by hand.

#### Adopting collections

Adoption works like it does for metrics, and is shallow: the collection keeps
its ID, but its member metrics are not adopted. Every member must still be a
synced metric. See `examples/collections-adoption.yaml` for name-based
adoption. With `upgrade_by_name: true`, no matching manual collection creates
one, a single match is adopted, and multiple matches are blocked. Without
`upgrade_by_name`, a manual collection with the same name blocks the sync
instead of creating a duplicate.

Name matching is exact, the same rule as metric by-name adoption: the server
compares the trimmed YAML name against stored names as they are, case-sensitive
and without trimming the stored name. A collection created in the UI with
different case or with leading or trailing spaces is not matched: it does not
block a create, and `upgrade_by_name` does not find it. To adopt such a
collection, use `existing_id` with `upgrade_mode: by_id`.

A collection synced by another tag cannot be adopted, by `existing_id` or by
`upgrade_by_name`: the result reports it as blocked and names the tag that
manages it. Release that tag first (see
[Releasing or resetting a sync tag](#releasing-or-resetting-a-sync-tag)), or
move the collection's YAML to the tag that manages it. To adopt by ID:

```yaml
options:
  upgrade_mode: by_id
metric_collections:
  - sync_id: checkout_core
    existing_id: 00000000-0000-0000-0000-000000000000
    name: Checkout core
    metrics:
      - metric_sync_id: checkout_revenue
```

#### Cross-tag members

A member under another tag must already be synced by that tag; sync the other
tag first. While a collection references a metric, that metric cannot be
deleted by its own tag, even with `force_delete`, and that tag cannot be
released or reset; the result reports the metric, the referencing collection
and its tag. To remove such a metric:

1. Remove the member from the collection that references it.
2. Sync the collection's tag.
3. Delete the metric, or release or reset its tag (see
   [Releasing or resetting a sync tag](#releasing-or-resetting-a-sync-tag)).

Metric collections must be enabled for your Datadog organization. Until they
are, `plan` previews collection changes, and `execute` fails before writing
anything when it would create, update, adopt, or delete a collection. That
includes deleting collections a tag already manages by omitting
`metric_collections`.

### Releasing or resetting a sync tag

The CLI has no release or reset command. To hand a tag's objects back to manual
management, call the Metric Sync API directly with the same `DD_API_KEY` and
`DD_APP_KEY`:

- `POST /api/v2/experiments/metric-syncs/tags/{sync_tag}/release` clears sync
  ownership from the tag's sources, measures, properties, metrics, and
  collections. The objects stay active and become manually managed; collections
  keep their members.
- `POST /api/v2/experiments/metric-syncs/tags/{sync_tag}/reset?force=true` does
  the same release and also drops the tag's sync history, so the next sync for
  the tag starts from a clean lineage.

Released objects are manual objects: to sync them again, adopt them with
`existing_id` or `upgrade_by_name`. A release or reset is refused while a
collection under another tag references one of the tag's metrics (see
[Cross-tag members](#cross-tag-members)). The full contract, unblock steps, and
rollback runbook are in `dd-source`, in
`domains/ffe/libs/experimentation/core/metric_sync/schema/schema_v1/README.md`.

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

## Idempotency

The CLI sends an `Idempotency-Key` for `plan` and `execute` requests. In CI, the
generated key includes the CI run identity so retries in the same run dedupe
naturally. In local/manual runs, the generated key includes a fresh nonce so
re-running the same file starts a new operation instead of replaying an old one.

Use `--idempotency-key` only when you intentionally want to replay or debug a
specific request. Reusing a key with the same request returns the recorded
result. To apply the same files again after a failed or suspect operation,
review a fresh `plan`, then run `execute` with a new key.

## API behavior

The CLI uses these public Experiments v2 routes:

```text
POST /api/v2/experiments/metric-syncs
GET  /api/v2/experiments/metric-syncs/{metric_sync_id}
GET  /api/v2/experiments/metric-syncs/{metric_sync_id}/result
```

These routes must be available on your Datadog site before using this CLI.
`plan` and `execute` poll by default and exit with code 1 on terminal failure.

To submit without waiting, then inspect the returned operation ID:

```sh
metric-sync plan ./metrics --no-poll --idempotency-key ci-preview-123
metric-sync status <metric_sync_id>
metric-sync result <metric_sync_id>
metric-sync execute ./metrics --no-poll
```

A result that is still pending exits with code 2; retry `result` once the
operation completes.

HTTP 429 responses are retried for all API requests, including submission and
warehouse lookup, up to eight attempts total and three minutes of retry waiting
per request. The command's `--timeout` can stop retries sooner. Retries honor
`Retry-After` (seconds or HTTP date) and Datadog's `X-RateLimit-Reset` (seconds
until reset), using the longer valid delay plus up to one second of jitter.
Missing or invalid guidance uses exponential backoff with jitter, starting at
1–2 seconds and capped at 30 seconds. A server delay beyond the remaining retry
budget returns an error instead of retrying early. Submission retries preserve
the payload and `Idempotency-Key`. Other HTTP errors and transport failures are
not retried.

Warehouse auto-selection still uses
`GET /api/unstable/ffe/warehouse-connections` when a source omits its connection
ID and no top-level `warehouse_connection_id` is set. It selects the sole
connection and errors when zero or multiple connections exist. To skip this
unstable lookup, set `warehouse_connection_id` at the top level of the YAML or
on every warehouse metric source.
