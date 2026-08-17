package rewrite

import (
	"strings"
	"testing"
)

const testSnippet = `<div id="probe">injected</div>`

func TestInjectIdempotent(t *testing.T) {
	inj := New(testSnippet)

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

func TestInjectFallbackAppend(t *testing.T) {
	inj := New(testSnippet)

	got, changed := inj.Inject("<p>no closing tags</p>")
	if !changed {
		t.Fatal("无 </body> 时应执行兜底注入")
	}
	if !strings.HasSuffix(got, testSnippet) {
		t.Fatalf("兜底注入应追加到末尾: %q", got)
	}
}

// TestInjectIgnoresLiteralTags 核心鲁棒性: 脚本/属性里的字面 "</body>" 是文本,
// 不是注入点 —— 字符串匹配法会在这里插错位置, tokenizer 不会。
func TestInjectIgnoresLiteralTags(t *testing.T) {
	inj := New(testSnippet)

	doc := `<!DOCTYPE html>
<html>
<head><script>
const s = "</body>";
document.write(s);
</script></head>
<body>
<div title="</body>">hi</div>
</body>
</html>`

	got, changed := inj.Inject(doc)
	if !changed {
		t.Fatal("应发生注入")
	}
	wantPos := strings.LastIndex(got, "</body>")
	gotPos := strings.Index(got, testSnippet)
	if gotPos == -1 || gotPos > wantPos {
		t.Fatalf("注入位置错误: snippet@%d 应在 body 结束标签@%d 前", gotPos, wantPos)
	}
	if !strings.Contains(got, `const s = "</body>";`) {
		t.Fatal("script 里的字面 </body> 被破坏")
	}
	scriptStart := strings.Index(got, "<script>")
	scriptEnd := strings.Index(got, "</script>")
	if scriptStart != -1 && scriptEnd != -1 &&
		strings.Contains(got[scriptStart:scriptEnd], testSnippet) {
		t.Fatal("注入片段被插进了 script 内容里")
	}
}

// TestInjectPreservesFormat 校验除注入点外原文逐字节一致。
func TestInjectPreservesFormat(t *testing.T) {
	inj := New(testSnippet)
	doc := "<html>\n<body>\n  <p a='x' b=\"y\">raw &amp; text</p>\n</body>\n</html>\n"

	got, _ := inj.Inject(doc)
	trimmed := strings.Replace(got, "\n"+inj.marker+"\n"+testSnippet, "", 1)
	if trimmed != doc {
		t.Fatalf("原文被篡改:\n got: %q\nwant: %q", trimmed, doc)
	}
}
