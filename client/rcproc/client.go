package rcproc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"github.com/TMS360/backend-pkg/middleware"
)

type client struct {
	baseURL  string
	provider string
	client   *http.Client
}

func NewClient(baseURL, provider string) Client {
	return &client{
		baseURL:  baseURL,
		provider: provider,
		client:   &http.Client{},
	}
}

func (c *client) SetAuthToken(ctx context.Context, req *http.Request) error {
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		return fmt.Errorf("failed to get actor from context: %w", err)
	}

	if actor.Token == nil {
		return fmt.Errorf("no auth token found in context")
	}

	req.Header.Set("Authorization", "Bearer "+*actor.Token)
	return nil
}

func (c *client) Process(ctx context.Context, fileUrl string) (*RCProcessingResponse, error) {
	reqBody := RCProcessingRequest{
		FileURL:  fileUrl,
		Provider: c.provider,
	}

	// 2. Marshal to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// 3. Create the Request
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/process", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	if err := c.SetAuthToken(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to set auth token: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rc processor request failed (async): %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// --- LOGGING ---
	fmt.Printf("RC Processor (async) Status: %s\n", resp.Status)
	fmt.Printf("RC Processor (async) Body: %s\n", string(bodyBytes))

	if resp.StatusCode > 300 {
		return nil, c.handleAPIError(resp.StatusCode, bodyBytes)
	}

	var rcResp RCProcessingResponse
	if err := json.Unmarshal(bodyBytes, &rcResp); err != nil {
		return nil, fmt.Errorf("failed to decode rc response (async): %w", err)
	}

	return &rcResp, nil
}

func (c *client) GetStatus(ctx context.Context, requestID string) (*RateConResponse, error) {
	// 1. Create the Request
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/status/"+requestID, nil)
	if err != nil {
		return nil, err
	}

	if err := c.SetAuthToken(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to set auth token: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rc processor status request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// --- LOGGING ---
	fmt.Printf("RC Processor get-status Status: %s\n", resp.Status)
	fmt.Printf("RC Processor get-status Body: %s\n", string(bodyBytes))

	if resp.StatusCode > 300 {
		return nil, c.handleAPIError(resp.StatusCode, bodyBytes)
	}

	var rcResp RCProcessingStatusResponse
	if err := json.Unmarshal(bodyBytes, &rcResp); err != nil {
		return nil, fmt.Errorf("failed to decode rc response: %w", err)
	}

	fmt.Println("rcResp", rcResp)

	if rcResp.Status != "completed" || rcResp.Data == nil {
		return nil, fmt.Errorf("processing not completed: status=%s, message=%s", rcResp.Status, rcResp.Message)
	}

	return rcResp.Data, nil
}

func (c *client) ProcessSync(ctx context.Context, file io.Reader, filename, contentType string) (*RateConResponse, error) {
	// 1. Prepare Multipart Request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	err := writer.WriteField("provider", c.provider)
	if err != nil {
		return nil, err
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", filename))
	h.Set("Content-Type", contentType)

	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err = io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}
	writer.Close() // Close to write boundary

	// 2. Send Request
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/process/sync", body)
	if err != nil {
		return nil, err
	}

	if err := c.SetAuthToken(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to set auth token: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rc processor request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// --- LOGGING ---
	fmt.Printf("RC Processor (sync) Status: %s\n", resp.Status)
	fmt.Printf("RC Processor (sync) Body: %s\n", string(bodyBytes))

	if resp.StatusCode > 300 {
		return nil, c.handleAPIError(resp.StatusCode, bodyBytes)
	}

	var rcResp RateConResponse
	if err := json.Unmarshal(bodyBytes, &rcResp); err != nil {
		return nil, fmt.Errorf("failed to decode rc response: %w", err)
	}

	return &rcResp, nil
}

func (c *client) handleAPIError(status int, bodyBytes []byte) error {
	switch status {
	case http.StatusUnprocessableEntity: // 422
		var errResp HTTPValidationError
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Detail != nil {
			return fmt.Errorf("422 status but decode failed (Body: %s)", string(bodyBytes))
		}
		if len(errResp.Detail) > 0 {
			return fmt.Errorf("validation error: %s", errResp.Detail[0].Message)
		}
		return fmt.Errorf("validation error: unknown details")

	case http.StatusBadRequest: // 400
		var badReq BadRequestError
		if err := json.Unmarshal(bodyBytes, &badReq); err != nil {
			return fmt.Errorf("400 status but decode failed (Body: %s)", string(bodyBytes))
		}
		return fmt.Errorf("bad request: %s", badReq.Detail)

	default:
		return fmt.Errorf("api returned unexpected status %d (Body: %s)", status, string(bodyBytes))
	}
}

// TODO: test with io.Pipe instead of buffering entire file in memory
//func (c *client) Process(ctx context.Context, file io.Reader, filename, contentType string) (*RateConResponse, error) {
//	// 1. Setup the Pipe
//	// 'pr' (Reader) will be passed to the HTTP Request (Main Thread)
//	// 'pw' (Writer) will be written to by the Multipart Writer (Goroutine)
//	pr, pw := io.Pipe()
//
//	// Create the multipart writer immediately so we can get the Boundary string
//	writer := multipart.NewWriter(pw)
//
//	// 2. Start Streaming in a Background Goroutine
//	go func() {
//		var err error
//		// Ensure the pipe is always closed.
//		// CloseWithError(err) tells the HTTP Client that the upload failed mid-stream.
//		defer func() {
//			if err != nil {
//				pw.CloseWithError(err)
//			} else {
//				pw.Close()
//			}
//		}()
//
//		// A. Write Simple Fields
//		if err = writer.WriteField("provider", c.provider); err != nil {
//			return
//		}
//
//		// B. Create File Part with Explicit Content-Type
//		// We use CreatePart (not CreateFormFile) to manually set headers
//		h := make(textproto.MIMEHeader)
//		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", filename))
//		h.Set("Content-Type", contentType) // e.g., "application/pdf"
//
//		part, err := writer.CreatePart(h)
//		if err != nil {
//			return
//		}
//
//		// C. Stream: Copy directly from Source (gRPC) -> Destination (HTTP Pipe)
//		// This line blocks until the HTTP client starts reading 'pr'
//		if _, err = io.Copy(part, file); err != nil {
//			return
//		}
//
//		// D. Close Multipart Writer explicitly to write the trailing boundary
//		// (e.g., "--boundary--") before closing the pipe.
//		err = writer.Close()
//	}()
//
//	// 3. Create and Send Request
//	// We use 'pr' as the Body. Go automatically uses "Transfer-Encoding: chunked"
//	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/process", pr)
//	if err != nil {
//		_ = pr.Close() // Clean up if request creation fails
//		return nil, err
//	}
//
//	// Set the Content-Type with the correct boundary from the writer
//	req.Header.Set("Content-Type", writer.FormDataContentType())
//
//	resp, err := c.client.Do(req)
//	if err != nil {
//		return nil, fmt.Errorf("rc processor request failed: %w", err)
//	}
//	defer resp.Body.Close()
//
//	bodyBytes, err := io.ReadAll(resp.Body)
//	if err != nil {
//		return nil, fmt.Errorf("failed to read response body: %w", err)
//	}
//
//	// --- LOGGING ---
//	fmt.Printf("RC Processor Status: %s\n", resp.Status)
//	fmt.Printf("RC Processor Body: %s\n", string(bodyBytes))
//	// ----------------
//
//	if resp.StatusCode != http.StatusOK {
//		return nil, fmt.Errorf("rc processor returned status: %d", resp.StatusCode)
//	}
//
//	// 4. Decode JSON from the Bytes
//	var rcResp RateConResponse
//	// We use json.Unmarshal here because we already have the bytes
//	if err := json.Unmarshal(bodyBytes, &rcResp); err != nil {
//		return nil, fmt.Errorf("failed to decode rc response: %w", err)
//	}
//
//	return &rcResp, nil
//}
