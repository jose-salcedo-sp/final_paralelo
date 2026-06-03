package transport

import "final_paralelo/internal/models"

type ErrorResponse struct {
	Error string `json:"error"`
}

type LoginResponse struct {
	User  string `json:"user"`
	Token string `json:"token"`
}

type LogoutResponse struct {
	LogoutMessage string `json:"logout_message"`
}

type ControllerStatusResponse struct {
	SystemName      string   `json:"system_name"`
	ServerTime      string   `json:"server_time"`
	ActiveWorkloads []string `json:"active_workloads"`
}

type CreateWorkloadRequest struct {
	Filter       string `json:"filter"`
	WorkloadName string `json:"workload_name"`
}

type RegisterImageRequest struct {
	WorkloadID    string `json:"workload_id"`
	Type          string `json:"type"`
	Path          string `json:"path"`
	SourceImageID string `json:"source_image_id,omitempty"`
}

type RegisterImageResponse struct {
	ImageID string `json:"image_id"`
}

type RegisterWorkerRequest struct {
	Name    string   `json:"name"`
	RPCAddr string   `json:"rpc_addr"`
	Tags    []string `json:"tags"`
}

type RegisterWorkerResponse struct {
	APIEndpoint string        `json:"api_endpoint"`
	APIToken    string        `json:"api_token"`
	Worker      models.Worker `json:"worker"`
}

type WorkerHeartbeatRequest struct {
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	RunningJobs   int     `json:"running_jobs"`
}

type ListWorkersResponse struct {
	Workers []models.Worker `json:"workers"`
}

type ListJobsResponse struct {
	Jobs []models.Job `json:"jobs"`
}

type AssignJobRequest struct {
	WorkerName string `json:"worker_name"`
}

type CompleteJobRequest struct {
	FilteredImageID string `json:"filtered_image_id,omitempty"`
	Error           string `json:"error,omitempty"`
}

type WorkerProcessArgs struct {
	JobID           string `json:"job_id"`
	WorkloadID      string `json:"workload_id"`
	OriginalImageID string `json:"original_image_id"`
	Filter          string `json:"filter"`
}

type WorkerProcessReply struct {
	FilteredImageID string `json:"filtered_image_id,omitempty"`
	Error           string `json:"error,omitempty"`
}
