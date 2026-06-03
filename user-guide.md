# User Guide: Distributed and Parallel Image Processing

## 1. Requirements

- Go 1.22+ (or newer)
- Unix-like shell (Linux/macOS)

## 2. Install dependencies

From project root:

```bash
go mod tidy
```

## 3. Run services (separate terminals)

From project root:

### Terminal 1: Controller

```bash
go run ./controller --listen :8090 --image-root ./images --api-endpoint http://localhost:8080 --worker-api-token worker-secret-token
```

### Terminal 2: Scheduler

```bash
go run ./scheduler --controller http://localhost:8090
```

### Terminal 3: Worker 1

```bash
cd worker
go run main.go --controller http://localhost:8090 --worker-name worker-1 --tags cpu,fast
```

### Terminal 4: Worker 2

```bash
cd worker
go run main.go --controller http://localhost:8090 --worker-name worker-2 --tags cpu,default
```

### Terminal 5: API

```bash
go run ./api --listen :8080 --controller http://localhost:8090 --image-root ./images --worker-token worker-secret-token
```

## 4. API usage

## Login

```bash
curl -X POST -u user:password localhost:8080/login
```

Save returned token as `TOKEN`.

## Status

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/status
```

## Create workload

```bash
curl -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -X POST \
  -d '{"filter":"grayscale","workload_name":"demo-workload"}' \
  localhost:8080/workloads
```

Save returned `workload_id` as `WID`.

## Upload original image

```bash
curl -H "Authorization: Bearer $TOKEN" \
  -F "data=@sample.png" \
  -F "workload_id=$WID" \
  -F "type=original" \
  -X POST \
  localhost:8080/images
```

Save returned `image_id` as `ORIG_IMG_ID`.

## Check workload progress

```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X GET \
  localhost:8080/workloads/$WID
```

After scheduler and worker complete processing, `filtered_images` should include one or more image IDs.

## Download image by ID

```bash
curl -H "Authorization: Bearer $TOKEN" \
  -X GET \
  localhost:8080/images/<image_id> \
  --output downloaded.png
```

## Logout

```bash
curl -X DELETE -H "Authorization: Bearer $TOKEN" localhost:8080/logout
```

## 5. Notes

- All API responses are JSON except `GET /images/{image_id}`, which returns binary file content.
- Workloads use statuses: `scheduling`, `running`, `completed`.
- Workers send periodic heartbeats with resource usage and running jobs.
