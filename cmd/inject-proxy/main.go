// inject-proxy: HTML 注入反向代理的 CLI 入口。
// 转发请求到上游,响应时把注入片段插到 </body> 前。
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	htmlinject "github.com/gearshell/matrix"

	"github.com/spf13/cobra"
)

// defaultPort 优先取环境变量 PORT,未设置或非法时回退 8080。
func defaultPort() int {
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			return n
		}
	}
	return 8080
}

func main() {
	var (
		upstream   string
		port       int
		injectFile string
		injectHTML string
		verbose    bool
	)

	root := &cobra.Command{
		Use:   "inject-proxy",
		Short: "HTML 注入反向代理",
		Long:  "转发请求到上游,响应时把注入片段插到 </body> 前(幂等、非 HTML 透传)。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 注入内容: 文件优先,内联兜底
			injection := injectHTML
			if injectFile != "" {
				b, err := os.ReadFile(injectFile)
				if err != nil {
					return fmt.Errorf("读取注入文件: %w", err)
				}
				injection = string(b)
			}
			if strings.TrimSpace(injection) == "" {
				return fmt.Errorf("缺少注入内容: 用 --inject <file> 或 --inject-html <html> 提供")
			}

			target, err := url.Parse(upstream)
			if err != nil {
				return fmt.Errorf("解析 upstream: %w", err)
			}

			handler, err := htmlinject.NewHandler(htmlinject.Config{
				Upstream:  target,
				Injection: injection,
				Verbose:   verbose,
			})
			if err != nil {
				return fmt.Errorf("构建代理: %w", err)
			}

			addr := fmt.Sprintf(":%d", port)
			log.Printf("注入代理监听 %s -> %s (注入片段 %d 字节)", addr, target, len(injection))
			return http.ListenAndServe(addr, handler)
		},
	}

	root.Flags().StringVar(&upstream, "upstream", "http://127.0.0.1:8000", "上游目标地址")
	root.Flags().IntVar(&port, "port", defaultPort(), "监听端口 (默认取环境变量 PORT, 缺省 8080)")
	root.Flags().StringVar(&injectFile, "inject", "", "注入片段文件路径")
	root.Flags().StringVar(&injectHTML, "inject-html", "", "注入片段(内联)")
	root.Flags().BoolVar(&verbose, "verbose", false, "打印日志")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
