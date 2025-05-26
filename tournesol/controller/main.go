package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicinformer "k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type diagnostic struct {
	name      string
	kind      string
	namespace string
	error     string
	solution  string
}

type aiPayload struct {
	Files    map[string]string `json:"files"`
	Solution string            `json:"solution"`
}

// ProcessedResources tracks which resources have been processed to avoid duplicates
var processedResources = make(map[string]bool)
var processedMutex sync.Mutex

// FileUpdate represents the format expected for file updates
type FileUpdate struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Configuration for GitHub and AI endpoint
var (
	githubOwner   = getEnvOrDefault("GITHUB_OWNER", "mfernd")
	githubRepo    = getEnvOrDefault("GITHUB_REPO", "prof-tournesol")
	githubBranch  = getEnvOrDefault("GITHUB_BRANCH", "main")
	githubToken   = getEnvOrDefault("GITHUB_TOKEN", "")
	aiBaseUrl     = getEnvOrDefault("AI_BASE_URL", "http://kubeai.kubeai.svc.cluster.local:80/openai/v1")
	aiEndpointUrl = aiBaseUrl + "/chat/completions"
	aiModel       = getEnvOrDefault("AI_MODEL", "gemma3-1b-cpu")
	aiTimeoutSecs = getEnvIntOrDefault("AI_TIMEOUT_SECONDS", 60)
	aiMaxRetries  = getEnvIntOrDefault("AI_MAX_RETRIES", 3)
	aiHealthCheck = getEnvBoolOrDefault("AI_HEALTH_CHECK", true)
	useFallback   = getEnvBoolOrDefault("USE_FALLBACK", true)
	httpClient    = &http.Client{Timeout: time.Duration(aiTimeoutSecs) * time.Second}
	githubApiUrl  = "https://api.github.com"
	ghServiceUrl  = getEnvOrDefault("GH_SERVICE_URL", "http://gh-service.tournesol:80")

	// Health check state
	endpointHealthy     = false
	endpointHealthMutex = &sync.Mutex{}
	lastHealthCheck     = time.Time{}
)

func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		if strings.ToLower(value) == "true" || value == "1" {
			return true
		} else if strings.ToLower(value) == "false" || value == "0" {
			return false
		}
	}
	return defaultValue
}

// handleResult processes Result information, fetches GitHub files, and sends to AI endpoint
func handleResult(obj *unstructured.Unstructured) {
	// Get unique identifier for this resource to avoid duplicate processing
	uid, _, _ := unstructured.NestedString(obj.Object, "metadata", "uid")
	name, _, _ := unstructured.NestedString(obj.Object, "metadata", "name")
	resourceID := uid
	if resourceID == "" {
		resourceID = name // Fallback to name if UID isn't available
	}

	// Check if we've already processed this resource
	processedMutex.Lock()
	if processed, exists := processedResources[resourceID]; exists && processed {
		processedMutex.Unlock()
		fmt.Printf("Skipping already processed resource: %s\n", resourceID)
		return
	}
	// Mark as being processed
	processedResources[resourceID] = true
	processedMutex.Unlock()

	var diag diagnostic
	// Get the resource name which might include namespace in format "namespace/name"
	resourceName, _, _ := unstructured.NestedString(obj.Object, "spec", "name")
	diag.kind, _, _ = unstructured.NestedString(obj.Object, "spec", "kind")

	// Parse namespace from the resource name (format: "namespace/name")
	if resourceName != "" && strings.Contains(resourceName, "/") {
		parts := strings.SplitN(resourceName, "/", 2)
		if len(parts) == 2 {
			diag.namespace = parts[0]
			diag.name = parts[1]
		} else {
			diag.name = resourceName
			diag.namespace = "default"
		}
	} else {
		diag.name = resourceName
		// Fallback to metadata namespace if resource name doesn't include namespace
		diag.namespace, _, _ = unstructured.NestedString(obj.Object, "metadata", "namespace")
		if diag.namespace == "" {
			diag.namespace = "default"
		}
	}

	details, _, _ := unstructured.NestedString(obj.Object, "spec", "details")
	parts := strings.SplitN(details, "\n\n", 2)
	if len(parts) >= 1 {
		diag.error = strings.TrimPrefix(parts[0], "Error: ")
	}
	if len(parts) >= 2 {
		diag.solution = strings.TrimPrefix(parts[1], "Solution: ")
	}

	// Print the diagnostic information
	fmt.Printf("[%s]\nKind: %s\nNamespace: %s\nError: %s\nSolution: %s\n",
		resourceName, diag.kind, diag.namespace, diag.error, diag.solution)

	// Process in the main thread to avoid concurrent requests
	// Use context with timeout for GitHub and AI API calls
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Extract the namespace from the Result resource
	namespace := diag.namespace
	fmt.Printf("Processing Result for namespace: %s\n", namespace)

	// Fetch files from GitHub
	files, err := fetchGitHubFiles(ctx, namespace)
	if err != nil {
		fmt.Printf("Error fetching GitHub files: %v\n", err)
		return
	}

	// If no files were found, log and return
	if len(files) == 0 {
		fmt.Printf("No files found in GitHub repository for namespace %s\n", namespace)
		return
	}

	// Send to AI endpoint
	fileUpdates, err := sendToAIEndpoint(ctx, files, diag.solution)
	if err != nil {
		fmt.Printf("Error sending to AI endpoint: %v\n", err)
		return
	}

	// Print the file updates that were returned
	fmt.Printf("Received %d file updates from analysis\n", len(fileUpdates))
	for _, update := range fileUpdates {
		fmt.Printf("- File: %s (%d bytes)\n", update.Path, len(update.Content))
	}

	// Send updates to gh-service to create a PR
	if len(fileUpdates) > 0 {
		err = sendToGHService(ctx, fileUpdates, diag)
		if err != nil {
			fmt.Printf("Error sending to gh-service: %v\n", err)
		} else {
			fmt.Printf("Successfully sent updates to gh-service for PR creation\n")
		}
	} else {
		fmt.Printf("No file updates to send to gh-service\n")
	}
}

// GitHub API response types
type GitHubContent struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int    `json:"size"`
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url"`
	GitURL      string `json:"git_url"`
	DownloadURL string `json:"download_url"`
	Content     string `json:"content"`
	Encoding    string `json:"encoding"`
}

// fetchGitHubFiles retrieves files from the GitHub repository for a specific namespace
func fetchGitHubFiles(ctx context.Context, namespace string) (map[string]string, error) {
	// Path in the repo to look for files - only check apps/<namespace>
	dirPath := fmt.Sprintf("apps/%s", namespace)
	fmt.Printf("Looking for files in GitHub path: %s\n", dirPath)

	// First, get the directory contents to find all files
	files, err := fetchDirectoryContents(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch files from %s: %w", dirPath, err)
	}

	// Log success if we found files
	if len(files) > 0 {
		fmt.Printf("Successfully found %d files in %s\n", len(files), dirPath)
	} else {
		fmt.Printf("No files found in %s\n", dirPath)
	}

	return files, nil
}

// fetchDirectoryContents uses GitHub API to get contents of a directory
func fetchDirectoryContents(ctx context.Context, dirPath string) (map[string]string, error) {
	files := make(map[string]string)

	// First get the directory listing
	contentsURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s",
		githubApiUrl, githubOwner, githubRepo, dirPath, githubBranch)

	fmt.Printf("Fetching directory contents from: %s\n", contentsURL)

	// Create request with GitHub API token if available
	req, err := http.NewRequestWithContext(ctx, "GET", contentsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if githubToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", githubToken))
	}

	// Send the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching directory contents: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limiting
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		resetTime := resp.Header.Get("X-RateLimit-Reset")
		return nil, fmt.Errorf("GitHub API rate limit exceeded. Rate limit resets at %s", resetTime)
	}

	// Handle 404 - directory doesn't exist
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("directory not found: %s", dirPath)
	}

	// Handle other errors
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse the response
	var contents []GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		// Try to decode as a single file instead of a directory
		resp.Body.Close()

		// If it's a file, fetch it directly
		return fetchSingleFile(ctx, dirPath)
	}

	// Process each item in the directory
	for _, item := range contents {
		// Skip directories, only process files
		if item.Type == "dir" {
			subDirFiles, err := fetchDirectoryContents(ctx, item.Path)
			if err != nil {
				fmt.Printf("Warning: Failed to fetch subdirectory %s: %v\n", item.Path, err)
				continue
			}

			// Add subdirectory files to main files map
			for name, content := range subDirFiles {
				// Create path relative to the original directory
				relativePath := filepath.Join(strings.TrimPrefix(item.Path, dirPath+"/"), name)
				files[relativePath] = content
			}
			continue
		}

		// For files, fetch the content
		fileContent, err := fetchFileContent(ctx, item.Path)
		if err != nil {
			fmt.Printf("Warning: Failed to fetch file %s: %v\n", item.Path, err)
			continue
		}

		// Add to files map with path relative to the requested directory
		relativePath := strings.TrimPrefix(item.Path, dirPath+"/")
		if relativePath == "" {
			relativePath = item.Name
		}

		files[relativePath] = fileContent
	}

	// Check if we found any files
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found at %s", dirPath)
	}

	return files, nil
}

// fetchSingleFile fetches a single file content if the path is a file, not a directory
func fetchSingleFile(ctx context.Context, filePath string) (map[string]string, error) {
	files := make(map[string]string)

	// Get file content URL
	contentURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s",
		githubApiUrl, githubOwner, githubRepo, filePath, githubBranch)

	fmt.Printf("Fetching file content from: %s\n", contentURL)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", contentURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if githubToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", githubToken))
	}

	// Send the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching file content: %w", err)
	}
	defer resp.Body.Close()

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse the response
	var content GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return nil, fmt.Errorf("error decoding file content: %w", err)
	}

	// Decode base64 content
	if content.Content != "" && content.Encoding == "base64" {
		decodedContent, err := base64.StdEncoding.DecodeString(
			strings.ReplaceAll(content.Content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("error decoding base64 content: %w", err)
		}

		files[filepath.Base(filePath)] = string(decodedContent)
		return files, nil
	}

	return nil, errors.New("file content not available")
}

// fetchFileContent fetches the content of a single file
func fetchFileContent(ctx context.Context, filePath string) (string, error) {
	// Get file content URL
	contentURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s",
		githubApiUrl, githubOwner, githubRepo, filePath, githubBranch)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", contentURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if githubToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", githubToken))
	}

	// Implement retry with backoff for rate limiting
	var resp *http.Response
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		// Send the request
		resp, err = httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("error fetching file content: %w", err)
		}

		// If rate limited, wait and retry
		if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			resp.Body.Close()

			// Parse the reset time and wait
			resetTimeStr := resp.Header.Get("X-RateLimit-Reset")
			if resetTimeStr != "" {
				resetTime, parseErr := strconv.ParseInt(resetTimeStr, 10, 64)
				if parseErr == nil {
					waitTime := time.Until(time.Unix(resetTime, 0))
					if waitTime > 0 && waitTime < 5*time.Minute {
						fmt.Printf("Rate limited. Waiting %s before retry\n", waitTime)
						time.Sleep(waitTime + time.Second)
						continue
					}
				}
			}

			// If we can't parse the reset time, use exponential backoff
			waitTime := time.Duration(1<<uint(i)) * time.Second
			fmt.Printf("Rate limited. Using exponential backoff: waiting %s before retry\n", waitTime)
			time.Sleep(waitTime)
			continue
		}

		// Break the loop if we got a response
		break
	}
	defer resp.Body.Close()

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse the response
	var content GitHubContent
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return "", fmt.Errorf("error decoding file content: %w", err)
	}

	// Decode base64 content
	if content.Content != "" && content.Encoding == "base64" {
		decodedContent, err := base64.StdEncoding.DecodeString(
			strings.ReplaceAll(content.Content, "\n", ""))
		if err != nil {
			return "", fmt.Errorf("error decoding base64 content: %w", err)
		}

		return string(decodedContent), nil
	}

	return "", errors.New("file content not available")
}

// sendToAIEndpoint sends the files and solution to the AI endpoint and returns file updates
func sendToAIEndpoint(ctx context.Context, files map[string]string, solution string) ([]FileUpdate, error) {
	fmt.Printf("Preparing to send data to AI endpoint: %s\n", aiEndpointUrl)
	fmt.Printf("Using model: %s with timeout %d seconds and max %d retries\n",
		aiModel, aiTimeoutSecs, aiMaxRetries)

	// Format content for AI processing with explicit instructions for the output format
	var content strings.Builder
	content.WriteString(fmt.Sprintf("# Problem Solution from K8SGPT\n\n%s\n\n", solution))
	content.WriteString("# Related Files\n\n")

	for filename, fileContent := range files {
		fmt.Printf("Including file in analysis: %s (%d bytes)\n", filename, len(fileContent))
		content.WriteString(fmt.Sprintf("## %s\n```yaml\n%s\n```\n\n", filename, fileContent))
	}

	// Add explicit instructions for response format
	content.WriteString(`
# Instructions for Response
Analyze the above error and solution recommendation along with the provided files.
If the error can be fixed by editing YAML files, please provide the updated file contents.

Respond ONLY with a JSON array of files to be updated, in this exact format:
[
  {
    "path": "relative/path/to/file.yaml",
    "content": "complete updated file content"
  }
]

If the error is in the application code itself or cannot be fixed by editing YAML files,
respond with an empty array: []
`)

	// Create payload for the AI endpoint
	payload := map[string]interface{}{
		"model": aiModel,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": content.String(),
			},
			{
				"role":    "system",
				"content": "You are an expert Kubernetes administrator. Respond only with the requested JSON format.",
			},
		},
		"temperature": 0.2, // Lower temperature for more deterministic responses
		"stream":      false,
		"max_tokens":  4096,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	// Check endpoint health if enabled
	if aiHealthCheck {
		healthy := checkAIEndpointHealth(ctx)
		if !healthy {
			fmt.Printf("⚠️ AI endpoint is not healthy, using local fallback response\n")
			return generateLocalFallbackResponse(solution, files), nil
		}
	}

	// Implement retry with exponential backoff
	var responseData map[string]interface{}

	for attempt := 0; attempt < aiMaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration: 1s, 2s, 4s, etc.
			backoffTime := time.Duration(1<<uint(attempt)) * time.Second
			if backoffTime > 30*time.Second {
				backoffTime = 30 * time.Second
			}

			fmt.Printf("Retry attempt %d/%d after waiting %v\n",
				attempt+1, aiMaxRetries, backoffTime)
			time.Sleep(backoffTime)
		}

		// Create a new timeout context for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(aiTimeoutSecs)*time.Second)
		defer cancel()

		// Try to call the AI endpoint
		resp, err := tryAIEndpoint(attemptCtx, jsonData)
		if err == nil {
			// Update endpoint health state on success
			updateEndpointHealth(true)
			responseData = resp
			break // Success!
		}

		fmt.Printf("Attempt %d/%d failed: %v\n", attempt+1, aiMaxRetries, err)

		// Don't retry on certain errors
		if strings.Contains(err.Error(), "received non-success status code") &&
			!strings.Contains(err.Error(), "429") && // Retry on 429 (Too Many Requests)
			!strings.Contains(err.Error(), "500") && // Retry on 500 (Internal Server Error)
			!strings.Contains(err.Error(), "502") && // Retry on 502 (Bad Gateway)
			!strings.Contains(err.Error(), "503") && // Retry on 503 (Service Unavailable)
			!strings.Contains(err.Error(), "504") { // Retry on 504 (Gateway Timeout)
			fmt.Printf("Not retrying due to non-retryable error\n")

			// Use fallback response after exhausting retries for this attempt
			fmt.Printf("⚠️ Using local fallback response after non-retryable error\n")
			return generateLocalFallbackResponse(solution, files), nil
		}
	}

	// If we got a response, try to extract the file updates
	if responseData != nil {
		fileUpdates, err := extractFileUpdatesFromResponse(responseData)
		if err != nil {
			fmt.Printf("Error extracting file updates from response: %v\n", err)
			return generateLocalFallbackResponse(solution, files), nil
		}
		return fileUpdates, nil
	}

	// All attempts failed, use fallback response
	fmt.Printf("⚠️ All %d attempts failed, using local fallback response\n", aiMaxRetries)
	return generateLocalFallbackResponse(solution, files), nil
}

// extractFileUpdatesFromResponse extracts file updates from AI response
func extractFileUpdatesFromResponse(responseData map[string]interface{}) ([]FileUpdate, error) {
	// Try to extract the content from the choices array
	choices, ok := responseData["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// Get the first choice
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	// Get the message
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	// Get the content
	content, ok := message["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	// Try to extract JSON content - find the first [ and last ]
	var jsonContent string
	startIdx := strings.Index(content, "[")
	endIdx := strings.LastIndex(content, "]")

	if startIdx >= 0 && endIdx > startIdx {
		jsonContent = content[startIdx : endIdx+1]
	} else {
		// If no brackets found, try the whole content
		jsonContent = content
	}

	// Try to parse the JSON content
	var fileUpdates []FileUpdate
	err := json.Unmarshal([]byte(jsonContent), &fileUpdates)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal file updates: %w", err)
	}

	return fileUpdates, nil
}

// tryAIEndpoint attempts to call the AI endpoint once
func tryAIEndpoint(ctx context.Context, jsonData []byte) (map[string]interface{}, error) {
	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", aiEndpointUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Skip connection test if we already know the endpoint is healthy
	endpointHealthMutex.Lock()
	isHealthy := endpointHealthy
	endpointHealthMutex.Unlock()

	if !isHealthy {
		// Attempt connection test first (HEAD request)
		testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		testReq, _ := http.NewRequestWithContext(testCtx, "HEAD", aiBaseUrl, nil)
		fmt.Printf("Testing connectivity to AI base URL: %s\n", aiBaseUrl)
		testResp, testErr := httpClient.Do(testReq)
		if testErr != nil {
			fmt.Printf("⚠️ Warning: Connectivity test failed: %v\n", testErr)
		} else {
			testResp.Body.Close()
			fmt.Printf("✅ Base URL connectivity test successful (status: %s)\n", testResp.Status)
		}
	}

	// Send the actual request
	fmt.Printf("Sending request to AI endpoint: %s\n", aiEndpointUrl)
	startTime := time.Now()
	resp, err := httpClient.Do(req)
	requestDuration := time.Since(startTime)

	if err != nil {
		// Update endpoint health state on failure
		updateEndpointHealth(false)

		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "timeout") ||
			strings.Contains(err.Error(), "deadline") {
			return nil, fmt.Errorf("request timed out after %v: %w", requestDuration, err)
		}
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Log response headers for debugging
	fmt.Printf("Response received in %v - status: %s\n", requestDuration, resp.Status)
	fmt.Printf("Response headers: Content-Type=%s, Content-Length=%s\n",
		resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length"))

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("received non-success status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Process the response
	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Log successful response
	fmt.Printf("Successfully received response from AI model %s\n", aiModel)

	// Extract and log a sample of the response content for debugging
	if choices, ok := responseData["choices"].([]interface{}); ok && len(choices) > 0 {
		if len(choices) > 0 {
			choice, ok := choices[0].(map[string]interface{})
			if ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						previewLen := 200
						if len(content) > previewLen {
							fmt.Printf("AI Response Preview: %s...\n", content[:previewLen])
						} else {
							fmt.Printf("AI Response: %s\n", content)
						}
					}
				}
			}
		}
	}

	return responseData, nil
}

// checkAIEndpointHealth checks if the AI endpoint is responsive
// Returns true if healthy, false if not
func checkAIEndpointHealth(ctx context.Context) bool {
	// Check if we've done a health check recently (within the last minute)
	endpointHealthMutex.Lock()
	defer endpointHealthMutex.Unlock()

	if !lastHealthCheck.IsZero() && time.Since(lastHealthCheck) < time.Minute {
		return endpointHealthy // Return cached health status if recent
	}

	// Create a shorter timeout context specifically for health check
	healthCheckCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Create a simple models list request to check if the API is responsive
	healthCheckUrl := aiBaseUrl + "/models"
	req, err := http.NewRequestWithContext(healthCheckCtx, "GET", healthCheckUrl, nil)
	if err != nil {
		fmt.Printf("❌ Health check failed to create request: %v\n", err)
		endpointHealthy = false
		lastHealthCheck = time.Now()
		return false
	}

	// Send the request
	fmt.Printf("🔍 Performing health check on %s\n", healthCheckUrl)
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Health check failed: %v\n", err)
		endpointHealthy = false
		lastHealthCheck = time.Now()
		return false
	}
	defer resp.Body.Close()

	// Check if response is successful
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("✅ AI endpoint health check passed (status: %s)\n", resp.Status)
		endpointHealthy = true
		lastHealthCheck = time.Now()
		return true
	}

	// Handle unsuccessful response
	fmt.Printf("❌ AI endpoint health check failed with status: %s\n", resp.Status)
	endpointHealthy = false
	lastHealthCheck = time.Now()
	return false
}

// updateEndpointHealth updates the cached health status of the endpoint
func updateEndpointHealth(healthy bool) {
	endpointHealthMutex.Lock()
	defer endpointHealthMutex.Unlock()

	endpointHealthy = healthy
	lastHealthCheck = time.Now()
}

// generateLocalFallbackResponse creates a local response when AI endpoint is unavailable
// Returns an array of FileUpdate objects in the expected format
func generateLocalFallbackResponse(solution string, files map[string]string) []FileUpdate {
	fmt.Printf("\n=== LOCAL RESPONSE (AI UNAVAILABLE) ===\n")
	fmt.Printf("Based on the provided information, generating fallback response\n")

	// Initialize the result
	var fileUpdates []FileUpdate

	// Check if this is an OOM issue
	isOOM := strings.Contains(strings.ToLower(solution), "oomkilled") ||
		strings.Contains(strings.ToLower(solution), "out of memory")

	if !isOOM {
		fmt.Printf("Not an OOM issue, returning empty updates\n")
		fmt.Printf("=== END LOCAL RESPONSE ===\n\n")
		return []FileUpdate{}
	}

	// Try to find deployment files with memory limits
	for name, content := range files {
		fmt.Printf("Processing file: %s\n", name)

		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			var deploymentName string
			var isDeployment bool
			var memoryLimit string

			// Try to extract deployment name
			if strings.Contains(content, "kind: Deployment") {
				isDeployment = true
				if idx := strings.Index(content, "name:"); idx >= 0 {
					subContent := content[idx+5:]
					endIdx := strings.Index(subContent, "\n")
					if endIdx > 0 {
						deploymentName = strings.TrimSpace(subContent[:endIdx])
					}
				}

				// Extract namespace if available (not used currently)
				if idx := strings.Index(content, "namespace:"); idx >= 0 {
					// We could extract namespace here if needed
					// Currently not used in our logic
				}

				// Check for memory limits
				if strings.Contains(content, "resources") {
					if idx := strings.Index(content, "limits:"); idx >= 0 {
						subContent := content[idx:]
						if memIdx := strings.Index(subContent, "memory:"); memIdx >= 0 {
							endIdx := strings.Index(subContent[memIdx+7:], "\n")
							if endIdx > 0 {
								memoryLimit = strings.TrimSpace(subContent[memIdx+7 : memIdx+7+endIdx])
							}
						}
					}
				}

				if isDeployment && deploymentName != "" {
					// If we found a deployment, try to update its memory limit
					updatedContent := content

					// Parse the current memory limit
					var currentMem int
					var unit string

					if memoryLimit != "" {
						// Parse values like "6Mi", "256Mi", "1Gi"
						numPart := ""
						unitPart := ""
						for i, c := range memoryLimit {
							if c >= '0' && c <= '9' {
								numPart += string(c)
							} else {
								unitPart = memoryLimit[i:]
								break
							}
						}

						if num, err := strconv.Atoi(numPart); err == nil {
							currentMem = num
							unit = unitPart
						}
					}

					// If we couldn't parse the limit or it's very small, set a reasonable default
					if currentMem == 0 || currentMem < 64 {
						// For very small values, increase substantially
						updatedContent = updateMemoryLimits(content, "256Mi")
						fmt.Printf("Updating memory limit to 256Mi\n")
					} else {
						// Increase by 50%
						newMem := int(float64(currentMem) * 1.5)
						updatedContent = updateMemoryLimits(content, fmt.Sprintf("%d%s", newMem, unit))
						fmt.Printf("Increasing memory limit from %s to %d%s\n", memoryLimit, newMem, unit)
					}

					// Add the updated file to our result
					if updatedContent != content {
						fileUpdate := FileUpdate{
							Path:    name,
							Content: updatedContent,
						}
						fileUpdates = append(fileUpdates, fileUpdate)
						fmt.Printf("Added file update for %s\n", name)
					}
				}
			}
		}
	}

	fmt.Printf("Generated %d file updates\n", len(fileUpdates))
	fmt.Printf("=== END LOCAL RESPONSE ===\n\n")

	return fileUpdates
}

// updateMemoryLimits updates the memory limits in a YAML file
func updateMemoryLimits(content string, newLimit string) string {
	lines := strings.Split(content, "\n")
	inResources := false
	inLimits := false

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Track if we're in the resources section
		if strings.HasPrefix(trimmedLine, "resources:") {
			inResources = true
			continue
		}

		// Check indent level to see if we're still in resources
		if inResources && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmedLine != "" {
			inResources = false
			inLimits = false
			continue
		}

		// Track if we're in the limits section
		if inResources && strings.HasPrefix(trimmedLine, "limits:") {
			inLimits = true
			continue
		}

		// Check indent level to see if we're still in limits
		if inLimits && strings.HasPrefix(trimmedLine, "requests:") {
			inLimits = false
			continue
		}

		// Update memory limit
		if inLimits && strings.HasPrefix(trimmedLine, "memory:") {
			indent := extractIndentation(line)
			lines[i] = indent + "memory: " + newLimit
		}
	}

	return strings.Join(lines, "\n")
}

// extractIndentation gets the whitespace prefix of a line
func extractIndentation(line string) string {
	indent := ""
	for _, c := range line {
		if c == ' ' || c == '\t' {
			indent += string(c)
		} else {
			break
		}
	}
	return indent
}

// sendToGHService sends file updates to the gh-service to create a pull request
func sendToGHService(ctx context.Context, fileUpdates []FileUpdate, diag diagnostic) error {
	fmt.Printf("Preparing to send updates to gh-service at %s\n", ghServiceUrl)

	// Create a title for the PR with emoji
	title := fmt.Sprintf("fix: Fixed %s issue in namespace %s", diag.name, diag.namespace)

	// Create PR body with diagnostic information
	body := fmt.Sprintf("This PR fixes an issue detected by K8sGPT for %s/%s in namespace %s. 🌻\n\n",
		diag.kind, diag.name, diag.namespace)
	body += fmt.Sprintf("**Error:** %s\n\n", diag.error)
	body += fmt.Sprintf("**Solution:** %s\n\n", diag.solution)
	body += "Changes were automatically generated by Prof Tournesol."

	// Create the files array in the format expected by gh-service
	// Map FileUpdate to the format expected by gh-service
	ghFiles := make([]map[string]string, len(fileUpdates))
	for i, update := range fileUpdates {
		// Ensure path includes apps/<namespace> prefix if not already present
		path := update.Path
		if !strings.HasPrefix(path, fmt.Sprintf("apps/%s/", diag.namespace)) {
			path = fmt.Sprintf("apps/%s/%s", diag.namespace, path)
		}
		
		ghFiles[i] = map[string]string{
			"path":    path,
			"content": update.Content,
		}
	}

	// Create the PR payload
	prPayload := map[string]interface{}{
		"owner": githubOwner,
		"repo":  githubRepo,
		"pr": map[string]interface{}{
			"title": title,
			"body":  body,
			"files": ghFiles,
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(prPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal PR payload: %w", err)
	}

	// Create request to gh-service
	reqUrl := fmt.Sprintf("%s/pull_requests", ghServiceUrl)
	req, err := http.NewRequestWithContext(ctx, "POST", reqUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to gh-service: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gh-service returned error status %d: %s", resp.StatusCode, respBody)
	}

	fmt.Printf("gh-service response: %s\n", respBody)
	return nil
}

func main() {
	// Log startup configuration
	fmt.Printf("Prof Tournesol controller starting with configuration:\n")
	fmt.Printf("- GitHub Repository: %s/%s (branch: %s)\n", githubOwner, githubRepo, githubBranch)
	fmt.Printf("- GitHub Path: apps/<namespace> using GitHub API\n")
	fmt.Printf("- AI Base URL: %s\n", aiBaseUrl)
	fmt.Printf("- AI Endpoint: %s\n", aiEndpointUrl)
	fmt.Printf("- AI Model: %s\n", aiModel)
	fmt.Printf("- AI Request Timeout: %d seconds\n", aiTimeoutSecs)
	fmt.Printf("- AI Max Retries: %d\n", aiMaxRetries)
	fmt.Printf("- AI Health Check Enabled: %v\n", aiHealthCheck)
	fmt.Printf("- Local Fallback Enabled: %v\n", useFallback)

	// Log the GH service URL configuration
	fmt.Printf("- GH Service URL: %s\n", ghServiceUrl)

	// Use in-cluster config
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		fmt.Printf("Error building kubeconfig: %v\n", err)
		os.Exit(1)
	}

	// Create dynamic client for CRDs
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating dynamic client: %v\n", err)
		os.Exit(1)
	}

	// Define the GVR for Result resources
	gvr := schema.GroupVersionResource{
		Group:    "core.k8sgpt.ai",
		Version:  "v1alpha1",
		Resource: "results",
	}

	// Create dynamic informer factory
	namespace := "k8sgpt-operator-system"
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		time.Minute*30,
		namespace,
		nil,
	)

	// Get informer for Result resources
	informer := factory.ForResource(gvr).Informer()

	// Set up event handlers
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			handleResult(obj.(*unstructured.Unstructured))
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			// Compare resource versions to avoid duplicate processing
			oldVersion, _, _ := unstructured.NestedString(oldObj.(*unstructured.Unstructured).Object, "metadata", "resourceVersion")
			newVersion, _, _ := unstructured.NestedString(newObj.(*unstructured.Unstructured).Object, "metadata", "resourceVersion")

			if oldVersion != newVersion {
				handleResult(newObj.(*unstructured.Unstructured))
			} else {
				fmt.Printf("Skipping unchanged resource version: %s\n", newVersion)
			}
		},
	})

	// Set up signal handling
	stopCh := make(chan struct{})
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// Setup for signal handling

	go func() {
		<-signalCh
		fmt.Println("Shutting down Prof Tournesol controller...")
		close(stopCh)
	}()

	// Listen for USR1 signal to test AI endpoint
	go func() {
		usr1Ch := make(chan os.Signal, 1)
		signal.Notify(usr1Ch, syscall.SIGUSR1)

		for {
			<-usr1Ch
			fmt.Println("Received USR1 signal - testing AI endpoint...")
			testCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			healthy := checkAIEndpointHealth(testCtx)
			cancel()

			if healthy {
				fmt.Println("✅ AI endpoint test succeeded")
			} else {
				fmt.Println("❌ AI endpoint test failed")
			}
		}
	}()

	// Perform initial health check if enabled
	if aiHealthCheck {
		initialCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		healthy := checkAIEndpointHealth(initialCtx)
		cancel()

		if healthy {
			fmt.Printf("✅ Initial AI endpoint health check passed\n")
		} else {
			fmt.Printf("⚠️ Initial AI endpoint health check failed - will use local fallback if needed\n")
			fmt.Printf("💡 TIP: Send SIGUSR1 signal to test AI endpoint: kill -USR1 <pid>\n")
		}
	}

	fmt.Printf("Prof Tournesol controller started - watching namespace '%s'\n", namespace)

	// Start the informer and wait
	go informer.Run(stopCh)

	// Wait for the stop signal
	<-stopCh
}
