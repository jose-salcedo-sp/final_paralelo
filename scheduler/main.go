package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/rpc"
	"sort"
	"strings"
	"time"

	"final_paralelo/internal/models"
	"final_paralelo/internal/transport"
)

func main() {
	controller := flag.String("controller", "http://localhost:8090", "controller endpoint")
	poll := flag.Duration("poll", 2*time.Second, "poll interval")
	flag.Parse()
	controllerURL := normalizeHTTPURL(*controller)

	for {
		runSchedulingCycle(controllerURL)
		time.Sleep(*poll)
	}
}

func runSchedulingCycle(controller string) {
	workers, err := listWorkers(controller)
	if err != nil || len(workers) == 0 {
		return
	}
	jobs, err := listPendingJobs(controller)
	if err != nil || len(jobs) == 0 {
		return
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Sequence < jobs[j].Sequence
	})

	for _, job := range jobs {
		worker, ok := pickWorker(workers)
		if !ok {
			return
		}

		if err := assignJob(controller, job.ID, worker.Name); err != nil {
			continue
		}

		workers = bumpLocalWorker(workers, worker.Name)

		reply, err := processJobRPC(worker.RPCAddr, transport.WorkerProcessArgs{
			JobID:           job.ID,
			WorkloadID:      job.WorkloadID,
			OriginalImageID: job.OriginalImageID,
			Filter:          job.Filter,
		})
		if err != nil {
			_ = completeJob(controller, job.ID, transport.CompleteJobRequest{
				Error: fmt.Sprintf("rpc process error: %v", err),
			})
			continue
		}

		if reply.Error != "" {
			_ = completeJob(controller, job.ID, transport.CompleteJobRequest{
				Error: reply.Error,
			})
			continue
		}

		_ = completeJob(controller, job.ID, transport.CompleteJobRequest{
			FilteredImageID: reply.FilteredImageID,
		})
	}
}

func pickWorker(workers []models.Worker) (models.Worker, bool) {
	if len(workers) == 0 {
		return models.Worker{}, false
	}
	sort.Slice(workers, func(i, j int) bool {
		scoreI := float64(workers[i].RunningJobs)*5 + workers[i].CPUPercent + workers[i].MemoryPercent
		scoreJ := float64(workers[j].RunningJobs)*5 + workers[j].CPUPercent + workers[j].MemoryPercent
		return scoreI < scoreJ
	})
	return workers[0], true
}

func bumpLocalWorker(workers []models.Worker, workerName string) []models.Worker {
	for i := range workers {
		if workers[i].Name == workerName {
			workers[i].RunningJobs++
			break
		}
	}
	return workers
}

func processJobRPC(addr string, args transport.WorkerProcessArgs) (transport.WorkerProcessReply, error) {
	client, err := rpc.Dial("tcp", addr)
	if err != nil {
		return transport.WorkerProcessReply{}, err
	}
	defer client.Close()

	var reply transport.WorkerProcessReply
	if err := client.Call("Worker.ProcessJob", args, &reply); err != nil {
		return transport.WorkerProcessReply{}, err
	}
	return reply, nil
}

func listWorkers(controller string) ([]models.Worker, error) {
	var response transport.ListWorkersResponse
	if err := getJSON(controller+"/workers", &response); err != nil {
		return nil, err
	}
	return response.Workers, nil
}

func listPendingJobs(controller string) ([]models.Job, error) {
	var response transport.ListJobsResponse
	if err := getJSON(controller+"/jobs/pending", &response); err != nil {
		return nil, err
	}
	return response.Jobs, nil
}

func assignJob(controller, jobID, workerName string) error {
	return postJSON(controller+"/jobs/"+jobID+"/assign", transport.AssignJobRequest{
		WorkerName: workerName,
	}, nil)
}

func completeJob(controller, jobID string, payload transport.CompleteJobRequest) error {
	return postJSON(controller+"/jobs/"+jobID+"/complete", payload, nil)
}

func postJSON(url string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func getJSON(url string, out any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
