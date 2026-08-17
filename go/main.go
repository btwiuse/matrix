// HTML 注入反向代理(Go 版):转发请求到上游,响应时把注入片段插到 </body> 前。
// 用法:
//
//	go run main.go --upstream http://127.0.0.1:8000 --port 8080 --inject ./inject-ball.html
//	go run main.go --upstream http://127.0.0.1:8000 --port 8080 --inject-html '<script src="/x.js"></script>'
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

func main() {
	upstream := flag.String("upstream", "http://127.0.0.1:8000", "上游目标地址")
	port := flag.Int("port", 8080, "监听端口")
	injectFile := flag.String("inject", "", "注入片段文件路径")
	injectHTML := flag.String("inject-html", "", "注入片段(内联)")
	verbose := flag.Bool("verbose", false, "打印日志")
	flag.Parse()

	// 注入内容:文件优先,内联兜底
	var injection string
	if *injectFile != "" {
		b, err := os.ReadFile(*injectFile)
		if err != nil {
			log.Fatalf("读取注入文件: %v", err)
		}
		injection = string(b)
	}
	if *injectHTML != "" {
		injection = *injectHTML
	}
	if strings.TrimSpace(injection) == "" {
		log.Fatal("缺少注入内容: 用 --inject <file> 或 --inject-html <html> 提供")
	}

	// 幂等标记:按注入内容哈希生成,检测到已注入则跳过
	sum := sha256.Sum256([]byte(injection))
	marker := fmt.Sprintf("<!-- html-inject:%s -->", hex.EncodeToString(sum[:])[:12])

	target, err := url.Parse(*upstream)
	if err != nil {
		log.Fatalf("解析 upstream: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(resp *http.Response) error {
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			if *verbose {
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
		injected, changed := injectInto(html, marker, injection)
		if !changed {
			if *verbose {
				log.Printf("[skip] %s %s 已注入,跳过", resp.Request.Method, resp.Request.URL.Path)
			}
			resp.Body = io.NopCloser(strings.NewReader(html))
			return nil
		}

		if *verbose {
			log.Printf("[inject] %s %s (%d -> %d 字节)", resp.Request.Method, resp.Request.URL.Path, len(raw), len(injected))
		}
		resp.Body = io.NopCloser(strings.NewReader(injected))
		resp.ContentLength = int64(len(injected))
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.Header.Del("Etag")
		return nil
	}

	log.Printf("注入代理监听 :%d -> %s (注入片段 %d 字节)", *port, target, len(injection))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), proxy))
}

// injectInto 把注入片段插到 HTML 中,返回改写后的内容与是否发生改写。
// 位置优先级: </body> 前 > </html> 前 > 末尾。
func injectInto(html, marker, injection string) (string, bool) {
	if strings.Contains(html, marker) {
		return html, false
	}
	if i := strings.LastIndex(html, "</body>"); i != -1 {
		return html[:i] + "\n" + marker + "\n" + injection + "\n" + html[i:], true
	}
	if i := strings.LastIndex(html, "</html>"); i != -1 {
		return html[:i] + "\n" + marker + "\n" + injection + "\n" + html[i:], true
	}
	return html + "\n" + marker + "\n" + injection, true
}
