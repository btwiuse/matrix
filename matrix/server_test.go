package matrix_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/gearshell/inject-proxy/matrix"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startServer launches the cmd/matrix binary in mock mode over stdio.
func startServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	client := mcp.NewClient(&mcp.Implementation{Name: "matrix-replica-test", Version: "0.0.1"}, nil)
	transport := &mcp.CommandTransport{Command: exec.CommandContext(ctx, "go", "run", "../cmd/matrix", "-mode", "mock")}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestToolsList(t *testing.T) {
	session := startServer(t)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 22 {
		t.Fatalf("expected 22 tools, got %d", len(res.Tools))
	}

	// Verify every tool name matches the embedded real schema, and every
	// input schema is a JSON object (spec requirement).
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	specs, err := matrix.LoadSpecs()
	if err != nil {
		t.Fatalf("LoadSpecs: %v", err)
	}
	if len(specs) != 22 {
		t.Fatalf("expected 22 specs, got %d", len(specs))
	}
	for _, s := range specs {
		tool, ok := byName[s.Name]
		if !ok {
			t.Errorf("tool %q missing from tools/list", s.Name)
			continue
		}
		var schema map[string]any
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Errorf("tool %q: marshal schema: %v", s.Name, err)
			continue
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Errorf("tool %q: bad input schema: %v", s.Name, err)
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("tool %q: input schema type = %v, want object", s.Name, schema["type"])
		}
	}
}

// textOf extracts the text of the first text content item.
func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("no content in result: %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

func TestCallToolBatchWebSearch(t *testing.T) {
	session := startServer(t)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "batch_web_search",
		Arguments: map[string]any{
			"queries": []map[string]any{{"query": "hello world"}},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %+v", res)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(textOf(t, res)), &body); err != nil {
		t.Fatalf("output is not JSON: %v (raw: %s)", err, textOf(t, res))
	}
	if _, ok := body["results"]; !ok {
		t.Errorf("expected \"results\" key in output, got %v", body)
	}
}

func TestCallToolGetVoiceList(t *testing.T) {
	session := startServer(t)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_voice_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var body map[string]any
	text := textOf(t, res)
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("output is not JSON: %v (raw: %s)", err, text)
	}
	if _, ok := body["available_voices"]; !ok {
		t.Errorf("expected \"available_voices\" key, got %v", body)
	}
}

func TestCallToolRejectsInvalidInput(t *testing.T) {
	session := startServer(t)

	// deploy requires dist_dir; omitting it must produce a tool error, not a panic.
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "deploy",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError for missing required field, got %+v", res)
	}
}

func TestProxyHandlerForwardsToRealServer(t *testing.T) {
	// Requires the real matrix server to be reachable; skip otherwise.
	url := "http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message"
	token := "sbk_v1_AGQAMY7IGNPLZQPUQE7X5GJKPU_7B4KJGNO6F2UJOFQCFFOIT6V"
	h := matrix.NewProxyHandler(matrix.ProxyConfig{URL: url, Token: token, Source: "hermes"})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := h.GetVoiceList(ctx, &matrix.GetVoiceListRequest{})
	if err != nil {
		t.Skipf("real matrix server unreachable: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("proxy output is not JSON: %v", err)
	}
	if _, ok := body["available_voices"]; !ok {
		t.Errorf("expected available_voices from real server, got %v", body)
	}
}
