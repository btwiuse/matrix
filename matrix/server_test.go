package matrix_test

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"strings"
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
	transport := &mcp.CommandTransport{Command: exec.CommandContext(ctx, "go", "run", "../cmd/matrix", "--mode", "mock")}
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

// TestCallToolAllTools exercises the call path of every one of the 22 tools
// with minimal valid arguments, asserting the mock response is JSON and
// carries the expected top-level key. This closes the gap where only 3 tools
// had call-level coverage: argument parsing bugs in the remaining 19 would
// otherwise go undetected.
func TestCallToolAllTools(t *testing.T) {
	session := startServer(t)

	cases := []struct {
		name     string
		args     map[string]any
		key      string // expected JSON top-level key, when output is JSON
		contains string // expected substring, when output is not JSON
	}{
		{"image_synthesize", map[string]any{
			"requests": []map[string]any{{"prompt": "a cat", "output_file": "cat.png"}},
		}, "output_files", ""},
		{"gen_videos", map[string]any{
			"video_requests": []map[string]any{{"prompt": "a cat runs", "output_file": "cat.mp4"}},
		}, "output_files", ""},
		{"batch_text_to_video", map[string]any{
			"count": 1, "prompt_list": []string{"a cat"}, "output_file_list": []string{"cat.mp4"},
		}, "output_files", ""},
		{"batch_image_to_video", map[string]any{
			"count": 1, "image_file_list": []string{"cat.png"}, "output_file_list": []string{"cat.mp4"},
		}, "output_files", ""},
		{"get_voice_list", map[string]any{}, "available_voices", ""},
		{"batch_text_to_audio", map[string]any{
			"count": 1, "text_list": []string{"hello"}, "output_file_list": []string{"hello.mp3"},
		}, "output_files", ""},
		{"batch_text_to_music", map[string]any{
			"count": 1, "prompt_list": []string{"calm piano"}, "output_file_list": []string{"bgm.mp3"},
		}, "output_files", ""},
		{"synthesize_speech", map[string]any{"text": "hello", "output_file": "s.mp3"}, "output_file", ""},
		{"batch_synthesize_speech", map[string]any{
			"count": 1, "text_list": []string{"hello"}, "output_file_list": []string{"s.mp3"},
		}, "output_files", ""},
		{"listen_audio", map[string]any{"file": "a.mp3"}, "transcript", ""},
		{"images_understand", map[string]any{
			"image_info": []map[string]any{{"file": "i.png", "prompt": "describe"}},
		}, "results", ""},
		{"audios_understand", map[string]any{
			"audio_info": []map[string]any{{"file": "a.mp3", "prompt": "transcribe"}},
		}, "results", ""},
		{"videos_understand", map[string]any{
			"video_info": []map[string]any{{"file": "v.mp4", "prompt": "describe"}},
		}, "results", ""},
		{"extract_content_from_websites", map[string]any{
			"tasks": []map[string]any{{"url": "https://example.com/", "prompt": "summarize"}},
		}, "results", ""},
		{"batch_web_search", map[string]any{
			"queries": []map[string]any{{"query": "hello world"}},
		}, "results", ""},
		{"image_reverse_search", map[string]any{
			"image_url": "https://example.com/i.png", "output_file": "out.md",
		}, "status", ""},
		{"images_search_and_download", map[string]any{
			"queries": []map[string]any{{"query": "cat", "prompt": "a cat photo", "task_name": "cat"}},
		}, "results", ""},
		{"images_list", map[string]any{}, "", "# Total Images:"},
		{"deploy", map[string]any{"dist_dir": "dist"}, "website_url", ""},
		{"init_react_project", map[string]any{"project_name": "p", "target_dir": "/tmp/p"}, "status", ""},
		{"deploy_html_presentation", map[string]any{"slides_dir": "/tmp/slides"}, "url", ""},
		{"upload_to_cdn", map[string]any{"file_path": "/tmp/i.png"}, "cdn_url", ""},
	}
	if len(cases) != 22 {
		t.Fatalf("expected 22 cases, got %d (new tool without test?)", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tc.name,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("unexpected tool error: %+v", res)
			}
			text := textOf(t, res)
			if tc.contains != "" {
				if !strings.Contains(text, tc.contains) {
					t.Errorf("expected %q in output, got: %s", tc.contains, text)
				}
				return
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(text), &body); err != nil {
				t.Fatalf("output is not JSON: %v (raw: %s)", err, text)
			}
			if _, ok := body[tc.key]; !ok {
				t.Errorf("expected %q key in output, got %v", tc.key, body)
			}
		})
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

// TestProxyForwardsLowSideEffectTools drives several low-side-effect tools
// through the ProxyHandler to the real matrix server, verifying argument
// compatibility and JSON output shape. Expensive tools (gen_videos, deploy,
// batch_text_to_music, ...) are deliberately excluded to avoid side effects.
func TestProxyForwardsLowSideEffectTools(t *testing.T) {
	url := "http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message"
	token := "sbk_v1_AGQAMY7IGNPLZQPUQE7X5GJKPU_7B4KJGNO6F2UJOFQCFFOIT6V"
	h := matrix.NewProxyHandler(matrix.ProxyConfig{URL: url, Token: token, Source: "hermes"})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cases := []struct {
		name     string
		contains string // expected substring of the raw output
		call     func(context.Context) (matrix.Output, error)
	}{
		{"get_voice_list", "available_voices", func(ctx context.Context) (matrix.Output, error) {
			return h.GetVoiceList(ctx, &matrix.GetVoiceListRequest{})
		}},
		// The real server returns a markdown listing for images_list.
		{"images_list", "# Total Images:", func(ctx context.Context) (matrix.Output, error) {
			return h.ImagesList(ctx, &matrix.ImagesListRequest{Start: 0, Number: 3})
		}},
		{"synthesize_speech", "url", func(ctx context.Context) (matrix.Output, error) {
			return h.SynthesizeSpeech(ctx, &matrix.SynthesizeSpeechRequest{Text: "hello", OutputFile: "/tmp/matrix-proxy-test.mp3"})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.call(ctx)
			if err != nil {
				t.Skipf("real matrix server unreachable: %v", err)
			}
			if !strings.Contains(string(out), tc.contains) {
				t.Errorf("expected %q in proxy output, got: %s", tc.contains, out)
			}
		})
	}
}

// startHTTPProcess launches the cmd/matrix binary as a streamable HTTP
// server on a random local port with the given extra args, and returns the
// bound address once it accepts connections.
func startHTTPProcess(t *testing.T, extraArgs ...string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // release; the subprocess binds it

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	args := append([]string{"run", "../cmd/matrix", "--http", addr}, extraArgs...)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// Wait for the HTTP endpoint to accept connections.
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not come up on %s: %v", addr, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return addr
}

// TestHTTPTransport verifies the -http streamable HTTP entry point end to
// end: launch the binary, connect with a go-sdk HTTP client, list tools and
// call one.
func TestHTTPTransport(t *testing.T) {
	addr := startHTTPProcess(t, "--mode", "mock")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	client := mcp.NewClient(&mcp.Implementation{Name: "matrix-http-test", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             "http://" + addr,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("http connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("http ListTools: %v", err)
	}
	if len(res.Tools) != 22 {
		t.Fatalf("expected 22 tools over HTTP, got %d", len(res.Tools))
	}

	call, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_voice_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("http CallTool: %v", err)
	}
	if call.IsError {
		t.Fatalf("unexpected error over HTTP: %+v", call)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(textOf(t, call)), &body); err != nil {
		t.Fatalf("output is not JSON over HTTP: %v", err)
	}
	if _, ok := body["available_voices"]; !ok {
		t.Errorf("expected available_voices over HTTP, got %v", body)
	}
}

// TestHTTPTransportProxyMode verifies the -http entry point in proxy mode:
// the streamable HTTP server forwards tool calls to the real matrix backend.
// Skipped when the backend is unreachable.
func TestHTTPTransportProxyMode(t *testing.T) {
	url := "http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message"
	token := "sbk_v1_AGQAMY7IGNPLZQPUQE7X5GJKPU_7B4KJGNO6F2UJOFQCFFOIT6V"
	addr := startHTTPProcess(t, "--mode", "proxy", "--url", url, "--token", token)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	client := mcp.NewClient(&mcp.Implementation{Name: "matrix-http-proxy-test", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             "http://" + addr,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("http connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("http ListTools: %v", err)
	}
	if len(res.Tools) != 22 {
		t.Fatalf("expected 22 tools over HTTP proxy, got %d", len(res.Tools))
	}

	call, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_voice_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("http CallTool: %v", err)
	}
	if call.IsError {
		t.Skipf("real matrix server unreachable: %+v", call)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(textOf(t, call)), &body); err != nil {
		t.Fatalf("output is not JSON over HTTP proxy: %v", err)
	}
	if _, ok := body["available_voices"]; !ok {
		t.Errorf("expected available_voices over HTTP proxy, got %v", body)
	}
}
