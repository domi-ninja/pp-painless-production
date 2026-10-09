# 006 - Transfer images and bundles over SSH

## Goal

Publish release artifacts to target hosts using SSH-first image transfer.

## Dependencies

- 004
- 005

## Scope

- Upload host-specific bundle directories.
- Transfer image tar files to each assigned host.
- Load images with `docker load`.
- Verify loaded image tags exist on the host.
- Keep private registry out of v1 default path.
- Use SSH for all remote transfer commands.

## Deliverables

- SSH transport module.
- Remote path convention.
- Image load command runner.
- Transfer progress and error reporting.

## Acceptance

- Each host receives only its required compose/env bundle.
- Image transfer is idempotent for the same release ID.
- A failed upload/load aborts before compose apply.
- Transfer logs do not expose secret env values.
- SSH failure is reported per host with the failed command stage.
