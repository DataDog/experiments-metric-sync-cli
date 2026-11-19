# Vendored Schemas

`metric_sync_v1.schema.json` is copied from:

```text
DataDog/dd-source/domains/ffe/libs/experimentation/core/metric_sync/schema/schema_v1/metric_sync_v1.schema.json
```

Source commit when initially vendored:

```text
85503a4995d2c5cb154c54cdcf6bef1af6661230
```

Until Datadog publishes a public schema artifact, `dd-source` remains the source of truth. Update this file when refreshing the vendored schema so schema drift is detectable.

## Local additions

The `owner_email` metric property mirrors the pending `dd-source` metric owner change (ddoghq/dd-source#190191). It was added here ahead of that change merging; re-vendor the file from `dd-source` once it lands and drop this note.
