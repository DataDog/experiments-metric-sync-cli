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

The `metric_collections` request property, the `metric_collection`, `metric_collection_member`, and `collection_name` definitions, and the `metric_collections` diff bucket mirror the pending `dd-source` metric collection change. They were added here ahead of that change merging; re-vendor the file from `dd-source` once it lands and drop this note.
