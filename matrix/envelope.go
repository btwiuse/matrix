package matrix

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// EnvelopeRewriter post-processes JSON-RPC envelopes emitted by the go-sdk
// streamable handler so the wire output matches the real matrix server
// where the SDK cannot express it:
//
//   - tools/list restores the "required" field of the deploy input schema
//     (the deploy tool is registered without it so a missing dist_dir is
//     accepted at call time, like on the real server);
//   - deploy tool results gain the real server's "display_data" metadata
//     (website_id/website_url) and an explicit "isError" flag (false for
//     success and for the soft missing-dist error, true for validation
//     errors, which the SDK already emits).
//
// Both plain JSON and SSE-framed responses are handled; responses that do
// not need rewriting pass through byte-identical.
type EnvelopeRewriter struct {
	next http.Handler
}

// NewEnvelopeRewriter wraps next with the envelope post-processor.
func NewEnvelopeRewriter(next http.Handler) *EnvelopeRewriter {
	return &EnvelopeRewriter{next: next}
}

// bufferedResponseWriter captures the inner handler's output so it can be
// rewritten before reaching the client.
type bufferedResponseWriter struct {
	header http.Header
	status int
	body   []byte
}

func (b *bufferedResponseWriter) Header() http.Header { return b.header }
func (b *bufferedResponseWriter) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	b.body = append(b.body, p...)
	return len(p), nil
}
func (b *bufferedResponseWriter) Flush() {}

// ServeHTTP reads the request (to learn the called tool name), buffers the
// response, rewrites the envelope when needed and forwards it.
func (e *EnvelopeRewriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(reqBody))

	var toolName string
	var msg map[string]any
	if json.Unmarshal(reqBody, &msg) == nil {
		if method, _ := msg["method"].(string); method == "tools/call" {
			if params, ok := msg["params"].(map[string]any); ok {
				toolName, _ = params["name"].(string)
			}
		}
	}

	bw := &bufferedResponseWriter{header: make(http.Header)}
	e.next.ServeHTTP(bw, r)

	out, changed := rewriteEnvelope(bw.body, toolName)
	for k, vs := range bw.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if changed {
		// The rewritten body may differ in length; recompute and drop any
		// framing flags the inner handler set for its own body.
		w.Header().Del("Transfer-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	}
	if bw.status != 0 {
		w.WriteHeader(bw.status)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if _, err := w.Write(out); err != nil {
		return
	}
}

// rewriteEnvelope parses body (plain JSON or a single SSE event), applies
// the deploy envelope patches and returns the rewritten body. The second
// return value reports whether the body changed.
func rewriteEnvelope(body []byte, tool string) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body, false
	}
	sse := bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:"))
	data := trimmed
	if sse {
		var lines [][]byte
		for _, line := range bytes.Split(trimmed, []byte("\n")) {
			if bytes.HasPrefix(line, []byte("data:")) {
				lines = append(lines, bytes.TrimSpace(line[len("data:"):]))
			}
		}
		if len(lines) == 0 {
			return body, false
		}
		data = bytes.Join(lines, nil)
	}

	var env map[string]any
	if err := json.Unmarshal(data, &env); err != nil {
		return body, false
	}
	result, _ := env["result"].(map[string]any)
	if result == nil {
		return body, false
	}

	changed := false

	// tools/list: restore the deploy input schema's required field.
	if tools, ok := result["tools"].([]any); ok {
		for _, t := range tools {
			tm, ok := t.(map[string]any)
			if !ok || tm["name"] != "deploy" {
				continue
			}
			schema, ok := tm["inputSchema"].(map[string]any)
			if !ok {
				continue
			}
			if _, has := schema["required"]; !has {
				schema["required"] = []any{"dist_dir"}
				changed = true
			}
		}
	}

	// deploy results: add display_data from content[0].text and an explicit
	// isError flag (the SDK omits false).
	if tool == "deploy" {
		if content, ok := result["content"].([]any); ok && len(content) > 0 {
			if c0, ok := content[0].(map[string]any); ok {
				if text, ok := c0["text"].(string); ok {
					var textObj map[string]any
					if json.Unmarshal([]byte(text), &textObj) == nil {
						id, idOK := textObj["website_id"]
						url, urlOK := textObj["website_url"]
						if idOK && urlOK {
							result["display_data"] = map[string]any{"website_id": id, "website_url": url}
							changed = true
						}
					}
				}
			}
			if _, has := result["isError"]; !has {
				result["isError"] = false
				changed = true
			}
		}
	}

	if !changed {
		return body, false
	}
	out, err := json.Marshal(env)
	if err != nil {
		return body, false
	}
	if sse {
		return append([]byte("event: message\ndata: "), append(out, []byte("\n\n")...)...), true
	}
	return out, true
}
