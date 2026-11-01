package cloud

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudSavePatchesOnlyEditedFieldsAndReadsBack(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "saved")
	t.Setenv("CLOUD_TEST_STATE", state)
	bin := fixtureCLI(t, `case "$*" in
 'collection request get r1 --collection c1 --workspace w1 --json')
  if [ -f "$CLOUD_TEST_STATE" ]; then
   printf '%s' '{"id":"r1","$kind":"http-request","name":"Original","url":"https://example.test/new","scripts":[{"type":"afterResponse","code":"keep-me"}]}'
  else
   printf '%s' '{"id":"r1","$kind":"http-request","name":"Original","url":"https://example.test/old","scripts":[{"type":"afterResponse","code":"keep-me"}]}'
  fi ;;
 'collection request update r1 --collection c1 --workspace w1 --type http --json --url https://example.test/new')
  touch "$CLOUD_TEST_STATE"; printf '{}' ;;
 *) exit 3 ;;
esac
`)
	cli, target := CLI{Binary: bin}, Target{Workspace: "w1", Collection: "c1", Request: "r1"}
	baseline, err := cli.Request(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	draft := append(json.RawMessage(nil), baseline...)
	draft = []byte(strings.Replace(string(draft), "/old", "/new", 1))
	saved, err := cli.SaveRequest(context.Background(), target, baseline, draft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "/new") || !strings.Contains(string(saved), "keep-me") {
		t.Fatal("cloud read-back lost changes or untouched fields")
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatal("cloud update was not called")
	}
}

func TestCloudSaveRejectsGUIConflictWithoutUpdating(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "unexpected-write")
	t.Setenv("CLOUD_TEST_STATE", state)
	bin := fixtureCLI(t, `if [ "$3" = get ]; then
 printf '%s' '{"id":"r1","$kind":"http-request","url":"https://example.test/gui"}'
else
 touch "$CLOUD_TEST_STATE"; printf '{}'
fi
`)
	baseline := json.RawMessage(`{"id":"r1","$kind":"http-request","url":"https://example.test/old"}`)
	draft := json.RawMessage(`{"id":"r1","$kind":"http-request","url":"https://example.test/draft"}`)
	_, err := (CLI{Binary: bin}).SaveRequest(context.Background(), Target{Workspace: "w1", Collection: "c1", Request: "r1"}, baseline, draft)
	if err == nil || !strings.Contains(err.Error(), "changed in Postman Cloud") {
		t.Fatalf("expected conflict: %v", err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("conflict wrote cloud data")
	}
}

func TestCloudSaveSendsJSONBodyOnStdin(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "body")
	t.Setenv("CLOUD_TEST_STATE", state)
	bin := fixtureCLI(t, `case "$*" in
 'collection request get r1 --collection c1 --workspace w1 --json')
  if [ -f "$CLOUD_TEST_STATE" ]; then
   printf '%s' '{"id":"r1","$kind":"http-request","body":{"type":"json","content":"{\"fixture\":\"new\"}"}}'
  else
   printf '%s' '{"id":"r1","$kind":"http-request","body":{"type":"json","content":"{}"}}'
  fi ;;
 'collection request update r1 --collection c1 --workspace w1 --type http --json --body json:-') cat > "$CLOUD_TEST_STATE"; printf '{}' ;;
 *) exit 3 ;;
esac
`)
	baseline := json.RawMessage(`{"id":"r1","$kind":"http-request","body":{"type":"json","content":"{}"}}`)
	draft := json.RawMessage(`{"id":"r1","$kind":"http-request","body":{"type":"json","content":"{\"fixture\":\"new\"}"}}`)
	_, err := (CLI{Binary: bin}).SaveRequest(context.Background(), Target{Workspace: "w1", Collection: "c1", Request: "r1"}, baseline, draft)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(state)
	if err != nil || string(data) != `{"fixture":"new"}` {
		t.Fatal("body not passed exactly through stdin")
	}
}

func TestCloudSaveRejectsUnsupportedBodyWithoutCallingCLI(t *testing.T) {
	for _, body := range []string{
		`{"type":"json","content":"{}","extra":"preserve"}`,
		`{"type":"file","content":"private.bin"}`,
		`{"type":"json","content":null}`,
		`null`,
	} {
		t.Run(body, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "called")
			t.Setenv("CLOUD_TEST_MARKER", marker)
			bin := fixtureCLI(t, `touch "$CLOUD_TEST_MARKER"; exit 1`)
			baseline := json.RawMessage(`{"id":"r1","$kind":"http-request"}`)
			draft := json.RawMessage(`{"id":"r1","$kind":"http-request","body":` + body + `}`)
			_, err := (CLI{Binary: bin}).SaveRequest(context.Background(), Target{Workspace: "w1", Collection: "c1", Request: "r1"}, baseline, draft)
			if err == nil {
				t.Fatal("unsupported body accepted")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("unsupported body invoked CLI")
			}
		})
	}
}

func TestCloudSaveDoesNotReportSuccessWhenReadBackDiffers(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "updated")
	t.Setenv("CLOUD_TEST_MARKER", marker)
	bin := fixtureCLI(t, `if [ "$3" = get ]; then
 printf '%s' '{"id":"r1","$kind":"http-request","url":"https://example.test/old"}'
else
 touch "$CLOUD_TEST_MARKER"; printf '{}'
fi`)
	baseline := json.RawMessage(`{"id":"r1","$kind":"http-request","url":"https://example.test/old"}`)
	draft := json.RawMessage(`{"id":"r1","$kind":"http-request","url":"https://example.test/new"}`)
	_, err := (CLI{Binary: bin}).SaveRequest(context.Background(), Target{Workspace: "w1", Collection: "c1", Request: "r1"}, baseline, draft)
	if err == nil || !strings.Contains(err.Error(), "different values") {
		t.Fatalf("expected read-back mismatch: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("update was not attempted")
	}
}

func TestCloudSaveUnchangedRequestDoesNotUpdate(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "unexpected-update")
	t.Setenv("CLOUD_TEST_MARKER", marker)
	bin := fixtureCLI(t, `if [ "$3" = get ]; then
 printf '%s' '{"id":"r1","$kind":"http-request","url":"https://example.test/old"}'
else
 touch "$CLOUD_TEST_MARKER"; printf '{}'
fi`)
	baseline := json.RawMessage(`{"id":"r1","$kind":"http-request","url":"https://example.test/old"}`)
	_, err := (CLI{Binary: bin}).SaveRequest(context.Background(), Target{Workspace: "w1", Collection: "c1", Request: "r1"}, baseline, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("unchanged request was updated")
	}
}
