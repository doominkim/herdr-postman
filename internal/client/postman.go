package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/darrenburns/posting/v3/internal/model"
)

// PostmanRequest executes independent HTTP drafts using Postman's documented
// single-request JSON output. Native collection requests must retain their
// inherited scripts/auth through the collection runner instead.
type PostmanRequest struct{ Binary string }

func (p *PostmanRequest) Send(ctx context.Context, call Call) (*model.Response, error) {
	req := call.Request
	if req.Payload != nil || (req.Body.Type != "" && req.Body.Type != model.BodyNone && req.Body.Type != model.BodyRaw) || (req.Auth.Type != "" && req.Auth.Type != model.AuthNone) || req.Scripts != (model.Scripts{}) || req.NativeRevision != "" {
		return nil, fmt.Errorf("this request requires additional Postman execution mapping; nothing was sent")
	}
	req, err := model.Resolve(req, call.Lookup)
	if err != nil {
		return nil, err
	}
	bin := p.Binary
	if bin == "" {
		bin = "postman"
	}
	dir, err := os.MkdirTemp("", "herdr-postman-response-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "response.json")
	args := []string{"request", string(req.Method), req.URL, "--no-report-events", "--output", output}
	hasContentType := false
	for _, h := range req.Headers {
		if h.Enabled {
			args = append(args, "--header", h.Name+":"+h.Value)
			if strings.EqualFold(h.Name, "Content-Type") {
				hasContentType = true
			}
		}
	}
	if req.Body.Type == model.BodyRaw {
		// CLI 1.82 treats '-' literally and '@file' as a multipart upload,
		// despite its help describing raw stdin/file payloads. Pass raw text
		// as one argument without a shell to preserve the payload bytes.
		args = append(args, "--body", req.Body.Raw)
		if !hasContentType && req.Body.ContentType != "" {
			args = append(args, "--header", "Content-Type:"+req.Body.ContentType)
		}
	}
	if req.Options.TimeoutSeconds > 0 {
		args = append(args, "--timeout", strconv.FormatInt(int64(req.Options.TimeoutSeconds*1000), 10))
	}
	if !req.Options.VerifySSL {
		args = append(args, "--insecure")
	}
	if !req.Options.FollowRedirects {
		args = append(args, "--redirects-ignore")
	}
	command := exec.CommandContext(ctx, bin, args...)
	runErr := command.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := os.ReadFile(output)
	if err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("Postman request did not produce a response: %w", runErr)
		}
		return nil, fmt.Errorf("Postman response file was not produced")
	}
	var result struct {
		Request struct {
			URL    string       `json:"url"`
			Method model.Method `json:"method"`
		} `json:"request"`
		Response struct {
			Code   int    `json:"code"`
			Status string `json:"status"`
			Header []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"header"`
			Body         string  `json:"body"`
			ResponseTime float64 `json:"responseTime"`
		} `json:"response"`
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid Postman response JSON: %w", err)
	}
	if result.Response.Code == 0 {
		return nil, fmt.Errorf("Postman returned no HTTP response")
	}
	response := &model.Response{StatusCode: result.Response.Code, Reason: result.Response.Status, Body: []byte(result.Response.Body), Elapsed: time.Duration(result.Response.ResponseTime * float64(time.Millisecond)), ReceivedAt: result.Timestamp, URL: result.Request.URL, Method: result.Request.Method}
	for _, h := range result.Response.Header {
		response.Headers = append(response.Headers, model.Header{Name: h.Key, Value: h.Value})
	}
	postmanResponseCookies(response)
	return response, nil
}
