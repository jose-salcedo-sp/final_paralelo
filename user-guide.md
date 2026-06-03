# User Guide: Distributed and Parallel Image Processing

## 1. Requirements

- Go 1.25 or newer
- Windows PowerShell
- `curl.exe` (included with modern Windows)
- Python 3.10 or newer for the provided stress/video tools

## 2. Install Go dependencies

From the project root:

```powershell
go mod download
go test ./...
```

## 3. Run the services on Windows

To start all services from one PowerShell command:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-components.ps1
```

This opens a separate PowerShell terminal window for the controller, scheduler, each worker, and the API.

To choose how many workers to start:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-components.ps1 -WorkerCount 6
```

To stop services started by that script:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-components.ps1 -Stop
```

To run everything in hidden background windows and write logs to `logs/` instead:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-components.ps1 -Hidden
```

You can combine options for a faster large provided test run:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-components.ps1 -WorkerCount 6 -Hidden
```

Open five separate PowerShell terminals. Start from the project root in each terminal:

### Terminal 1: Controller

```powershell
go run ./controller --listen :8090 --image-root ./images --api-endpoint localhost:8080 --worker-api-token worker-secret-token
```

### Terminal 2: Scheduler

```powershell
go run ./scheduler --controller localhost:8090 --poll 250ms
```

### Terminal 3: Worker 1

```powershell
Set-Location .\worker
go run main.go --controller localhost:8090 --worker-name worker-1 --tags cpu,fast
```

### Terminal 4: Worker 2

```powershell
Set-Location .\worker
go run main.go --controller localhost:8090 --worker-name worker-2 --tags cpu,default
```

### Terminal 5: API

```powershell
go run ./api --listen :8080 --controller localhost:8090 --image-root ./images --worker-token worker-secret-token
```

Worker startup also supports the rubric format:

```powershell
cd worker
go run main.go --controller <host>:<port> --worker-name <worker_name> --tags <tag1>,<tag2>
```

## 4. API usage in PowerShell

### Login

```powershell
$login = curl.exe -s -X POST -u user:password http://localhost:8080/login | ConvertFrom-Json
$TOKEN = $login.token
```

`username:password` also works for the provided test flow.

### System status

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/status
```

### Create a workload with JSON

```powershell
$workload = curl.exe -s -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -X POST -d '{"filter":"grayscale","workload_name":"demo-workload"}' http://localhost:8080/workloads | ConvertFrom-Json
$WID = $workload.workload_id
```

### Create a workload with the provided no-body request

```powershell
$workload = curl.exe -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/workloads | ConvertFrom-Json
$WID = $workload.workload_id
```

### Upload an original image

Replace `tests/sample.png` with any PNG file if that sample file is not present.

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" -F "data=@tests/sample.png" -F "workload_id=$WID" -F "type=original" -X POST http://localhost:8080/images
```

### Check workload progress

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/workloads/$WID
```

When the scheduler and worker finish, `status` becomes `completed` and `filtered_images` contains the processed image IDs.

### List image records

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/images
```

### Download an image

```powershell
$IMGID = "<image_id>"
curl.exe -L -H "Authorization: Bearer $TOKEN" "http://localhost:8080/images/$IMGID" --output downloaded.png
```

### Logout

```powershell
curl.exe -s -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:8080/logout
```

## 5. Endpoint examples

Use these commands after the API is running on `localhost:8080`.

### POST /login

```powershell
$login = curl.exe -s -X POST -u user:password http://localhost:8080/login | ConvertFrom-Json
$TOKEN = $login.token
```

Expected JSON fields: `user`, `token`.

### DELETE /logout

```powershell
curl.exe -s -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:8080/logout
```

Expected JSON fields: `logout_message`.

### GET /status

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/status
```

Expected JSON fields: `system_name`, `server_time`, `active_workloads`.

### POST /workloads

```powershell
curl.exe -s -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -X POST -d '{"filter":"grayscale","workload_name":"demo-workload"}' http://localhost:8080/workloads
```

Expected JSON fields: `workload_id`, `filter`, `workload_name`, `status`, `running_jobs`, `filtered_images`.

Provided-test-compatible no-body workload creation:

```powershell
$workload = curl.exe -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/workloads | ConvertFrom-Json
$WID = $workload.workload_id
```

### GET /workloads/{workload_id}

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" "http://localhost:8080/workloads/$WID"
```

Expected JSON fields: `workload_id`, `filter`, `workload_name`, `status`, `running_jobs`, `filtered_images`.

### POST /images

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" -F "data=@tests/sample.png" -F "workload_id=$WID" -F "type=original" -X POST http://localhost:8080/images
```

Expected JSON fields: `workload_id`, `image_id`, `type`.

Workers also call this endpoint with `type=filtered` and `source_image_id=<original_image_id>`.

### GET /images

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/images
```

Returns image records with fields such as `image_id`, `workload_id`, `type`, and `source_image_id`.

### GET /images/{image_id}

```powershell
$IMGID = "<image_id>"
curl.exe -L -H "Authorization: Bearer $TOKEN" "http://localhost:8080/images/$IMGID" --output downloaded.png
```

This endpoint returns file content instead of JSON.

## 6. Run the Provided Tests on Windows

After starting the Go services, you can run the full provided test workflow with:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-provided-tests.ps1
```

Create a Python virtual environment from the project root:

```powershell
py -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r .\provided-tests\requirements.txt
```

The provided Python test files are stored in the `provided-tests` directory.

Download a sample video and extract frames:

```powershell
curl.exe -L -o big_buck_bunny_720p_stereo.avi https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_720p_stereo.avi
.\.venv\Scripts\python.exe .\provided-tests\video_utils_windows.py -action extract big_buck_bunny_720p_stereo.avi frames
```

With all Go services running, create a token and no-body workload:

```powershell
$login = curl.exe -s -X POST -u username:password http://localhost:8080/login | ConvertFrom-Json
$TOKEN = $login.token
$workload = curl.exe -s -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8080/workloads | ConvertFrom-Json
$WID = $workload.workload_id
```

Push frames to the API:

```powershell
.\.venv\Scripts\python.exe .\provided-tests\stress_test.py -action push -workload-id $WID -token $TOKEN -frames-path frames
```

Wait until the workload is completed:

```powershell
curl.exe -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/workloads/$WID
```

Pull filtered images and join them into a video:

```powershell
.\.venv\Scripts\python.exe .\provided-tests\stress_test.py -action pull -workload-id $WID -image-type filtered -token $TOKEN -frames-path filtered
.\.venv\Scripts\python.exe .\provided-tests\video_utils_windows.py -action join filtered.mp4 filtered
```

## 7. Notes

- All API responses are JSON except `GET /images/{image_id}`, which returns file content.
- URL flags accept both `localhost:8090` and `http://localhost:8090`.
- Workloads use `scheduling`, `running`, and `completed` statuses.
- Workers download originals from the API, run the filter, upload filtered images, and report heartbeats to the controller.
