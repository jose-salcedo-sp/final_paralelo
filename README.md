# Distributed and Parallel Image Processing (DPIP)

This project implements a distributed image processing platform in Go with four components:

- API service
- Controller (master node)
- Scheduler
- Worker nodes

It supports authenticated workload creation, image upload/download, worker registration, and distributed filtering (grayscale and blur).

## Components

- `api/`: REST API with token authentication and required assignment endpoints.
- `controller/`: in-memory datastore for workers, workloads, images, and jobs.
- `scheduler/`: smart worker selection based on running jobs + CPU + memory.
- `worker/`: RPC node that processes images and reports heartbeats.
- `internal/`: shared models, transport types, auth, and filters.

## Quick start

Detailed setup and usage is documented in `user-guide.md`.

## Required deliverables included

- Complete endpoint set from the project PDF.
- In-memory distributed architecture with process-separated services.
- At least one worker-side filtering method (grayscale, plus blur).
- Worker startup contract:
  - `cd worker/`
  - `go run main.go --controller <host>:<port> --worker-name <worker_name> --tags <tag1>,<tag2>`

## Future work

1. Replace in-memory datastore with persistent database and recovery snapshots.
2. Add GPU/CUDA acceleration mode for worker-side filtering.
3. Add queue durability, retries, and dead-letter handling for failed jobs.
