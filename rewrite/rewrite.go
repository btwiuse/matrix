// Package rewrite rewrites HTML documents by inserting a snippet before the
// closing </body> tag (falling back to </html>, then to the end of the
// document). It is the reusable core of inject-proxy's HTML injection and of
// deploy-server's index rewriting: any component that serves HTML can apply
// the same idempotent rewrite without going through a reverse proxy.
//
// The rewrite uses the HTML5 tokenizer, so a literal "</body>" inside a
// script or attribute is treated as text and never corrupted (the failure
// mode of regex/string injection). Documents are rewritten token by token
// with Raw(), preserving the original bytes exactly apart from the
// injection point. Injection is idempotent: a marker derived from the
// snippet's SHA-256 marks documents that already carry the snippet, so a
// second pass leaves them untouched.
package rewrite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	htmlt "golang.org/x/net/html"
)

// Injector holds a snippet and its idempotency marker, and can rewrite any
// HTML document.
type Injector struct {
	marker    string
	injection string
}

// New returns an Injector for the snippet. The idempotency marker derives
// from the snippet's hash, so identical snippets share the same marker.
func New(injection string) *Injector {
	sum := sha256.Sum256([]byte(injection))
	marker := fmt.Sprintf("<!-- html-inject:%s -->", hex.EncodeToString(sum[:])[:12])
	return &Injector{marker: marker, injection: injection}
}

// Inject inserts the snippet into html, returning the rewritten document
// and whether a rewrite happened. The position priority is: before
// </body>, else before </html>, else appended at the end. Documents that
// already carry the marker are returned unchanged.
func (i *Injector) Inject(html string) (string, bool) {
	if strings.Contains(html, i.marker) {
		return html, false
	}
	var out strings.Builder
	out.Grow(len(html) + len(i.injection) + 64)

	z := htmlt.NewTokenizer(strings.NewReader(html))
	inserted := false
	for {
		tt := z.Next()
		if tt == htmlt.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				break
			}
			// Unparseable stream: pass through untouched rather than
			// risk a corrupting rewrite.
			return html, false
		}
		if tt == htmlt.EndTagToken && !inserted {
			switch z.Token().Data {
			case "body", "html":
				out.WriteString("\n" + i.marker + "\n" + i.injection)
				inserted = true
			}
		}
		out.Write(z.Raw())
	}
	if !inserted {
		out.WriteString("\n" + i.marker + "\n" + i.injection)
	}
	return out.String(), true
}
