package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MockHandler serves deterministic responses for every tool, so the replica
// works fully offline and its responses are inspectable in tests. It mirrors
// the output shapes of the real matrix server.
type MockHandler struct{}

// NewMockHandler builds a MockHandler.
func NewMockHandler() *MockHandler { return &MockHandler{} }

func mockOutput(v any) (Output, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (m *MockHandler) ImageSynthesize(_ context.Context, in *ImageSynthesizeRequest) (Output, error) {
	outs := make([]string, 0, len(in.Requests))
	for _, r := range in.Requests {
		outs = append(outs, r.OutputFile)
	}
	return mockOutput(map[string]any{"status": "ok", "output_files": outs})
}

func (m *MockHandler) GenVideos(_ context.Context, in *GenVideosRequest) (Output, error) {
	outs := make([]string, 0, len(in.VideoRequests))
	for _, r := range in.VideoRequests {
		outs = append(outs, r.OutputFile)
	}
	return mockOutput(map[string]any{"status": "ok", "output_files": outs})
}

func (m *MockHandler) BatchTextToVideo(_ context.Context, in *BatchTextToVideoRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "output_files": in.OutputFileList, "count": in.Count})
}

func (m *MockHandler) BatchImageToVideo(_ context.Context, in *BatchImageToVideoRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "output_files": in.OutputFileList, "count": in.Count})
}

func (m *MockHandler) GetVoiceList(_ context.Context, _ *GetVoiceListRequest) (Output, error) {
	return mockOutput(map[string]any{
		"available_voices": []map[string]string{
			{"voice_id": "male-qn-qingse", "voice_name": "青涩青年音色"},
			{"voice_id": "female-shaonv", "voice_name": "少女音色"},
			{"voice_id": "female-yujie", "voice_name": "御姐音色"},
		},
	})
}

func (m *MockHandler) BatchTextToAudio(_ context.Context, in *BatchTextToAudioRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "output_files": in.OutputFileList, "count": in.Count})
}

func (m *MockHandler) BatchTextToMusic(_ context.Context, in *BatchTextToMusicRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "output_files": in.OutputFileList, "count": in.Count})
}

// SynthesizeSpeech mirrors the real server's output shape (CDN url fields
// plus a completion message; there is no "status" key).
func (m *MockHandler) SynthesizeSpeech(_ context.Context, in *SynthesizeSpeechRequest) (Output, error) {
	return mockOutput(map[string]any{
		"output_file": in.OutputFile,
		"url":         "https://mock-cdn.example.com/audio/" + in.OutputFile,
		"url_clean":   "https://mock-cdn.example.com/audio/clean_" + in.OutputFile,
		"url_visible": "https://mock-cdn.example.com/audio/visible_" + in.OutputFile,
		"message":     "Speech synthesis completed",
	})
}

func (m *MockHandler) BatchSynthesizeSpeech(_ context.Context, in *BatchSynthesizeSpeechRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "output_files": in.OutputFileList, "count": in.Count})
}

func (m *MockHandler) ListenAudio(_ context.Context, _ *ListenAudioRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "transcript": "mock transcript"})
}

func (m *MockHandler) ImagesUnderstand(_ context.Context, in *ImagesUnderstandRequest) (Output, error) {
	res := make([]map[string]any, 0, len(in.ImageInfo))
	for _, i := range in.ImageInfo {
		res = append(res, map[string]any{"file": i.File, "description": "mock description"})
	}
	return mockOutput(map[string]any{"results": res})
}

func (m *MockHandler) AudiosUnderstand(_ context.Context, in *AudiosUnderstandRequest) (Output, error) {
	return mockOutput(map[string]any{"results": in.AudioInfo})
}

func (m *MockHandler) VideosUnderstand(_ context.Context, in *VideosUnderstandRequest) (Output, error) {
	return mockOutput(map[string]any{"results": in.VideoInfo})
}

func (m *MockHandler) ExtractContentFromWebsites(_ context.Context, in *ExtractContentFromWebsitesRequest) (Output, error) {
	res := make([]map[string]any, 0, len(in.Tasks))
	for _, t := range in.Tasks {
		res = append(res, map[string]any{"task_name": t.TaskName, "url": t.URL, "content": "mock extracted content"})
	}
	return mockOutput(map[string]any{"results": res})
}

func (m *MockHandler) BatchWebSearch(_ context.Context, in *BatchWebSearchRequest) (Output, error) {
	res := make([]map[string]any, 0, len(in.Queries))
	for _, q := range in.Queries {
		res = append(res, map[string]any{
			"query": q.Query,
			"results": []map[string]string{
				{"title": "Mock Result for " + q.Query, "url": "https://example.com/"},
			},
		})
	}
	return mockOutput(map[string]any{"results": res})
}

func (m *MockHandler) ImageReverseSearch(_ context.Context, in *ImageReverseSearchRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "output_file": in.OutputFile, "image_url": in.ImageURL})
}

func (m *MockHandler) ImagesSearchAndDownload(_ context.Context, in *ImagesSearchAndDownloadRequest) (Output, error) {
	res := make([]map[string]any, 0, len(in.Queries))
	for _, q := range in.Queries {
		res = append(res, map[string]any{"task_name": q.TaskName, "query": q.Query, "files": []string{"mock_image_1.png"}})
	}
	return mockOutput(map[string]any{"results": res})
}

// ImagesList mirrors the real server, which returns a markdown listing
// (not JSON) of the workspace images, honoring start/number pagination.
func (m *MockHandler) ImagesList(_ context.Context, in *ImagesListRequest) (Output, error) {
	names := []string{
		"profile.png", "og.png", "banner.png", "thumb.png", "hero.png",
		"avatar.png", "logo.png", "screenshot.png", "cover.png", "icon.png", "favicon.png",
	}
	start := in.Start
	if start < 0 {
		start = 0
	}
	if start > len(names) {
		start = len(names)
	}
	end := len(names)
	if in.Number > 0 && start+in.Number < end {
		end = start + in.Number
	}
	sel := names[start:end]
	var b strings.Builder
	fmt.Fprintf(&b, "# Total Images: %d", len(sel))
	for _, n := range sel {
		fmt.Fprintf(&b, "\n\n## Image %s\nFile: %s\nPath: /workspace/%s\nFile Size: 1.0 KB", n, n, n)
	}
	return []byte(b.String()), nil
}

func (m *MockHandler) Deploy(_ context.Context, _ *DeployRequest) (Output, error) {
	// Same output shape and formatting as the real server: random website
	// id, absolute URL without trailing slash, Python-style JSON.
	site := newSiteID(12)
	return deploySuccess(newWebsiteID(), "https://mock.example.com/"+site), nil
}

func (m *MockHandler) RemoteDeploy(_ context.Context, _ *RemoteDeployRequest) (Output, error) {
	site := newSiteID(12)
	return deploySuccess(newWebsiteID(), "https://mock.example.com/"+site), nil
}

func (m *MockHandler) InitReactProject(_ context.Context, in *InitReactProjectRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "project_name": in.ProjectName, "target_dir": in.TargetDir})
}

func (m *MockHandler) DeployHTMLPresentation(_ context.Context, in *DeployHTMLPresentationRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "url": "https://mock.example.com/slides", "slides_dir": in.SlidesDir})
}

func (m *MockHandler) UploadToCDN(_ context.Context, in *UploadToCDNRequest) (Output, error) {
	return mockOutput(map[string]any{"status": "ok", "cdn_url": "https://mock-cdn.example.com/" + in.FilePath})
}

var _ Handler = (*MockHandler)(nil)
