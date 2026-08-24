// HTML 注入反向代理(Go 版):转发请求到上游,响应时把注入片段插到 </body> 前。
// 用法(CLI 见 cmd/inject-proxy):
//
//	go run ./cmd/inject-proxy --upstream http://127.0.0.1:8000 --port 8080 --inject ./inject-ball.html
//	go run ./cmd/inject-proxy --upstream http://127.0.0.1:8000 --port 8080 --inject-html '<script src="/x.js"></script>'
package htmlinject

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gearshell/matrix/rewrite"
)

// Config 描述注入代理的静态配置。
type Config struct {
	Upstream  *url.URL // 上游目标地址
	Injection string   // 注入片段(原始 HTML)
	Verbose   bool     // 打印日志
}

// Injector 持有注入片段与其幂等标记,可对任意 HTML 执行注入。
// 改写核心在 rewrite 包,这里仅附加代理侧的日志开关。
type Injector struct {
	*rewrite.Injector
	verbose bool
}

// NewInjector 根据注入片段构造 Injector,幂等标记取内容哈希。
func NewInjector(injection string, verbose bool) *Injector {
	return &Injector{Injector: rewrite.New(injection), verbose: verbose}
}

// maybeDecompress 按首字节嗅探并解压。
// 返回 (解压后字节, 编码名, 错误)。enc=="" 表示非压缩。
// 处理两种常见场景:gzip(1f 8b) 与 zlib/deflate(78 01/9c/da)。
// 上游某些实现会漏写 Content-Encoding 头(比如 nginx 的 gzip_static / 反代配置错),
// 此时 http.Transport 不会自动解压,需要在这里兜底。
func maybeDecompress(b []byte) ([]byte, string, error) {
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, "gzip", err
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, "gzip", err
		}
		return out, "gzip", nil
	}
	if len(b) >= 2 && b[0] == 0x78 && (b[1] == 0x01 || b[1] == 0x9c || b[1] == 0xda) {
		r, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, "zlib", err
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, "zlib", err
		}
		return out, "zlib", nil
	}
	// raw deflate(无头)难以可靠识别,交给上游 Content-Encoding,这里不再兜底
	return b, "", nil
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

	// 善意补全 charset:如果上游响应是 text/html 但头里没声明 charset,
	// 浏览器会按 lang 等启发式 fallback,经常把 UTF-8 字节按 GB18030/Windows-1252 解码,
	// 导致注入片段里的非 ASCII 字符(如 "关闭"、"×")变成 mojibake。
	// 主动声明 utf-8 能让浏览器稳定按 UTF-8 解码整页。
	// 注意:仅在"完全没声明 charset"时才补;上游明确声明了别的 charset(如 gb18030)
	// 不能改头——那是上游的明确意图,改了就违背原意(需要走编码转码方案才能处理)。
	if !strings.Contains(strings.ToLower(ct), "charset=") {
		resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		if i.verbose {
			log.Printf("[charset] %s %s 头里缺 charset, 补为 utf-8", resp.Request.Method, resp.Request.URL.Path)
		}
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体: %w", err)
	}
	resp.Body.Close()

	// 防御性解压:即便上游漏写了 Content-Encoding 头,只要 body 看起来像 gzip/zlib,
	// 也按压缩流处理。否则会把压缩字节当作 HTML 注入,产生 "乱码 + 注入片段" 的拼接。
	decompressed, enc, derr := maybeDecompress(raw)
	if derr != nil {
		return fmt.Errorf("检测/解压失败 (enc=%s): %w", enc, derr)
	}
	if enc != "" && i.verbose {
		log.Printf("[decompress] %s %s 按 %s 解压 (%d -> %d 字节)", resp.Request.Method, resp.Request.URL.Path, enc, len(raw), len(decompressed))
	}
	raw = decompressed

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
	// 关掉压缩协商:跟 Node 版 server.js 一致。让"行为正常"的上游直接返回明文,
	// 节省 CPU/带宽,也让 Inject 拿到最干净的 HTML。
	// 设为 "identity" 而不是 Del 是因为 http.Transport 在 RoundTrip 前,如果发现
	// 请求没有 Accept-Encoding 且 DisableCompression=false,会自动补上 "gzip",
	// 那时 Del 已经晚了。显式给个 identity,transport 看到已有值就不再动。
	// 即便上游配置错乱(漏 Content-Encoding 头等)继续吐压缩字节,
	// ModifyResponse 里的 maybeDecompress 会按首字节嗅探兜底。
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Header.Set("Accept-Encoding", "identity")
	}
	proxy.ModifyResponse = NewInjector(cfg.Injection, cfg.Verbose).ModifyResponse
	return proxy, nil
}
