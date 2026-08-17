package matrix

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
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
	var raw map[string]ToolSpec
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
			// ToolError carries the exact JSON body the real server puts in
			// content[0].text of an error result (with isError=true).
			var te *ToolError
			if errors.As(err, &te) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: te.JSON}},
					IsError: te.IsError,
				}, nil, nil
			}
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
// Each binding is a single line: the typed Handler method is registered
// under its exact schema name.
func registerAll(server *mcp.Server, byName map[string]ToolSpec, h Handler) error {
	if err := reg1(server, byName, "image_synthesize", h.ImageSynthesize); err != nil {
		return err
	}
	if err := reg1(server, byName, "gen_videos", h.GenVideos); err != nil {
		return err
	}
	if err := reg1(server, byName, "batch_text_to_video", h.BatchTextToVideo); err != nil {
		return err
	}
	if err := reg1(server, byName, "batch_image_to_video", h.BatchImageToVideo); err != nil {
		return err
	}
	if err := reg1(server, byName, "get_voice_list", h.GetVoiceList); err != nil {
		return err
	}
	if err := reg1(server, byName, "batch_text_to_audio", h.BatchTextToAudio); err != nil {
		return err
	}
	if err := reg1(server, byName, "batch_text_to_music", h.BatchTextToMusic); err != nil {
		return err
	}
	if err := reg1(server, byName, "synthesize_speech", h.SynthesizeSpeech); err != nil {
		return err
	}
	if err := reg1(server, byName, "batch_synthesize_speech", h.BatchSynthesizeSpeech); err != nil {
		return err
	}
	if err := reg1(server, byName, "listen_audio", h.ListenAudio); err != nil {
		return err
	}
	if err := reg1(server, byName, "images_understand", h.ImagesUnderstand); err != nil {
		return err
	}
	if err := reg1(server, byName, "audios_understand", h.AudiosUnderstand); err != nil {
		return err
	}
	if err := reg1(server, byName, "videos_understand", h.VideosUnderstand); err != nil {
		return err
	}
	if err := reg1(server, byName, "extract_content_from_websites", h.ExtractContentFromWebsites); err != nil {
		return err
	}
	if err := reg1(server, byName, "batch_web_search", h.BatchWebSearch); err != nil {
		return err
	}
	if err := reg1(server, byName, "image_reverse_search", h.ImageReverseSearch); err != nil {
		return err
	}
	if err := reg1(server, byName, "images_search_and_download", h.ImagesSearchAndDownload); err != nil {
		return err
	}
	if err := reg1(server, byName, "images_list", h.ImagesList); err != nil {
		return err
	}
	if err := regDeploy(server, byName, h); err != nil {
		return err
	}
	if err := reg1(server, byName, "init_react_project", h.InitReactProject); err != nil {
		return err
	}
	if err := reg1(server, byName, "deploy_html_presentation", h.DeployHTMLPresentation); err != nil {
		return err
	}
	if err := reg1(server, byName, "upload_to_cdn", h.UploadToCDN); err != nil {
		return err
	}
	return nil
}

// regDeploy registers the deploy tool with a call-time schema that omits
// the required list: the real server does not enforce it (a missing
// dist_dir defaults to <workspace>/dist). tools/list still advertises the
// verbatim schema: the EnvelopeRewriter restores the required field in
// tools/list responses.
func regDeploy(server *mcp.Server, byName map[string]ToolSpec, h Handler) error {
	spec, ok := byName["deploy"]
	if !ok {
		return fmt.Errorf("tool %q missing from embedded schema", "deploy")
	}
	var m map[string]any
	if err := json.Unmarshal(spec.InputSchema, &m); err != nil {
		return fmt.Errorf("deploy input schema: %w", err)
	}
	delete(m, "required")
	lenient, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("deploy input schema: %w", err)
	}
	spec.InputSchema = lenient
	return register(server, spec, h.Deploy)
}

// reg1 looks up the tool spec by name and registers the typed handler call
// under the schema stored for that name.
func reg1[In any](server *mcp.Server, byName map[string]ToolSpec, name string, call func(context.Context, *In) (Output, error)) error {
	spec, ok := byName[name]
	if !ok {
		return fmt.Errorf("tool %q missing from embedded schema", name)
	}
	return register(server, spec, call)
}
