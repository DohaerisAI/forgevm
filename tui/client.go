package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type apiClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newAPIClient(baseURL, apiKey string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

type sandboxData struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	Provider  string    `json:"provider"`
	Image     string    `json:"image"`
	MemoryMB  int       `json:"memory_mb"`
	VCPUs     int       `json:"vcpus"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type execResultData struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration"`
}

type healthData struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

type providerData struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Healthy   bool   `json:"healthy"`
}

type templateData struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	Description string `json:"description"`
	MemoryMB    int    `json:"memory_mb"`
	CPUCores    int    `json:"cpu_cores"`
	TTLSeconds  int    `json:"ttl_seconds"`
	PoolSize    int    `json:"pool_size"`
}

func (c *apiClient) do(method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

func (c *apiClient) health() (*healthData, error) {
	data, _, err := c.do("GET", "/api/v1/health", nil)
	if err != nil {
		return nil, err
	}
	var h healthData
	json.Unmarshal(data, &h)
	return &h, nil
}

func (c *apiClient) listSandboxes() ([]sandboxData, error) {
	data, _, err := c.do("GET", "/api/v1/sandboxes", nil)
	if err != nil {
		return nil, err
	}
	var sbs []sandboxData
	json.Unmarshal(data, &sbs)
	return sbs, nil
}

func (c *apiClient) spawn(image, ttl string) (*sandboxData, error) {
	body := map[string]string{"image": image}
	if ttl != "" {
		body["ttl"] = ttl
	}
	data, code, err := c.do("POST", "/api/v1/sandboxes", body)
	if err != nil {
		return nil, err
	}
	if code != 201 {
		return nil, fmt.Errorf("spawn failed: %s", string(data))
	}
	var sb sandboxData
	json.Unmarshal(data, &sb)
	return &sb, nil
}

func (c *apiClient) spawnTemplate(name string) (*sandboxData, error) {
	data, code, err := c.do("POST", "/api/v1/templates/"+name+"/spawn", nil)
	if err != nil {
		return nil, err
	}
	if code != 201 {
		return nil, fmt.Errorf("spawn from template failed: %s", string(data))
	}
	var sb sandboxData
	json.Unmarshal(data, &sb)
	return &sb, nil
}

func (c *apiClient) destroy(id string) error {
	_, code, err := c.do("DELETE", "/api/v1/sandboxes/"+id, nil)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("destroy failed (HTTP %d)", code)
	}
	return nil
}

func (c *apiClient) exec(id, command string) (*execResultData, error) {
	data, code, err := c.do("POST", "/api/v1/sandboxes/"+id+"/exec", map[string]string{"command": command})
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("exec failed: %s", string(data))
	}
	var r execResultData
	json.Unmarshal(data, &r)
	return &r, nil
}

func (c *apiClient) writeFile(id, path, content string) error {
	_, code, err := c.do("POST", "/api/v1/sandboxes/"+id+"/files", map[string]string{
		"path": path, "content": content,
	})
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("write failed (HTTP %d)", code)
	}
	return nil
}

func (c *apiClient) readFile(id, path string) (string, error) {
	req, _ := http.NewRequest("GET", c.baseURL+"/api/v1/sandboxes/"+id+"/files?path="+path, nil)
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("read failed: %s", string(data))
	}
	return string(data), nil
}

func (c *apiClient) listProviders() ([]providerData, error) {
	data, _, err := c.do("GET", "/api/v1/providers", nil)
	if err != nil {
		return nil, err
	}
	var providers []providerData
	json.Unmarshal(data, &providers)
	return providers, nil
}

func (c *apiClient) listTemplates() ([]templateData, error) {
	data, _, err := c.do("GET", "/api/v1/templates", nil)
	if err != nil {
		return nil, err
	}
	var templates []templateData
	json.Unmarshal(data, &templates)
	return templates, nil
}

func (c *apiClient) createTemplate(name, image, desc, memStr, cpuStr, ttlStr string) error {
	mem := 512
	if memStr != "" {
		if v, err := strconv.Atoi(memStr); err == nil {
			mem = v
		}
	}
	cpus := 1
	if cpuStr != "" {
		if v, err := strconv.Atoi(cpuStr); err == nil {
			cpus = v
		}
	}
	ttl := 300
	if ttlStr != "" {
		if v, err := strconv.Atoi(ttlStr); err == nil {
			ttl = v
		}
	}

	body := map[string]interface{}{
		"name":        name,
		"image":       image,
		"description": desc,
		"memory_mb":   mem,
		"cpu_cores":   cpus,
		"ttl_seconds": ttl,
	}

	_, code, err := c.do("POST", "/api/v1/templates", body)
	if err != nil {
		return err
	}
	if code != 201 {
		return fmt.Errorf("create template failed (HTTP %d)", code)
	}
	return nil
}

func (c *apiClient) deleteTemplate(name string) error {
	_, code, err := c.do("DELETE", "/api/v1/templates/"+name, nil)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("delete template failed (HTTP %d)", code)
	}
	return nil
}
