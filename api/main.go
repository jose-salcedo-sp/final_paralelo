package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"final_paralelo/internal/auth"
	"final_paralelo/internal/models"
	"final_paralelo/internal/transport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func main() {
	listen := flag.String("listen", ":8080", "api listen address")
	controller := flag.String("controller", "http://localhost:8090", "controller endpoint")
	imageRoot := flag.String("image-root", "./images", "image storage root")
	workerToken := flag.String("worker-token", "worker-secret-token", "worker auth token")
	flag.Parse()

	if err := os.MkdirAll(*imageRoot, 0o755); err != nil {
		panic(err)
	}

	tokens := auth.NewManager(map[string]string{
		*workerToken: "worker-system",
	})
	credentials := map[string]string{
		"user":  "password",
		"admin": "admin123",
	}

	r := gin.Default()

	r.POST("/login", func(c *gin.Context) {
		user, pass, ok := c.Request.BasicAuth()
		if !ok {
			c.JSON(http.StatusUnauthorized, transport.ErrorResponse{Error: "basic auth required"})
			return
		}
		expectedPass, exists := credentials[user]
		if !exists || expectedPass != pass {
			c.JSON(http.StatusUnauthorized, transport.ErrorResponse{Error: "invalid credentials"})
			return
		}
		token := tokens.IssueToken(user)
		c.JSON(http.StatusOK, transport.LoginResponse{
			User:  user,
			Token: token,
		})
	})

	authorized := r.Group("/")
	authorized.Use(authMiddleware(tokens))

	authorized.DELETE("/logout", func(c *gin.Context) {
		token := bearerFromHeader(c.GetHeader("Authorization"))
		if ok := tokens.Revoke(token); !ok {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: "token cannot be revoked"})
			return
		}
		c.JSON(http.StatusOK, transport.LogoutResponse{LogoutMessage: "logged out"})
	})

	authorized.GET("/status", func(c *gin.Context) {
		var status transport.ControllerStatusResponse
		if err := getJSON(*controller+"/status", &status); err != nil {
			c.JSON(http.StatusBadGateway, transport.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	})

	authorized.POST("/workloads", func(c *gin.Context) {
		var req transport.CreateWorkloadRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}
		var out models.Workload
		if err := postJSON(*controller+"/workloads", req, &out); err != nil {
			c.JSON(http.StatusBadGateway, transport.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"workload_id":      out.ID,
			"filter":           out.Filter,
			"workload_name":    out.Name,
			"status":           out.Status,
			"running_jobs":     out.RunningJobs,
			"filtered_images":  out.FilteredImages,
			"original_images":  out.OriginalImages,
			"created_at_unix":  time.Now().Unix(),
			"controller_state": "in-memory",
		})
	})

	authorized.GET("/workloads/:workload_id", func(c *gin.Context) {
		var out models.Workload
		if err := getJSON(*controller+"/workloads/"+c.Param("workload_id"), &out); err != nil {
			c.JSON(http.StatusBadGateway, transport.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"workload_id":     out.ID,
			"filter":          out.Filter,
			"workload_name":   out.Name,
			"status":          out.Status,
			"running_jobs":    out.RunningJobs,
			"filtered_images": out.FilteredImages,
		})
	})

	authorized.POST("/images", func(c *gin.Context) {
		workloadID := c.PostForm("workload_id")
		imgType := c.PostForm("type")
		sourceImageID := c.PostForm("source_image_id")
		if workloadID == "" || (imgType != "original" && imgType != "filtered") {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: "workload_id and valid type are required"})
			return
		}

		fileHeader, err := c.FormFile("data")
		if err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: "data file is required"})
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, transport.ErrorResponse{Error: err.Error()})
			return
		}
		defer file.Close()

		bytesData, err := io.ReadAll(file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, transport.ErrorResponse{Error: err.Error()})
			return
		}

		extension := strings.ToLower(filepath.Ext(fileHeader.Filename))
		if extension == "" {
			extension = ".png"
		}
		filename := uuid.NewString() + extension
		localDir := filepath.Join(*imageRoot, workloadID)
		if err := os.MkdirAll(localDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, transport.ErrorResponse{Error: err.Error()})
			return
		}
		localPath := filepath.Join(localDir, filename)
		if err := os.WriteFile(localPath, bytesData, 0o644); err != nil {
			c.JSON(http.StatusInternalServerError, transport.ErrorResponse{Error: err.Error()})
			return
		}

		var register transport.RegisterImageResponse
		err = postJSON(*controller+"/images/register", transport.RegisterImageRequest{
			WorkloadID:    workloadID,
			Type:          imgType,
			Path:          localPath,
			SourceImageID: sourceImageID,
		}, &register)
		if err != nil {
			c.JSON(http.StatusBadGateway, transport.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"workload_id": workloadID,
			"image_id":    register.ImageID,
			"type":        imgType,
		})
	})

	authorized.GET("/images/:image_id", func(c *gin.Context) {
		var img models.ImageRecord
		if err := getJSON(*controller+"/images/"+c.Param("image_id"), &img); err != nil {
			c.JSON(http.StatusNotFound, transport.ErrorResponse{Error: err.Error()})
			return
		}
		c.FileAttachment(img.Path, filepath.Base(img.Path))
	})

	if err := r.Run(*listen); err != nil {
		panic(err)
	}
}

func authMiddleware(tokens *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerFromHeader(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, transport.ErrorResponse{Error: "missing bearer token"})
			return
		}
		if _, ok := tokens.Validate(token); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, transport.ErrorResponse{Error: "invalid token"})
			return
		}
		c.Next()
	}
}

func bearerFromHeader(authHeader string) string {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
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
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed: %s (%s)", resp.Status, string(b))
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
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed: %s (%s)", resp.Status, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func _unusedMultipartMarker(_ *multipart.Writer) {}
