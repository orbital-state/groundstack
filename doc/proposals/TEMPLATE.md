# GRS-<CAT>-XXXX-<slug>

**Status**: drafted | accepted | rejected | withdrawn
**Created**: YYYY-MM-DD
**Authors**: @github_handle
**Discussion**: [link to discussion thread]

## Proposal ID format

Proposals are stored as **subfolders** under:

- `doc/proposals/drafted/`
- `doc/proposals/accepted/`
- `doc/proposals/resolved/`

Folder naming convention:

`GRS-<CAT>-XXXX-<slug>/`

Where the **proposal ID** is:

`GRS-<CAT>-XXXX`

and the folder adds `-<slug>` to keep titles human-scannable (see `0001`).

Inside each proposal folder:

- `README.md` (the proposal; copy this template)
- Optional supporting docs like `plan.md`, diagrams, traces, etc.

### Proposal ID format

The proposal ID uses the format:

`GRS-<CAT>-XXXX`

Where:
- `<CAT>` is a short category label (see below).
- `XXXX` is a zero-padded numeric sequence (e.g. `0001`).
- `<slug>` (folder only) is a short, kebab-case title suffix (e.g. `init-go-api`).

### Categories

Use the smallest category that best fits:
- `CORE`: overall architecture, component boundaries, module layout, big design pivots
- `STATE`: state model, consistency/CRUD semantics, migrations, persistence guarantees
- `API`: HTTP surfaces (ARM/AAD/Graph), routing, Azure-shaped request/response behavior
- `NET`: DNS overrides, TLS termination, edge proxy routing, host/SNI behavior
- `SEC`: auth model, token contents/signing, sanitization, secrets handling
- `VM`: compute control-plane (VMs, disks, extensions), LRO behaviors
- `DB`: Postgres schema, resource indexing/query semantics, persistence guarantees
- `ID`: AAD/Graph identity surfaces (tenant/app/SP concepts, stub behaviors)
- `STO`: storage control-plane + data-plane (blob/queue/table/file) behaviors
- `OBS`: request tracing/logging/metrics, redaction, debug UX
- `DX`: local dev ergonomics, tooling, examples, CI harnesses

If you need more, add one intentionally (keep it 2–6 uppercase letters) and update this list.

### Example

Folder:

`doc/proposals/drafted/GRS-CORE-0001-init-go-api/`

Proposal ID:

`GRS-CORE-0001`

## Abstract
One paragraph summary of the change.

## Motivation

## Specification

## Backwards Compatibility

## Security Considerations

## Deployment / Activation

## Reference Implementation

## Test Plan

## Changelog
- YYYY-MM-DD: drafted
- YYYY-MM-DD: accepted