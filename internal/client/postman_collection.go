package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/darrenburns/posting/v3/internal/model"
	"gopkg.in/yaml.v3"
)

// PostmanCollection runs the selected saved request with its inherited Postman
// context. A private temporary copy adds a response callback using supported
// sandbox APIs; the source collection is never instrumented or rewritten.
type PostmanCollection struct {
	Binary, Root string
	Render       func(model.Request) ([]byte, error)
	cookies      postmanCookieJar
}

type collectionCapture struct {
	Code    int    `json:"code"`
	Status  string `json:"status"`
	Headers []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"headers"`
	Body   string       `json:"body"`
	Time   float64      `json:"time"`
	URL    string       `json:"url"`
	Method model.Method `json:"method"`
}

func (p *PostmanCollection) Send(ctx context.Context, call Call) (*model.Response, error) {
	if call.Request.File != "" && (!fs.ValidPath(call.Request.File) || !strings.HasSuffix(call.Request.File, ".request.yaml") || strings.Contains(call.Request.File, "\\")) {
		return nil, fmt.Errorf("select a saved Postman request before running")
	}
	root, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "herdr-postman-run-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	tmp, err = filepath.EvalSymlinks(tmp)
	if err != nil {
		return nil, err
	}
	copyRoot := filepath.Join(tmp, "collection")
	if err := copyPostmanCollection(root, copyRoot); err != nil {
		return nil, err
	}
	requestFile := call.Request.File
	if requestFile == "" {
		requestFile = "draft-" + rand.Text() + ".request.yaml"
	}
	file := filepath.Join(copyRoot, filepath.FromSlash(requestFile))
	var data []byte
	if p.Render != nil {
		data, err = p.Render(call.Request)
	} else if call.Request.File != "" {
		data, err = os.ReadFile(file)
	} else {
		return nil, fmt.Errorf("unsaved requests require a native document renderer")
	}
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document["$kind"] != "http-request" {
		return nil, fmt.Errorf("Postman response capture for %v is not implemented yet", document["$kind"])
	}
	if call.Request.File == "" {
		document["name"] = strings.TrimSuffix(requestFile, ".request.yaml")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := rand.Text()
	capturePath := "/capture/" + token
	captures := make(chan collectionCapture, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(capturePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var c collectionCapture
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case captures <- c:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	environmentPath := "/environment/" + token
	environments := make(chan map[string]string, 1)
	mux.HandleFunc(environmentPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		values := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case environments <- values:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	callback, _ := json.Marshal("http://" + listener.Addr().String() + capturePath)
	script := `pm.sendRequest({url:` + string(callback) + `,method:'POST',header:{'Content-Type':'application/json'},body:{mode:'raw',raw:JSON.stringify({code:pm.response.code,status:pm.response.status,headers:pm.response.headers.all(),body:Buffer.from(pm.response.stream).toString('base64'),time:pm.response.responseTime,url:pm.request.url.toString(),method:pm.request.method})}},function(){});` + "\n"
	environmentCallback, _ := json.Marshal("http://" + listener.Addr().String() + environmentPath)
	environmentScript := `pm.sendRequest({url:` + string(environmentCallback) + `,method:'POST',header:{'Content-Type':'application/json'},body:{mode:'raw',raw:JSON.stringify(Object.fromEntries(Object.entries(pm.environment.toObject()).map(([k,v])=>[k,v==null?'':String(v)])))}},function(){});` + "\n"
	scripts, _ := document["scripts"].([]any)
	attached := false
	for _, item := range scripts {
		if entry, ok := item.(map[string]any); ok && entry["type"] == "afterResponse" {
			code, _ := entry["code"].(string)
			entry["code"] = script + "try {\n" + code + "\n} finally {\n" + environmentScript + "}\n"
			attached = true
			break
		}
	}
	if !attached {
		scripts = append(scripts, map[string]any{"type": "afterResponse", "language": "text/javascript", "code": script + environmentScript})
	}
	document["scripts"] = scripts
	// Select by the schema-defined item ID so duplicate display names and
	// slashes cannot select another request. Only the private run copy changes.
	// The CLI matches owner-prefixed cloud IDs by their bare UUID.
	selector, _ := document["id"].(string)
	selector = bareItemID(selector)
	if selector == "" {
		selector = rand.Text()
	}
	document["id"] = selector
	data, err = yaml.Marshal(document)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		return nil, err
	}
	if err := checkPostmanItemID(copyRoot, file, selector); err != nil {
		return nil, err
	}
	bin := p.Binary
	if bin == "" {
		bin = "postman"
	}
	args := []string{"collection", "run", copyRoot, "-i", selector, "--working-dir", root, "--no-report-events"}
	{
		keys := make([]string, 0, len(call.Variables))
		for key := range call.Variables {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			values = append(values, map[string]any{"key": key, "value": call.Variables[key], "enabled": true})
		}
		data, err := yaml.Marshal(map[string]any{"name": "herdr-postman execution", "values": values})
		if err != nil {
			return nil, err
		}
		environmentFile := filepath.Join(tmp, "execution.environment.yaml")
		if err := os.WriteFile(environmentFile, data, 0600); err != nil {
			return nil, err
		}
		args = append(args, "--environment", environmentFile)
	}
	var startingCookies []byte
	cookieOutput := ""
	if call.Request.Options.AttachCookies {
		startingCookies = p.cookies.snapshot()
		if len(startingCookies) > 0 {
			input := filepath.Join(tmp, "cookies-input.json")
			if err := os.WriteFile(input, startingCookies, 0600); err != nil {
				return nil, err
			}
			args = append(args, "--cookie-jar", input)
		}
		cookieOutput = filepath.Join(tmp, "cookies-output.json")
		args = append(args, "--export-cookie-jar", cookieOutput)
	}
	command := exec.CommandContext(ctx, bin, args...)
	command.Dir = root
	command.WaitDelay = 5 * time.Second
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	runErr := command.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var captured collectionCapture
	select {
	case captured = <-captures:
	default:
		if runErr != nil {
			return nil, fmt.Errorf("Postman collection produced no response: %w\n\n%s", runErr, lastLines(output.String(), 20))
		}
		return nil, fmt.Errorf("Postman collection finished without a captured response")
	}
	body, err := base64.StdEncoding.DecodeString(captured.Body)
	if err != nil {
		return nil, fmt.Errorf("invalid captured response body: %w", err)
	}
	exitCode := 0
	if runErr != nil {
		if e, ok := runErr.(*exec.ExitError); ok {
			exitCode = e.ExitCode()
		} else {
			return nil, runErr
		}
	}
	runOutput := output.String()
	captureURL := "http://" + listener.Addr().String() + capturePath
	// CLI 1.82.0's reporter can label a successful pm.sendRequest callback
	// as errored. Keep its raw output, but distinguish our verified capture
	// from user requests and assertions.
	if strings.Contains(runOutput, "POST "+captureURL+" [errored]") {
		runOutput = "Internal response capture succeeded. Postman CLI reported an error for the internal capture callback; its raw output follows.\n\n" + runOutput
	}
	response := &model.Response{StatusCode: captured.Code, Reason: captured.Status, Body: body, Elapsed: time.Duration(captured.Time * float64(time.Millisecond)), ReceivedAt: time.Now(), URL: captured.URL, Method: captured.Method, PostmanRun: &model.PostmanRun{ExitCode: exitCode, Output: runOutput}}
	select {
	case values := <-environments:
		response.PostmanRun.Environment = values
	default:
	}
	for _, h := range captured.Headers {
		response.Headers = append(response.Headers, model.Header{Name: h.Key, Value: h.Value})
	}
	if cookieOutput != "" {
		exported, err := os.ReadFile(cookieOutput)
		if err != nil {
			return nil, fmt.Errorf("Postman cookie export unavailable: %w", err)
		}
		if err := p.cookies.merge(startingCookies, exported); err != nil {
			return nil, err
		}
	}
	postmanResponseCookies(response)
	return response, nil
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

var ownerPrefixedID = regexp.MustCompile(`^[0-9]+-([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

func bareItemID(id string) string {
	if match := ownerPrefixedID.FindStringSubmatch(id); match != nil {
		return match[1]
	}
	return id
}

func checkPostmanItemID(root, selected, id string) error {
	return filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || file == selected {
			return nil
		}
		if !strings.HasSuffix(file, ".request.yaml") && !(entry.Name() == "definition.yaml" && filepath.Base(filepath.Dir(file)) == ".resources") {
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var item struct {
			ID string `yaml:"id"`
		}
		if yaml.Unmarshal(data, &item) != nil {
			return fmt.Errorf("invalid Postman item document")
		}
		if bareItemID(item.ID) == id {
			return fmt.Errorf("duplicate Postman item ID; resolve duplicate IDs before sending")
		}
		return nil
	})
}

func copyPostmanCollection(source, target string) error {
	return filepath.WalkDir(source, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, file)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if rel != "." && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(target, rel), 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("collection contains a symbolic link; cannot safely snapshot %s", rel)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("collection contains a non-regular file: %s", rel)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(target, rel), data, 0600)
	})
}
