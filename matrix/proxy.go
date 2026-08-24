package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ProxyConfig configures forwarding to the real matrix MCP server.
type ProxyConfig struct {
	// URL is the HTTP endpoint, e.g. http://host:8080/mcp/message.
	URL string
	// Token is the sk token passed as the ?sk= query parameter.
	Token string
	// Source is the ?source= query parameter (default "hermes").
	Source string
	// HTTPClient overrides the default client (optional).
	HTTPClient *http.Client
}

// ProxyHandler forwards every tool call to the real matrix MCP server,
// preserving its exact behavior and output formats.
type ProxyHandler struct {
	http     *http.Client
	mu       sync.Mutex
	id       int
	endpoint string // upstream URL with the sk/source query params attached
}

// NewProxyHandler builds a ProxyHandler for the given endpoint. The token
// and source label are attached as URL query parameters, percent-encoded
// and merged with any query the endpoint URL already carries.
func NewProxyHandler(cfg ProxyConfig) (*ProxyHandler, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("proxy: empty upstream URL")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	src := cfg.Source
	if src == "" {
		src = "hermes"
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing upstream URL %q: %w", cfg.URL, err)
	}
	q := u.Query()
	q.Set("sk", cfg.Token)
	q.Set("source", src)
	u.RawQuery = q.Encode()
	return &ProxyHandler{http: hc, endpoint: u.String()}, nil
}

// call forwards a tools/call request and returns the raw text content of the
// real server's response.
func (p *ProxyHandler) call(ctx context.Context, name string, args any) (Output, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encoding arguments: %w", err)
	}
	// The real matrix server rejects a null arguments value, so always send
	// a JSON object, even for tools with no parameters.
	arguments := json.RawMessage(raw)
	if len(arguments) == 0 || string(arguments) == "null" {
		arguments = json.RawMessage("{}")
	}

	p.mu.Lock()
	p.id++
	id := p.id
	p.mu.Unlock()

	reqBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forwarding %s: %w", name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("reading upstream response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s returned HTTP %d: %s", name, resp.StatusCode, truncate(string(body), 300))
	}

	var rpc struct {
		Result *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &rpc); err != nil {
		return nil, fmt.Errorf("parsing upstream response: %w", err)
	}
	if rpc.Error != nil {
		return nil, fmt.Errorf("upstream error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	if rpc.Result == nil {
		return nil, fmt.Errorf("upstream returned no result for %s", name)
	}
	for _, c := range rpc.Result.Content {
		if c.Type == "text" {
			return []byte(c.Text), nil
		}
	}
	return nil, fmt.Errorf("upstream returned no text content for %s", name)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- Handler interface implementation (22 methods) ---

func (p *ProxyHandler) ImageSynthesize(ctx context.Context, in *ImageSynthesizeRequest) (Output, error) {
	return p.call(ctx, "image_synthesize", in)
}

func (p *ProxyHandler) GenVideos(ctx context.Context, in *GenVideosRequest) (Output, error) {
	return p.call(ctx, "gen_videos", in)
}

func (p *ProxyHandler) BatchTextToVideo(ctx context.Context, in *BatchTextToVideoRequest) (Output, error) {
	return p.call(ctx, "batch_text_to_video", in)
}

func (p *ProxyHandler) BatchImageToVideo(ctx context.Context, in *BatchImageToVideoRequest) (Output, error) {
	return p.call(ctx, "batch_image_to_video", in)
}

func (p *ProxyHandler) GetVoiceList(ctx context.Context, in *GetVoiceListRequest) (Output, error) {
	return p.call(ctx, "get_voice_list", struct{}{})
}

func (p *ProxyHandler) BatchTextToAudio(ctx context.Context, in *BatchTextToAudioRequest) (Output, error) {
	return p.call(ctx, "batch_text_to_audio", in)
}

func (p *ProxyHandler) BatchTextToMusic(ctx context.Context, in *BatchTextToMusicRequest) (Output, error) {
	return p.call(ctx, "batch_text_to_music", in)
}

func (p *ProxyHandler) SynthesizeSpeech(ctx context.Context, in *SynthesizeSpeechRequest) (Output, error) {
	return p.call(ctx, "synthesize_speech", in)
}

func (p *ProxyHandler) BatchSynthesizeSpeech(ctx context.Context, in *BatchSynthesizeSpeechRequest) (Output, error) {
	return p.call(ctx, "batch_synthesize_speech", in)
}

func (p *ProxyHandler) ListenAudio(ctx context.Context, in *ListenAudioRequest) (Output, error) {
	return p.call(ctx, "listen_audio", in)
}

func (p *ProxyHandler) ImagesUnderstand(ctx context.Context, in *ImagesUnderstandRequest) (Output, error) {
	return p.call(ctx, "images_understand", in)
}

func (p *ProxyHandler) AudiosUnderstand(ctx context.Context, in *AudiosUnderstandRequest) (Output, error) {
	return p.call(ctx, "audios_understand", in)
}

func (p *ProxyHandler) VideosUnderstand(ctx context.Context, in *VideosUnderstandRequest) (Output, error) {
	return p.call(ctx, "videos_understand", in)
}

func (p *ProxyHandler) ExtractContentFromWebsites(ctx context.Context, in *ExtractContentFromWebsitesRequest) (Output, error) {
	return p.call(ctx, "extract_content_from_websites", in)
}

func (p *ProxyHandler) BatchWebSearch(ctx context.Context, in *BatchWebSearchRequest) (Output, error) {
	return p.call(ctx, "batch_web_search", in)
}

func (p *ProxyHandler) ImageReverseSearch(ctx context.Context, in *ImageReverseSearchRequest) (Output, error) {
	return p.call(ctx, "image_reverse_search", in)
}

func (p *ProxyHandler) ImagesSearchAndDownload(ctx context.Context, in *ImagesSearchAndDownloadRequest) (Output, error) {
	return p.call(ctx, "images_search_and_download", in)
}

func (p *ProxyHandler) ImagesList(ctx context.Context, in *ImagesListRequest) (Output, error) {
	return p.call(ctx, "images_list", in)
}

func (p *ProxyHandler) Deploy(ctx context.Context, in *DeployRequest) (Output, error) {
	return p.call(ctx, "deploy", in)
}

// RemoteDeploy is a replica-only extension: the real matrix server has no
// such tool, so in proxy mode it is a hard error rather than a forwarded
// call the backend would reject with "tool not found".
func (p *ProxyHandler) RemoteDeploy(_ context.Context, _ *RemoteDeployRequest) (Output, error) {
	return nil, toolErr("remote_deploy is a replica extension; use deploy in proxy mode")
}

// UploadFile is a replica-only extension, mirroring RemoteDeploy.
func (p *ProxyHandler) UploadFile(_ context.Context, _ *UploadFileRequest) (Output, error) {
	return nil, toolErr("upload_file is a replica extension; use upload_to_cdn in proxy mode")
}

func (p *ProxyHandler) InitReactProject(ctx context.Context, in *InitReactProjectRequest) (Output, error) {
	return p.call(ctx, "init_react_project", in)
}

func (p *ProxyHandler) DeployHTMLPresentation(ctx context.Context, in *DeployHTMLPresentationRequest) (Output, error) {
	return p.call(ctx, "deploy_html_presentation", in)
}

func (p *ProxyHandler) UploadToCDN(ctx context.Context, in *UploadToCDNRequest) (Output, error) {
	return p.call(ctx, "upload_to_cdn", in)
}

var _ Handler = (*ProxyHandler)(nil)
