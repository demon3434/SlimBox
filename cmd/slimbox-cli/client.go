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
	"slimbox/internal/engine"
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

	if err := checkHTTPResponse(resp); err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	var task domain.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, fmt.Errorf("failed to decode upload response: %w", err)
	}

	return &task, nil
}

func checkHTTPResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	errMsg := string(body)

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("401 身份鉴权失败: 服务端已启用安全鉴权模式 (SLIMBOX_AUTH_ENABLED=true)。请在 Web 界面【设置】中创建或复制 API Key (访问密钥)，并使用 -k <key> (或 --key) 参数传入 (原始返回: %s)", errMsg)
	case http.StatusForbidden:
		return fmt.Errorf("403 权限不足: 当前 API Key 角色无权执行该管理操作 (原始返回: %s)", errMsg)
	default:
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errMsg)
	}
}

// StartTaskOptions contains optional overrides when queueing a task.
type StartTaskOptions struct {
	ProfileName    string
	VideoCodec     string
	CRF            int
	Preset         string
	AudioBitrate   string
	AudioCodec     string
	SubtitlePolicy string
}

// StartTask queues a task with given profile or codec override.
func (c *APIClient) StartTask(taskID, profileName, codec string) (*domain.Task, error) {
	return c.StartTaskWithOptions(taskID, StartTaskOptions{
		ProfileName: profileName,
		VideoCodec:  codec,
	})
}

// StartTaskWithOptions queues a task with full custom parameter overrides.
func (c *APIClient) StartTaskWithOptions(taskID string, opts StartTaskOptions) (*domain.Task, error) {
	payload := map[string]interface{}{
		"profile_name": opts.ProfileName,
	}
	customParams := make(map[string]interface{})
	if opts.VideoCodec != "" {
		customParams["video_codec"] = opts.VideoCodec
	}
	if opts.CRF > 0 {
		customParams["crf"] = opts.CRF
	}
	if opts.Preset != "" {
		customParams["preset"] = opts.Preset
	}
	if opts.AudioBitrate != "" {
		customParams["audio_bitrate"] = opts.AudioBitrate
	}
	if opts.AudioCodec != "" {
		customParams["audio_codec"] = opts.AudioCodec
	}
	if opts.SubtitlePolicy != "" {
		customParams["subtitle_policy"] = opts.SubtitlePolicy
	}
	if len(customParams) > 0 {
		payload["custom_params"] = customParams
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

	if err := checkHTTPResponse(resp); err != nil {
		return nil, fmt.Errorf("start task failed: %w", err)
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
	if err := checkHTTPResponse(resp); err != nil {
		return nil, fmt.Errorf("get task failed: %w", err)
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
	if err := checkHTTPResponse(resp); err != nil {
		return fmt.Errorf("abort task failed: %w", err)
	}
	return nil
}

// RetryTask re-queues an aborted or failed task.
func (c *APIClient) RetryTask(taskID string) error {
	req, err := c.newRequest("POST", fmt.Sprintf("/api/v1/tasks/%s/retry", taskID), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkHTTPResponse(resp); err != nil {
		return fmt.Errorf("retry task failed: %w", err)
	}
	return nil
}

// AdjustPriority moves a queued task's position ("top", "bottom", "up", "down").
func (c *APIClient) AdjustPriority(taskID, direction string) error {
	payload, _ := json.Marshal(map[string]string{"direction": direction})
	req, err := c.newRequest("POST", fmt.Sprintf("/api/v1/tasks/%s/priority", taskID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkHTTPResponse(resp); err != nil {
		return fmt.Errorf("adjust priority failed: %w", err)
	}
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

	if err := checkHTTPResponse(resp); err != nil {
		return fmt.Errorf("download failed: %w", err)
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
	if err := checkHTTPResponse(resp); err != nil {
		return fmt.Errorf("delete task failed: %w", err)
	}
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

	if err := checkHTTPResponse(resp); err != nil {
		return nil, fmt.Errorf("list tasks failed: %w", err)
	}

	var res struct {
		Tasks []*domain.Task `json:"tasks"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	return res.Tasks, nil
}

// ScanOrphans queries the server for orphan files and abandoned chunk upload sessions.
func (c *APIClient) ScanOrphans() (*engine.OrphanScanSummary, error) {
	req, err := c.newRequest("GET", "/api/v1/system/orphans/scan", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkHTTPResponse(resp); err != nil {
		return nil, fmt.Errorf("scan orphans failed: %w", err)
	}

	var res engine.OrphanScanSummary
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode scan response failed: %w", err)
	}
	return &res, nil
}

// CleanOrphans sends a request to purge orphan artifacts on the server.
func (c *APIClient) CleanOrphans(categories []string) (int, int64, error) {
	payload := map[string]interface{}{}
	if len(categories) > 0 {
		payload["categories"] = categories
	}
	data, _ := json.Marshal(payload)
	req, err := c.newRequest("POST", "/api/v1/system/orphans/clean", bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if err := checkHTTPResponse(resp); err != nil {
		return 0, 0, fmt.Errorf("clean orphans failed: %w", err)
	}

	var res struct {
		DeletedCount int   `json:"deleted_count"`
		FreedBytes   int64 `json:"freed_bytes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, 0, fmt.Errorf("decode clean response failed: %w", err)
	}
	return res.DeletedCount, res.FreedBytes, nil
}

