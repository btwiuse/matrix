package matrix

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed schema.json
var schemaJSON []byte

// ToolSpec is one entry of the real matrix server's tools/list response.
// It is kept verbatim (including descriptions) for fidelity.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// LoadSpecs parses the embedded schema.json (captured from the real matrix
// MCP server) into tool definitions.
func LoadSpecs() ([]ToolSpec, error) {
	var raw map[string]struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	if err := json.Unmarshal(schemaJSON, &raw); err != nil {
		return nil, fmt.Errorf("parsing embedded schema: %w", err)
	}
	specs := make([]ToolSpec, 0, len(raw))
	for name, t := range raw {
		specs = append(specs, ToolSpec{Name: name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return specs, nil
}

// NewServer builds a go-sdk MCP server with all 22 matrix tools registered.
// Each tool uses the exact input schema from the real server, and dispatches
// to h (which may proxy to the real backend or serve locally).
func NewServer(h Handler) (*mcp.Server, error) {
	specs, err := LoadSpecs()
	if err != nil {
		return nil, err
	}
	if len(specs) != 22 {
		return nil, fmt.Errorf("expected 22 tools, got %d (schema drift?)", len(specs))
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "matrix-mcp-replica",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})

	byName := map[string]ToolSpec{}
	for _, s := range specs {
		byName[s.Name] = s
	}
	if err := registerAll(server, byName, h); err != nil {
		return nil, err
	}
	return server, nil
}

// register adds a single tool with the given name, schema and typed handler.
func register[In any](server *mcp.Server, spec ToolSpec, call func(context.Context, *In) (Output, error)) error {
	handle := func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := call(ctx, &in)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", spec.Name, err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(out)}},
		}, nil, nil
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        spec.Name,
		Description: spec.Description,
		InputSchema: spec.InputSchema,
	}, handle)
	return nil
}

// registerAll wires every one of the 22 tools to the Handler interface.
func registerAll(server *mcp.Server, byName map[string]ToolSpec, h Handler) error {
	regs := []struct {
		name string
		fn   func(*mcp.Server, ToolSpec, Handler) error
	}{
		{"image_synthesize", regImageSynthesize},
		{"gen_videos", regGenVideos},
		{"batch_text_to_video", regBatchTextToVideo},
		{"batch_image_to_video", regBatchImageToVideo},
		{"get_voice_list", regGetVoiceList},
		{"batch_text_to_audio", regBatchTextToAudio},
		{"batch_text_to_music", regBatchTextToMusic},
		{"synthesize_speech", regSynthesizeSpeech},
		{"batch_synthesize_speech", regBatchSynthesizeSpeech},
		{"listen_audio", regListenAudio},
		{"images_understand", regImagesUnderstand},
		{"audios_understand", regAudiosUnderstand},
		{"videos_understand", regVideosUnderstand},
		{"extract_content_from_websites", regExtractContentFromWebsites},
		{"batch_web_search", regBatchWebSearch},
		{"image_reverse_search", regImageReverseSearch},
		{"images_search_and_download", regImagesSearchAndDownload},
		{"images_list", regImagesList},
		{"deploy", regDeploy},
		{"init_react_project", regInitReactProject},
		{"deploy_html_presentation", regDeployHTMLPresentation},
		{"upload_to_cdn", regUploadToCDN},
	}
	for _, r := range regs {
		spec, ok := byName[r.name]
		if !ok {
			return fmt.Errorf("tool %q missing from embedded schema", r.name)
		}
		if err := r.fn(server, spec, h); err != nil {
			return err
		}
	}
	return nil
}
func regImageSynthesize(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *ImageSynthesizeRequest) (Output, error) {
		return h.ImageSynthesize(ctx, in)
	})
}

func regGenVideos(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *GenVideosRequest) (Output, error) {
		return h.GenVideos(ctx, in)
	})
}

func regBatchTextToVideo(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *BatchTextToVideoRequest) (Output, error) {
		return h.BatchTextToVideo(ctx, in)
	})
}

func regBatchImageToVideo(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *BatchImageToVideoRequest) (Output, error) {
		return h.BatchImageToVideo(ctx, in)
	})
}

func regGetVoiceList(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *GetVoiceListRequest) (Output, error) {
		return h.GetVoiceList(ctx, in)
	})
}

func regBatchTextToAudio(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *BatchTextToAudioRequest) (Output, error) {
		return h.BatchTextToAudio(ctx, in)
	})
}

func regBatchTextToMusic(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *BatchTextToMusicRequest) (Output, error) {
		return h.BatchTextToMusic(ctx, in)
	})
}

func regSynthesizeSpeech(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *SynthesizeSpeechRequest) (Output, error) {
		return h.SynthesizeSpeech(ctx, in)
	})
}

func regBatchSynthesizeSpeech(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *BatchSynthesizeSpeechRequest) (Output, error) {
		return h.BatchSynthesizeSpeech(ctx, in)
	})
}

func regListenAudio(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *ListenAudioRequest) (Output, error) {
		return h.ListenAudio(ctx, in)
	})
}

func regImagesUnderstand(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *ImagesUnderstandRequest) (Output, error) {
		return h.ImagesUnderstand(ctx, in)
	})
}

func regAudiosUnderstand(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *AudiosUnderstandRequest) (Output, error) {
		return h.AudiosUnderstand(ctx, in)
	})
}

func regVideosUnderstand(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *VideosUnderstandRequest) (Output, error) {
		return h.VideosUnderstand(ctx, in)
	})
}

func regExtractContentFromWebsites(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *ExtractContentFromWebsitesRequest) (Output, error) {
		return h.ExtractContentFromWebsites(ctx, in)
	})
}

func regBatchWebSearch(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *BatchWebSearchRequest) (Output, error) {
		return h.BatchWebSearch(ctx, in)
	})
}

func regImageReverseSearch(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *ImageReverseSearchRequest) (Output, error) {
		return h.ImageReverseSearch(ctx, in)
	})
}

func regImagesSearchAndDownload(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *ImagesSearchAndDownloadRequest) (Output, error) {
		return h.ImagesSearchAndDownload(ctx, in)
	})
}

func regImagesList(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *ImagesListRequest) (Output, error) {
		return h.ImagesList(ctx, in)
	})
}

func regDeploy(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *DeployRequest) (Output, error) {
		return h.Deploy(ctx, in)
	})
}

func regInitReactProject(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *InitReactProjectRequest) (Output, error) {
		return h.InitReactProject(ctx, in)
	})
}

func regDeployHTMLPresentation(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *DeployHTMLPresentationRequest) (Output, error) {
		return h.DeployHTMLPresentation(ctx, in)
	})
}

func regUploadToCDN(s *mcp.Server, spec ToolSpec, h Handler) error {
	return register(s, spec, func(ctx context.Context, in *UploadToCDNRequest) (Output, error) {
		return h.UploadToCDN(ctx, in)
	})
}
