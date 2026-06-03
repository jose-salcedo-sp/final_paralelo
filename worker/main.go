package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/rpc"
	"runtime"
	"strings"
	"sync"
	"time"

	"final_paralelo/internal/filters"
	"final_paralelo/internal/transport"
)

type Worker struct {
	Name         string
	Controller   string
	APIEndpoint  string
	APIToken     string
	RunningJobs  int
	runningJobsM sync.Mutex
}

func main() {
	controller := flag.String("controller", "http://localhost:8090", "controller endpoint")
	workerName := flag.String("worker-name", "worker-1", "worker name")
	tagsFlag := flag.String("tags", "default", "comma separated tags")
	rpcListen := flag.String("rpc-listen", "127.0.0.1:0", "worker rpc listen address")
	flag.Parse()
	controllerURL := normalizeHTTPURL(*controller)

	tags := parseTags(*tagsFlag)
	ln, err := net.Listen("tcp", *rpcListen)
	if err != nil {
		panic(err)
	}

	w := &Worker{
		Name:       *workerName,
		Controller: controllerURL,
	}

	regResp, err := registerWorker(controllerURL, transport.RegisterWorkerRequest{
		Name:    *workerName,
		RPCAddr: ln.Addr().String(),
		Tags:    tags,
	})
	if err != nil {
		panic(err)
	}
	w.APIEndpoint = normalizeHTTPURL(regResp.APIEndpoint)
	w.APIToken = regResp.APIToken

	if err := rpc.RegisterName("Worker", w); err != nil {
		panic(err)
	}
	go rpc.Accept(ln)
	go w.sendHeartbeats()

	fmt.Printf("worker %s connected to controller %s; API %s; RPC listening on %s\n", *workerName, controllerURL, w.APIEndpoint, ln.Addr().String())
	select {}
}

func (w *Worker) ProcessJob(args transport.WorkerProcessArgs, reply *transport.WorkerProcessReply) error {
	w.incRunningJobs()
	defer w.decRunningJobs()

	input, err := w.downloadImage(args.OriginalImageID)
	if err != nil {
		reply.Error = err.Error()
		return nil
	}

	output, err := filters.Apply(args.Filter, input)
	if err != nil {
		reply.Error = err.Error()
		return nil
	}

	imageID, err := w.uploadFilteredImage(args.WorkloadID, args.OriginalImageID, output)
	if err != nil {
		reply.Error = err.Error()
		return nil
	}
	reply.FilteredImageID = imageID
	return nil
}

func (w *Worker) sendHeartbeats() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		_ = postJSON(w.Controller+"/workers/heartbeat", transport.WorkerHeartbeatRequest{
			Name:          w.Name,
			CPUPercent:    w.estimateCPU(),
			MemoryPercent: estimateMemoryPercent(),
			RunningJobs:   w.getRunningJobs(),
		}, nil)
	}
}

func (w *Worker) estimateCPU() float64 {
	return 8.0 + float64(w.getRunningJobs())*20.0
}

func estimateMemoryPercent() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	usedMB := float64(m.Alloc) / 1024.0 / 1024.0
	if usedMB > 95 {
		return 95
	}
	if usedMB < 1 {
		return 1
	}
	return usedMB
}

func (w *Worker) downloadImage(imageID string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, w.APIEndpoint+"/images/"+imageID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.APIToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: %s (%s)", resp.Status, string(body))
	}
	return io.ReadAll(resp.Body)
}

func (w *Worker) uploadFilteredImage(workloadID, sourceImageID string, content []byte) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	fileWriter, err := form.CreateFormFile("data", "filtered.png")
	if err != nil {
		return "", err
	}
	if _, err := fileWriter.Write(content); err != nil {
		return "", err
	}
	_ = form.WriteField("workload_id", workloadID)
	_ = form.WriteField("type", "filtered")
	_ = form.WriteField("source_image_id", sourceImageID)
	if err := form.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, w.APIEndpoint+"/images", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+w.APIToken)
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyData, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed: %s (%s)", resp.Status, string(bodyData))
	}

	var parsed struct {
		ImageID string `json:"image_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.ImageID == "" {
		return "", fmt.Errorf("missing image_id in upload response")
	}
	return parsed.ImageID, nil
}

func registerWorker(controller string, req transport.RegisterWorkerRequest) (transport.RegisterWorkerResponse, error) {
	var out transport.RegisterWorkerResponse
	err := postJSON(controller+"/workers/register", req, &out)
	return out, err
}

func parseTags(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"default"}
	}
	return out
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed: %s (%s)", resp.Status, string(body))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (w *Worker) incRunningJobs() {
	w.runningJobsM.Lock()
	defer w.runningJobsM.Unlock()
	w.RunningJobs++
}

func (w *Worker) decRunningJobs() {
	w.runningJobsM.Lock()
	defer w.runningJobsM.Unlock()
	if w.RunningJobs > 0 {
		w.RunningJobs--
	}
}

func (w *Worker) getRunningJobs() int {
	w.runningJobsM.Lock()
	defer w.runningJobsM.Unlock()
	return w.RunningJobs
}
