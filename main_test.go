package htmlinject

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testSnippet = `<div id="probe">injected</div>`

// newTestHandler 起一个带三种响应的上游,并返回注入代理 handler。
// 不监听真实端口: 直接经 httptest 调用 handler,这正是 handler/listener 解耦的收益。
func newTestHandler(t *testing.T) (http.Handler, *httptest.Server) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"ok":true}`)
		case "/nobody":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, "<p>no body tag</p>")
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, "<!DOCTYPE html><html><head><title>t</title></head><body><h1>hi</h1></body></html>")
		}
	}))
	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("解析上游地址: %v", err)
	}
	h, err := NewHandler(Config{Upstream: u, Injection: testSnippet})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h, upstream
}

func TestInjectBeforeBody(t *testing.T) {
	h, upstream := newTestHandler(t)
	defer upstream.Close()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, testSnippet) {
		t.Fatalf("响应缺少注入片段")
	}
	if i, j := strings.Index(body, testSnippet), strings.Index(body, "</body>"); i == -1 || j == -1 || i > j {
		t.Fatalf("注入片段应在 </body> 前 (snippet@%d, body@%d)", i, j)
	}
}

func TestInjectIdempotent(t *testing.T) {
	inj := NewInjector(testSnippet, false)

	html := "<html><body>x</body></html>"
	injected, changed := inj.Inject(html)
	if !changed {
		t.Fatal("首次注入应发生改写")
	}

	again, changed := inj.Inject(injected)
	if changed {
		t.Fatal("已注入的内容不应再次改写")
	}
	if got := strings.Count(again, testSnippet); got != 1 {
		t.Fatalf("注入片段出现 %d 次, 期望 1", got)
	}
}

func TestNonHTMLPassthrough(t *testing.T) {
	h, upstream := newTestHandler(t)
	defer upstream.Close()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api", nil))

	if got, want := rec.Body.String(), `{"ok":true}`; got != want {
		t.Fatalf("非 HTML 响应被改写: %q, 期望 %q", got, want)
	}
}

func TestInjectFallbackAppend(t *testing.T) {
	inj := NewInjector(testSnippet, false)

	got, changed := inj.Inject("<p>no closing tags</p>")
	if !changed {
		t.Fatal("无 </body> 时应执行兜底注入")
	}
	if !strings.HasSuffix(got, testSnippet) {
		t.Fatalf("兜底注入应追加到末尾: %q", got)
	}
}
