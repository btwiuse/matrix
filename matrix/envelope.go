package matrix

import (
	"bytes"
	"encoding/json"
	"errors"
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
// Rewrites are surgical byte splices: every byte that does not need to
// change is forwarded exactly as produced by the SDK (no JSON re-marshaling,
// so key order and formatting survive). Requests that need no patching (all
// tools except deploy, and everything besides tools/list) are streamed
// through without buffering.
type EnvelopeRewriter struct {
	next http.Handler
}

// NewEnvelopeRewriter wraps next with the envelope post-processor.
func NewEnvelopeRewriter(next http.Handler) *EnvelopeRewriter {
	return &EnvelopeRewriter{next: next}
}

// deploySchemaReal is the deploy tool's input schema exactly as captured in
// the embedded schema.json, "required" list included, compacted for the
// wire (the SDK compacts every tool schema; byte fidelity demands the same
// formatting and the captured key order). The SDK registration drops
// "required" for lenient call-time validation (a missing dist_dir defaults
// to <workspace>/dist, like on the real server); this restores it in
// tools/list responses.
var deploySchemaReal []byte

func init() {
	var raw map[string]ToolSpec
	if err := json.Unmarshal(schemaJSON, &raw); err == nil {
		if t, ok := raw["deploy"]; ok {
			var compact bytes.Buffer
			if json.Compact(&compact, t.InputSchema) == nil {
				deploySchemaReal = append([]byte(nil), compact.Bytes()...)
			}
		}
	}
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

// ServeHTTP reads the request (to learn the called tool name) and buffers
// only the responses that can need rewriting: tools/list and deploy calls.
// Everything else is streamed straight to the inner handler, untouched.
func (e *EnvelopeRewriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(reqBody))

	var method, toolName string
	var msg map[string]any
	if json.Unmarshal(reqBody, &msg) == nil {
		method, _ = msg["method"].(string)
		if method == "tools/call" {
			if params, ok := msg["params"].(map[string]any); ok {
				toolName, _ = params["name"].(string)
			}
		}
	}
	if method != "tools/list" && toolName != "deploy" {
		e.next.ServeHTTP(w, r)
		return
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
// return value reports whether the body changed. Unpatched bytes are
// preserved exactly: only the result object's value is replaced (spliced),
// and only the new keys are appended inside it.
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

	rs, re, ok := valueSpan(data, "result")
	if !ok {
		return body, false
	}
	result := data[rs:re]

	var patched []byte
	if tool == "deploy" {
		patched, ok = patchDeployResult(result)
	} else {
		patched, ok = patchToolsList(result)
	}
	if !ok {
		return body, false
	}
	data = splice(data, rs, re, patched)
	if sse {
		return append([]byte("event: message\ndata: "), append(data, []byte("\n\n")...)...), true
	}
	return data, true
}

// patchDeployResult appends the real server's display_data metadata and an
// explicit isError flag to a deploy tool result. Original bytes stay in
// place: new keys are added at the end of the result object. It reports
// whether the result changed.
func patchDeployResult(result []byte) ([]byte, bool) {
	cs, ce, hasContent := valueSpan(result, "content")
	if !hasContent {
		return result, false
	}
	spans, err := arraySpans(result[cs:ce])
	if err != nil || len(spans) == 0 {
		return result, false
	}
	elemStart := cs + spans[0][0]
	ts, te, ok := valueSpan(result[elemStart:cs+spans[0][1]], "text")
	if !ok {
		return result, false
	}
	var text string
	if err := json.Unmarshal(result[elemStart+ts:elemStart+te], &text); err != nil {
		return result, false
	}

	var add []byte
	if _, _, has := valueSpan(result, "display_data"); !has {
		var textObj map[string]any
		if json.Unmarshal([]byte(text), &textObj) == nil {
			id, idOK := textObj["website_id"]
			url, urlOK := textObj["website_url"]
			if idOK && urlOK {
				if dd, err := json.Marshal(map[string]any{"website_id": id, "website_url": url}); err == nil {
					add = append(add, `"display_data":`...)
					add = append(add, dd...)
				}
			}
		}
	}
	if _, _, has := valueSpan(result, "isError"); !has {
		if len(add) > 0 {
			add = append(add, ',')
		}
		add = append(add, `"isError":false`...)
	}
	if len(add) == 0 || len(result) == 0 || result[len(result)-1] != '}' {
		return result, false
	}
	patched := make([]byte, 0, len(result)+len(add)+1)
	patched = append(patched, result[:len(result)-1]...)
	patched = append(patched, ',')
	patched = append(patched, add...)
	patched = append(patched, '}')
	return patched, true
}

// patchToolsList replaces the deploy tool's inputSchema value with the
// verbatim embedded schema (restoring its "required" list), leaving every
// other byte of the response untouched. It reports whether the result
// changed.
func patchToolsList(result []byte) ([]byte, bool) {
	if deploySchemaReal == nil {
		return result, false
	}
	ts, te, ok := valueSpan(result, "tools")
	if !ok {
		return result, false
	}
	tools := result[ts:te]
	spans, err := arraySpans(tools)
	if err != nil {
		return result, false
	}
	patched := tools
	for i := len(spans) - 1; i >= 0; i-- {
		elem := patched[spans[i][0]:spans[i][1]]
		ns, ne, ok := valueSpan(elem, "name")
		if !ok || !bytes.Equal(elem[ns:ne], []byte(`"deploy"`)) {
			continue
		}
		is, ie, ok := valueSpan(elem, "inputSchema")
		if !ok {
			continue
		}
		if bytes.Equal(elem[is:ie], deploySchemaReal) {
			break
		}
		patched = splice(patched, spans[i][0]+is, spans[i][0]+ie, deploySchemaReal)
		break
	}
	if bytes.Equal(patched, tools) {
		return result, false
	}
	return splice(result, ts, te, patched), true
}

// valueSpan returns the byte span [start, end) of the value of key inside
// the JSON object data, and whether the key was found. The span points into
// data and covers exactly the value, including any embedded whitespace.
func valueSpan(data []byte, key string) (start, end int, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return 0, 0, false
	}
	for dec.More() {
		ktok, err := dec.Token()
		if err != nil {
			return 0, 0, false
		}
		pos := int(dec.InputOffset()) // right after the key token
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return 0, 0, false
		}
		end = int(dec.InputOffset())
		if k, isStr := ktok.(string); isStr && k == key {
			s := pos
			for s < end && isJSONSpace(data[s]) {
				s++
			}
			if s < end && data[s] == ':' {
				s++
			}
			for s < end && isJSONSpace(data[s]) {
				s++
			}
			return s, end, true
		}
	}
	return 0, 0, false
}

// arraySpans returns the byte span of every element of the JSON array data.
func arraySpans(data []byte) ([][2]int, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('[') {
		return nil, errors.New("not a JSON array")
	}
	var spans [][2]int
	for dec.More() {
		start := int(dec.InputOffset()) // right after '[' or the previous value
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		end := int(dec.InputOffset())
		s := start
		for s < end && (isJSONSpace(data[s]) || data[s] == ',') {
			s++
		}
		spans = append(spans, [2]int{s, end})
	}
	return spans, nil
}

// splice replaces data[a:b] with repl, preserving everything else.
func splice(data []byte, a, b int, repl []byte) []byte {
	out := make([]byte, 0, len(data)-(b-a)+len(repl))
	out = append(out, data[:a]...)
	out = append(out, repl...)
	out = append(out, data[b:]...)
	return out
}

func isJSONSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
