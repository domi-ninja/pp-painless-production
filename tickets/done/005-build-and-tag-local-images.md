# 005 - Build and tag local images

## Goal

Build deployment images locally and tag them with deterministic release metadata.

## Dependencies

- 003

## Scope

- Run local `docker buildx build` from configured context and Dockerfile.
- Support configured target and platforms.
- Tag images using project, environment, git SHA, and timestamp-plus-SHA release ID.
- Run configured local smoke/test command if present.
- Export image tar files for SSH transfer.

## Deliverables

- Build module.
- Image tag generator.
- Image export module.
- Tests around tag generation and build command planning.

## Acceptance

- Prod hosts never build images.
- Build fails before publish if Docker is unavailable.
- Image tar paths are recorded in bundle metadata.
- Build output includes the exact image tags produced.
- Dirty source builds are allowed, and the build metadata records dirty state.
