package htmlinject

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
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

func TestNonHTMLPassthrough(t *testing.T) {
	h, upstream := newTestHandler(t)
	defer upstream.Close()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api", nil))

	if got, want := rec.Body.String(), `{"ok":true}`; got != want {
		t.Fatalf("非 HTML 响应被改写: %q, 期望 %q", got, want)
	}
}

// TestMaybeDecompress 验证 maybeDecompress 按首字节嗅探并解压,
// 处理"上游漏 Content-Encoding 头但 body 是压缩字节"这一类失配。
// 覆盖:空输入、太短、非压缩、看起来像但其实是损坏的流、合法 gzip、合法 zlib。
func TestMaybeDecompress(t *testing.T) {
	// 损坏的伪压缩流应当返回错误,且识别到对应编码名
	for _, tc := range []struct {
		name string
		in   []byte
		enc  string
	}{
		{"empty", nil, ""},
		{"one byte", []byte{0x1f}, ""},
		{"plain ascii", []byte("hello world"), ""},
		{"looks like gzip but corrupted", []byte{0x1f, 0x8b, 0x08, 0x99, 0x99}, "gzip"},
		{"looks like zlib but corrupted", []byte{0x78, 0x9c, 0x99, 0x99}, "zlib"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, enc, err := maybeDecompress(tc.in)
			if enc != tc.enc {
				t.Fatalf("enc: want %q got %q", tc.enc, enc)
			}
			if tc.enc == "" {
				// 不识别:必须原样返回且无错
				if err != nil {
					t.Fatalf("不应返回错误: %v", err)
				}
				if !bytes.Equal(out, tc.in) {
					t.Fatalf("不识别时应原样返回: in=%x out=%x", tc.in, out)
				}
				return
			}
			// 识别了编码但数据损坏:应当返回错误
			if err == nil {
				t.Fatalf("损坏的 %s 流应返回错误,但 err=nil out=%x", tc.enc, out)
			}
		})
	}

	// 合法 gzip 流应当解压回原文
	original := []byte("<!doctype html><html><head><title>t</title></head><body><h1>hi 你好 ×</h1></body></html>")
	var gbuf bytes.Buffer
	gw := gzip.NewWriter(&gbuf)
	gw.Write(original)
	gw.Close()
	out, enc, err := maybeDecompress(gbuf.Bytes())
	if err != nil {
		t.Fatalf("合法 gzip 解压失败: %v", err)
	}
	if enc != "gzip" {
		t.Fatalf("want enc=gzip, got %q", enc)
	}
	if !bytes.Equal(out, original) {
		t.Fatalf("gzip 解压结果不一致:\n want=%x\n  got=%x", original, out)
	}

	// 合法 zlib 流应当解压回原文(zlib.NewWriter 默认用 78 9c 头)
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	zw.Write(original)
	zw.Close()
	out, enc, err = maybeDecompress(zbuf.Bytes())
	if err != nil {
		t.Fatalf("合法 zlib 解压失败: %v", err)
	}
	if enc != "zlib" {
		t.Fatalf("want enc=zlib, got %q", enc)
	}
	if !bytes.Equal(out, original) {
		t.Fatalf("zlib 解压结果不一致:\n want=%x\n  got=%x", original, out)
	}
}

// TestCharsetBackfilled 验证代理在上游头里没声明 charset 时,主动补 charset=utf-8。
// 这是 mojibake("关闭" -> "å³é­")修复路径的回归保护。
func TestCharsetBackfilled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html") // 故意不设 charset
		io.WriteString(w, "<!doctype html><html><head><title>t</title></head><body><h1>hi</h1></body></html>")
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	h, err := NewHandler(Config{Upstream: u, Injection: testSnippet})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(strings.ToLower(ct), "charset=utf-8") {
		t.Fatalf("响应头应补 charset=utf-8,实际是 %q", ct)
	}
}

// TestCharsetRespectedWhenUpstreamSets 验证上游明确声明了 charset 时,代理绝不动。
// 这一条尤其重要:不能因为 charset 补全逻辑误伤 GBK 等非 UTF-8 页面。
func TestCharsetRespectedWhenUpstreamSets(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=gb18030")
		io.WriteString(w, "<!doctype html><html><body>hi</body></html>")
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	h, err := NewHandler(Config{Upstream: u, Injection: testSnippet})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	ct := rec.Header().Get("Content-Type")
	lower := strings.ToLower(ct)
	if !strings.Contains(lower, "gb18030") {
		t.Fatalf("上游 charset=gb18030 应被保留,实际 %q", ct)
	}
	if strings.Contains(lower, "utf-8") {
		t.Fatalf("绝不该被改成 utf-8,实际 %q", ct)
	}
}

// TestUpstreamMissingContentEncodingButGzipBody 验证上游漏 Content-Encoding 头
// 但 body 是合法 gzip 字节时,代理能嗅探解压并正常注入。
// 这正是本次调试发现的根因场景,作为回归保护。
func TestUpstreamMissingContentEncodingButGzipBody(t *testing.T) {
	original := "<!doctype html><html><head><title>t</title></head><body><h1>hi</h1></body></html>"
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	io.WriteString(gw, original)
	gw.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 关键:不设 Content-Encoding 头,只写 Content-Length(且写的是压缩后字节数)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
		w.Write(buf.Bytes())
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	h, err := NewHandler(Config{Upstream: u, Injection: testSnippet})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	body := rec.Body.Bytes()
	if bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		t.Fatalf("gzip 字节应被嗅探解压,而不是原样转发;首 8 字节: %x", body[:8])
	}
	if !strings.Contains(string(body), testSnippet) {
		t.Fatalf("响应缺少注入片段:gzip 流未被解压,Inject 走了末尾追加分支")
	}
	if !strings.Contains(string(body), "<body>") {
		t.Fatalf("响应里不应有解压前的压缩字节污染;实际响应:\n%s", body)
	}
}
