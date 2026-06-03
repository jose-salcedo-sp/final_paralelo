package models

type Worker struct {
	Name          string   `json:"name"`
	RPCAddr       string   `json:"rpc_addr"`
	Tags          []string `json:"tags"`
	CPUPercent    float64  `json:"cpu_percent"`
	MemoryPercent float64  `json:"memory_percent"`
	RunningJobs   int      `json:"running_jobs"`
	LastHeartbeat int64    `json:"last_heartbeat"`
}

type Workload struct {
	ID             string   `json:"workload_id"`
	Name           string   `json:"workload_name"`
	Filter         string   `json:"filter"`
	Status         string   `json:"status"`
	RunningJobs    int      `json:"running_jobs"`
	FilteredImages []string `json:"filtered_images"`
	OriginalImages []string `json:"original_images"`
}

type ImageRecord struct {
	ID            string `json:"image_id"`
	WorkloadID    string `json:"workload_id"`
	Type          string `json:"type"`
	Path          string `json:"path"`
	SourceImageID string `json:"source_image_id,omitempty"`
}

type Job struct {
	ID              string `json:"job_id"`
	WorkloadID      string `json:"workload_id"`
	OriginalImageID string `json:"original_image_id"`
	Filter          string `json:"filter"`
	Status          string `json:"status"`
	AssignedWorker  string `json:"assigned_worker,omitempty"`
	FilteredImageID string `json:"filtered_image_id,omitempty"`
	Error           string `json:"error,omitempty"`
}
