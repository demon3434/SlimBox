package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"slimbox/internal/domain"
)

type APIClient struct {
	serverURL string
	token     string
	http      *http.Client
}

func NewAPIClient(serverURL, token string) *APIClient {
	return &APIClient{
		serverURL: serverURL,
		token:     token,
		http: &http.Client{
			Timeout: 0, // No timeout for large uploads/downloads
		},
	}
}

func (c *APIClient) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := fmt.Sprintf("%s%s", c.serverURL, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// UploadVideo streams a local video file to the server.
func (c *APIClient) UploadVideo(localFilePath string, onProgress func(uploaded, total int64)) (*domain.Task, error) {
	file, err := os.Open(localFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}
	totalSize := fi.Size()

	// Use pipe for streaming memory efficiency
	pr, pw := io.Pipe()
	multiWriter := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		partWriter, err := multiWriter.CreateFormFile("file", filepath.Base(localFilePath))
		if err != nil {
			return
		}

		buf := make([]byte, 64*1024)
		var uploaded int64 = 0
		for {
			n, err := file.Read(buf)
			if n > 0 {
				_, _ = partWriter.Write(buf[:n])
				uploaded += int64(n)
				if onProgress != nil {
					onProgress(uploaded, totalSize)
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
		_ = multiWriter.Close()
	}()

	req, err := c.newRequest("POST", "/api/v1/upload", pr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", multiWriter.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var task domain.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, fmt.Errorf("failed to decode upload response: %w", err)
	}

	return &task, nil
}

// StartTask queues a task with given profile or codec override.
func (c *APIClient) StartTask(taskID, profileName, codec string) (*domain.Task, error) {
	payload := map[string]interface{}{
		"profile_name": profileName,
	}
	if codec != "" {
		payload["custom_params"] = map[string]interface{}{
			"video_codec": codec,
		}
	}

	data, _ := json.Marshal(payload)
	req, err := c.newRequest("POST", fmt.Sprintf("/api/v1/tasks/%s/start", taskID), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("start task failed (HTTP %d): %s", resp.StatusCode, string(b))
	}

	var task domain.Task
	_ = json.NewDecoder(resp.Body).Decode(&task)
	return &task, nil
}

// GetTask fetches the current state and progress of a task.
func (c *APIClient) GetTask(taskID string) (*domain.Task, error) {
	req, err := c.newRequest("GET", fmt.Sprintf("/api/v1/tasks/%s", taskID), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("task not found")
	}

	var task domain.Task
	_ = json.NewDecoder(resp.Body).Decode(&task)
	return &task, nil
}

// AbortTask cancels a running task.
func (c *APIClient) AbortTask(taskID string) error {
	req, err := c.newRequest("POST", fmt.Sprintf("/api/v1/tasks/%s/abort", taskID), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// DownloadOutput downloads the compressed file from server to local path.
func (c *APIClient) DownloadOutput(taskID, localDestPath string) error {
	req, err := c.newRequest("GET", fmt.Sprintf("/api/v1/tasks/%s/download", taskID), nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed (HTTP %d): %s", resp.StatusCode, string(b))
	}

	if err := os.MkdirAll(filepath.Dir(localDestPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(localDestPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// DeleteTask removes task record and temporary files on server.
func (c *APIClient) DeleteTask(taskID string) error {
	req, err := c.newRequest("DELETE", fmt.Sprintf("/api/v1/tasks/%s", taskID), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ListTasks gets all tasks from server.
func (c *APIClient) ListTasks() ([]*domain.Task, error) {
	req, err := c.newRequest("GET", "/api/v1/tasks", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Tasks []*domain.Task `json:"tasks"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	return res.Tasks, nil
}
