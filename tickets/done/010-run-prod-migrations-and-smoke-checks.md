# 010 - Run production migrations and smoke checks

## Work

Run the declared migration before applying services. Record its result and the smoke checks with the release. Keep service, migration and application environment inputs separate.

This ticket introduced migration and smoke-check execution. The current behavior is documented in [configuration](../../docs/configuration.md) and [recovery](../../docs/recovery.md).

## Decision

Should validation require a sibling application or production host?

Default: Use repository-owned fixtures and disposable containers.
