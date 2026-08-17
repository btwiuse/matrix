// HTML 注入反向代理(Go 版):转发请求到上游,响应时把注入片段插到 </body> 前。
// 用法:
//
//	go run main.go --upstream http://127.0.0.1:8000 --port 8080 --inject ./inject-ball.html
//	go run main.go --upstream http://127.0.0.1:8000 --port 8080 --inject-html '<script src="/x.js"></script>'
package htmlinject

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// Config 描述注入代理的静态配置。
type Config struct {
	Upstream  *url.URL // 上游目标地址
	Injection string   // 注入片段(原始 HTML)
	Verbose   bool     // 打印日志
}

// Injector 持有注入片段与其幂等标记,可对任意 HTML 执行注入。
type Injector struct {
	marker    string
	injection string
	verbose   bool
}

// NewInjector 根据注入片段构造 Injector,幂等标记取内容哈希。
func NewInjector(injection string, verbose bool) *Injector {
	sum := sha256.Sum256([]byte(injection))
	marker := fmt.Sprintf("<!-- html-inject:%s -->", hex.EncodeToString(sum[:])[:12])
	return &Injector{marker: marker, injection: injection, verbose: verbose}
}

// Inject 把注入片段插到 html 中,返回改写结果与是否发生改写。
// 位置优先级: </body> 前 > </html> 前 > 末尾。
func (i *Injector) Inject(html string) (string, bool) {
	if strings.Contains(html, i.marker) {
		return html, false
	}
	if p := strings.LastIndex(html, "</body>"); p != -1 {
		return html[:p] + "\n" + i.marker + "\n" + i.injection + "\n" + html[p:], true
	}
	if p := strings.LastIndex(html, "</html>"); p != -1 {
		return html[:p] + "\n" + i.marker + "\n" + i.injection + "\n" + html[p:], true
	}
	return html + "\n" + i.marker + "\n" + i.injection, true
}

// ModifyResponse 实现 httputil.ReverseProxy 的响应改写钩子。
func (i *Injector) ModifyResponse(resp *http.Response) error {
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		if i.verbose {
			log.Printf("[pass] %s %s 非 text/html (%q)", resp.Request.Method, resp.Request.URL.Path, ct)
		}
		return nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体: %w", err)
	}
	resp.Body.Close()

	html := string(raw)
	injected, changed := i.Inject(html)
	if !changed {
		if i.verbose {
			log.Printf("[skip] %s %s 已注入,跳过", resp.Request.Method, resp.Request.URL.Path)
		}
		resp.Body = io.NopCloser(strings.NewReader(html))
		return nil
	}

	if i.verbose {
		log.Printf("[inject] %s %s (%d -> %d 字节)", resp.Request.Method, resp.Request.URL.Path, len(raw), len(injected))
	}
	resp.Body = io.NopCloser(strings.NewReader(injected))
	resp.ContentLength = int64(len(injected))
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Etag")
	return nil
}

// NewHandler 构建注入反向代理,返回可挂载的 http.Handler。
// 与监听解耦: 由调用方决定 http.Server / ListenAndServe / 挂到子路由。
func NewHandler(cfg Config) (http.Handler, error) {
	if cfg.Upstream == nil {
		return nil, fmt.Errorf("缺少上游地址")
	}
	proxy := httputil.NewSingleHostReverseProxy(cfg.Upstream)
	proxy.ModifyResponse = NewInjector(cfg.Injection, cfg.Verbose).ModifyResponse
	return proxy, nil
}
