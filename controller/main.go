package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"final_paralelo/internal/models"
	"final_paralelo/internal/transport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type state struct {
	mu        sync.RWMutex
	workers   map[string]models.Worker
	workloads map[string]models.Workload
	images    map[string]models.ImageRecord
	jobs      map[string]models.Job

	nextImageID     int64
	nextJobSequence int64
}

func main() {
	listen := flag.String("listen", ":8090", "controller listen address")
	systemName := flag.String("system-name", "DPIP Controller", "system name")
	imageRoot := flag.String("image-root", "./images", "image storage root")
	apiEndpoint := flag.String("api-endpoint", "http://localhost:8080", "api endpoint for workers")
	workerAPIToken := flag.String("worker-api-token", "worker-secret-token", "worker token to call API")
	flag.Parse()
	apiEndpointURL := normalizeHTTPURL(*apiEndpoint)

	if err := os.MkdirAll(*imageRoot, 0o755); err != nil {
		panic(err)
	}

	st := &state{
		workers:   make(map[string]models.Worker),
		workloads: make(map[string]models.Workload),
		images:    make(map[string]models.ImageRecord),
		jobs:      make(map[string]models.Job),
	}

	r := gin.Default()

	r.POST("/workers/register", func(c *gin.Context) {
		var req transport.RegisterWorkerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}
		if req.Name == "" || req.RPCAddr == "" {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: "name and rpc_addr are required"})
			return
		}

		worker := models.Worker{
			Name:          req.Name,
			RPCAddr:       req.RPCAddr,
			Tags:          req.Tags,
			LastHeartbeat: time.Now().Unix(),
		}

		st.mu.Lock()
		st.workers[worker.Name] = worker
		st.mu.Unlock()

		c.JSON(http.StatusCreated, transport.RegisterWorkerResponse{
			APIEndpoint: apiEndpointURL,
			APIToken:    *workerAPIToken,
			Worker:      worker,
		})
	})

	r.POST("/workers/heartbeat", func(c *gin.Context) {
		var req transport.WorkerHeartbeatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}

		st.mu.Lock()
		defer st.mu.Unlock()
		worker, ok := st.workers[req.Name]
		if !ok {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: "worker not found"})
			return
		}

		worker.CPUPercent = req.CPUPercent
		worker.MemoryPercent = req.MemoryPercent
		worker.RunningJobs = req.RunningJobs
		worker.LastHeartbeat = time.Now().Unix()
		st.workers[worker.Name] = worker
		c.JSON(http.StatusOK, gin.H{"message": "heartbeat updated"})
	})

	r.GET("/workers", func(c *gin.Context) {
		st.mu.RLock()
		defer st.mu.RUnlock()
		out := make([]models.Worker, 0, len(st.workers))
		for _, w := range st.workers {
			out = append(out, w)
		}
		c.JSON(http.StatusOK, transport.ListWorkersResponse{Workers: out})
	})

	r.POST("/workloads", func(c *gin.Context) {
		req, err := bindOptionalCreateWorkload(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}
		if req.Filter != "grayscale" && req.Filter != "blur" {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: "invalid filter"})
			return
		}

		workloadID := uuid.NewString()
		workload := models.Workload{
			ID:             workloadID,
			Name:           req.WorkloadName,
			Filter:         req.Filter,
			Status:         "scheduling",
			FilteredImages: []string{},
			OriginalImages: []string{},
		}

		workloadDir := filepath.Join(*imageRoot, req.WorkloadName)
		if err := os.MkdirAll(workloadDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, transport.ErrorResponse{Error: err.Error()})
			return
		}

		st.mu.Lock()
		st.workloads[workloadID] = workload
		st.mu.Unlock()

		c.JSON(http.StatusCreated, workload)
	})

	r.GET("/workloads", func(c *gin.Context) {
		st.mu.RLock()
		defer st.mu.RUnlock()
		out := make([]models.Workload, 0, len(st.workloads))
		for _, w := range st.workloads {
			out = append(out, w)
		}
		c.JSON(http.StatusOK, gin.H{"workloads": out})
	})

	r.GET("/workloads/:id", func(c *gin.Context) {
		st.mu.RLock()
		defer st.mu.RUnlock()
		workload, ok := st.workloads[c.Param("id")]
		if !ok {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: "workload not found"})
			return
		}
		c.JSON(http.StatusOK, workload)
	})

	r.POST("/images/register", func(c *gin.Context) {
		var req transport.RegisterImageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}
		if req.WorkloadID == "" || req.Path == "" || (req.Type != "original" && req.Type != "filtered") {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: "invalid image register payload"})
			return
		}

		st.mu.Lock()
		defer st.mu.Unlock()
		workload, ok := st.workloads[req.WorkloadID]
		if !ok {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: "workload not found"})
			return
		}

		imgID := st.allocateImageID()
		img := models.ImageRecord{
			ID:            imgID,
			WorkloadID:    req.WorkloadID,
			Type:          req.Type,
			Path:          req.Path,
			SourceImageID: req.SourceImageID,
		}
		st.images[imgID] = img

		if req.Type == "original" {
			workload.OriginalImages = append(workload.OriginalImages, imgID)
			job := models.Job{
				ID:              uuid.NewString(),
				WorkloadID:      req.WorkloadID,
				OriginalImageID: imgID,
				Filter:          workload.Filter,
				Status:          "pending",
				Sequence:        st.allocateJobSequence(),
			}
			st.jobs[job.ID] = job
		} else {
			workload.FilteredImages = appendUnique(workload.FilteredImages, imgID)
		}
		st.workloads[workload.ID] = workload

		c.JSON(http.StatusCreated, transport.RegisterImageResponse{ImageID: imgID})
	})

	r.GET("/images", func(c *gin.Context) {
		st.mu.RLock()
		defer st.mu.RUnlock()
		c.JSON(http.StatusOK, sortedImages(st.images))
	})

	r.GET("/images/:id", func(c *gin.Context) {
		st.mu.RLock()
		defer st.mu.RUnlock()
		img, ok := st.images[c.Param("id")]
		if !ok {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: "image not found"})
			return
		}
		c.JSON(http.StatusOK, img)
	})

	r.GET("/jobs/pending", func(c *gin.Context) {
		st.mu.RLock()
		defer st.mu.RUnlock()
		out := make([]models.Job, 0)
		for _, job := range st.jobs {
			if job.Status == "pending" {
				out = append(out, job)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].Sequence < out[j].Sequence
		})
		c.JSON(http.StatusOK, transport.ListJobsResponse{Jobs: out})
	})

	r.POST("/jobs/:id/assign", func(c *gin.Context) {
		var req transport.AssignJobRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}
		st.mu.Lock()
		defer st.mu.Unlock()
		if _, ok := st.workers[req.WorkerName]; !ok {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: "worker not found"})
			return
		}
		job, ok := st.jobs[c.Param("id")]
		if !ok {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: "job not found"})
			return
		}
		if job.Status != "pending" {
			c.JSON(http.StatusConflict, transport.ErrorResponse{Error: "job already assigned"})
			return
		}
		job.Status = "running"
		job.AssignedWorker = req.WorkerName
		st.jobs[job.ID] = job

		worker := st.workers[req.WorkerName]
		worker.RunningJobs++
		st.workers[req.WorkerName] = worker

		workload := st.workloads[job.WorkloadID]
		workload.Status = "running"
		workload.RunningJobs++
		st.workloads[workload.ID] = workload

		c.JSON(http.StatusOK, job)
	})

	r.POST("/jobs/:id/complete", func(c *gin.Context) {
		var req transport.CompleteJobRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}

		st.mu.Lock()
		defer st.mu.Unlock()
		job, ok := st.jobs[c.Param("id")]
		if !ok {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: "job not found"})
			return
		}
		if job.Status != "running" {
			c.JSON(http.StatusConflict, transport.ErrorResponse{Error: "job not running"})
			return
		}

		if req.Error != "" {
			job.Status = "failed"
			job.Error = req.Error
		} else {
			job.Status = "completed"
			job.FilteredImageID = req.FilteredImageID
		}
		st.jobs[job.ID] = job

		if worker, ok := st.workers[job.AssignedWorker]; ok {
			if worker.RunningJobs > 0 {
				worker.RunningJobs--
			}
			st.workers[worker.Name] = worker
		}

		workload := st.workloads[job.WorkloadID]
		if workload.RunningJobs > 0 {
			workload.RunningJobs--
		}
		if req.FilteredImageID != "" {
			workload.FilteredImages = appendUnique(workload.FilteredImages, req.FilteredImageID)
		}
		workload.Status = deriveWorkloadStatus(st.jobs, workload.ID)
		st.workloads[workload.ID] = workload

		c.JSON(http.StatusOK, job)
	})

	r.GET("/status", func(c *gin.Context) {
		st.mu.RLock()
		defer st.mu.RUnlock()

		active := make([]string, 0)
		for _, workload := range st.workloads {
			if workload.Status != "completed" {
				active = append(active, workload.ID)
			}
		}
		c.JSON(http.StatusOK, transport.ControllerStatusResponse{
			SystemName:      *systemName,
			ServerTime:      time.Now().Format(time.RFC3339),
			ActiveWorkloads: active,
		})
	})

	if err := r.Run(*listen); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(fmt.Errorf("run controller: %w", err))
	}
}

func deriveWorkloadStatus(jobs map[string]models.Job, workloadID string) string {
	pendingOrRunning := false
	for _, job := range jobs {
		if job.WorkloadID != workloadID {
			continue
		}
		if job.Status == "pending" || job.Status == "running" {
			pendingOrRunning = true
			break
		}
	}
	if pendingOrRunning {
		return "running"
	}
	return "completed"
}

func bindOptionalCreateWorkload(c *gin.Context) (transport.CreateWorkloadRequest, error) {
	var req transport.CreateWorkloadRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			return req, err
		}
	}
	if req.Filter == "" {
		req.Filter = "grayscale"
	}
	if req.WorkloadName == "" {
		req.WorkloadName = "workload-" + uuid.NewString()[:8]
	}
	return req, nil
}

func normalizeHTTPURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}
	trimmed = strings.TrimRight(trimmed, "/")
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return trimmed
	}
	return "http://" + trimmed
}

func (st *state) allocateImageID() string {
	id := strconv.FormatInt(st.nextImageID, 10)
	st.nextImageID++
	return id
}

func (st *state) allocateJobSequence() int64 {
	sequence := st.nextJobSequence
	st.nextJobSequence++
	return sequence
}

func sortedImages(images map[string]models.ImageRecord) []models.ImageRecord {
	out := make([]models.ImageRecord, 0, len(images))
	for _, img := range images {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool {
		left, leftErr := strconv.ParseInt(out[i].ID, 10, 64)
		right, rightErr := strconv.ParseInt(out[j].ID, 10, 64)
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func appendUnique(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}
