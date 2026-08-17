package matrix

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rewriteTest invokes rewriteEnvelope with a canned envelope and returns
// the parsed rewritten envelope.
func rewriteTest(t *testing.T, body, tool string) (map[string]any, string, bool) {
	t.Helper()
	out, changed := rewriteEnvelope([]byte(body), tool)
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("rewritten envelope is not JSON: %v (%s)", err, out)
	}
	return env, string(out), changed
}

func parseResult(t *testing.T, body string) (map[string]any, string) {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("envelope is not JSON: %v (%s)", err, body)
	}
	result, _ := env["result"].(map[string]any)
	return result, body
}

func TestEnvelopeAddsDisplayDataAndIsError(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"website_id\": 431840818266354, \"website_url\": \"http://abc.localhost\", \"screenshot_url\": \"\"}"}]}}`
	env, out, changed := rewriteTest(t, body, "deploy")
	if !changed {
		t.Fatal("deploy success envelope must be rewritten")
	}
	result, _ := env["result"].(map[string]any)
	if result == nil {
		t.Fatalf("no result in %s", out)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Errorf("isError = true, want explicit false: %s", out)
	}
	display, _ := result["display_data"].(map[string]any)
	if display == nil {
		t.Fatalf("missing display_data: %s", out)
	}
	if id, ok := display["website_id"].(float64); !ok || int64(id) != 431840818266354 {
		t.Errorf("display_data.website_id = %v, want 431840818266354", display["website_id"])
	}
	if u, _ := display["website_url"].(string); u != "http://abc.localhost" {
		t.Errorf("display_data.website_url = %v", display["website_url"])
	}
}

func TestEnvelopeSoftErrorGetsExplicitIsErrorFalse(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"error\": \"dist directory does not exist\", \"message\": \"...\"}"}]}}`
	env, out, changed := rewriteTest(t, body, "deploy")
	if !changed {
		t.Fatal("soft error envelope must be rewritten")
	}
	result, _ := env["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Errorf("soft error isError = true, want false: %s", out)
	}
	if _, has := result["display_data"]; has {
		t.Errorf("soft error must not carry display_data: %s", out)
	}
}

func TestEnvelopeHardErrorKeepsIsErrorTrue(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"error\": \"dist_dir must be...\"}"}],"isError":true}}`
	// Nothing to add: isError is already explicit and the error text carries
	// no website_id, so the envelope passes through byte-identical.
	if _, changed := rewriteEnvelope([]byte(body), "deploy"); changed {
		t.Fatal("hard error envelope with explicit isError must pass through untouched")
	}
	// The isError=true flag survives the pass-through.
	if _, out, changed := rewriteTest(t, body, "deploy"); changed {
		t.Fatalf("unexpected rewrite: %s", out)
	} else if result, _ := parseResult(t, out); result != nil {
		if isErr, _ := result["isError"].(bool); !isErr {
			t.Errorf("isError = false, want true preserved: %s", out)
		}
	}
}

func TestEnvelopeLeavesOtherToolsAlone(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"available_voices\":[]}"}]}}`
	if _, out, changed := rewriteTest(t, body, "get_voice_list"); changed {
		t.Fatalf("non-deploy tool envelope must pass through untouched: %s", out)
	}
	// A non-JSON body (e.g. an error page) passes through untouched.
	if _, changed := rewriteEnvelope([]byte("<html>oops</html>"), "deploy"); changed {
		t.Fatalf("non-JSON body must pass through untouched")
	}
}

func TestEnvelopeRestoresDeployRequiredInToolsList(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"deploy","inputSchema":{"type":"object","properties":{}}},{"name":"gen_videos","inputSchema":{"type":"object","properties":{},"required":["video_requests"]}}]}}`
	env, out, changed := rewriteTest(t, body, "")
	if !changed {
		t.Fatal("tools/list with deploy must be rewritten")
	}
	tools, _ := env["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools list changed shape: %s", out)
	}
	deploy := tools[0].(map[string]any)
	schema := deploy["inputSchema"].(map[string]any)
	req, _ := schema["required"].([]any)
	if len(req) != 1 || req[0] != "dist_dir" {
		t.Errorf("deploy required = %v, want [dist_dir]: %s", schema["required"], out)
	}
	// Other tools keep their own required untouched.
	gen := tools[1].(map[string]any)["inputSchema"].(map[string]any)
	if req, _ := gen["required"].([]any); len(req) != 1 || req[0] != "video_requests" {
		t.Errorf("gen_videos required changed: %s", out)
	}
}

func TestEnvelopeRewriterOverSSE(t *testing.T) {
	body := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"website_id\\\": 1, \\\"website_url\\\": \\\"http://x.localhost\\\", \\\"screenshot_url\\\": \\\"\\\"}\"}]}}\n\n"
	out, changed := rewriteEnvelope([]byte(body), "deploy")
	if !changed {
		t.Fatal("SSE deploy envelope must be rewritten")
	}
	if !strings.HasPrefix(string(out), "event: message\ndata: ") || !strings.HasSuffix(string(out), "\n\n") {
		t.Fatalf("SSE framing lost: %s", out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(string(out), "event: message\ndata: "), "\n\n")), &env); err != nil {
		t.Fatalf("SSE payload is not JSON: %v (%s)", err, out)
	}
	result, _ := env["result"].(map[string]any)
	if _, ok := result["display_data"]; !ok {
		t.Errorf("SSE rewrite missing display_data: %s", out)
	}
}
func stubRPC(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	})
}

func TestEnvelopeRewriterServeHTTP(t *testing.T) {
	inner := stubRPC(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"website_id\": 7, \"website_url\": \"http://s.localhost\", \"screenshot_url\": \"\"}"}]}}`)
	srv := httptest.NewServer(NewEnvelopeRewriter(inner))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"deploy","arguments":{"dist_dir":"dist"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	got := make([]byte, 1<<20)
	n, _ := res.Body.Read(got)
	var env map[string]any
	if err := json.Unmarshal(got[:n], &env); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, got[:n])
	}
	result, _ := env["result"].(map[string]any)
	if _, ok := result["display_data"]; !ok {
		t.Errorf("ServeHTTP response missing display_data: %s", got[:n])
	}
	if res.Header.Get("Content-Length") == "" {
		t.Errorf("rewritten response must carry Content-Length")
	}
}
