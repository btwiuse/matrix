package matrix

import "context"

// Output is the raw JSON output of a tool call. The real matrix server
// returns tool results as JSON text content; to preserve fidelity the
// replica passes the raw bytes through unchanged.
type Output = []byte

// Handler is the interface layer between the MCP server and the tool
// implementations. It mirrors the 22 tools of the real matrix MCP server,
// each method taking the typed input and returning the raw JSON output.
type Handler interface {
	ImageSynthesize(ctx context.Context, in *ImageSynthesizeRequest) (Output, error)
	GenVideos(ctx context.Context, in *GenVideosRequest) (Output, error)
	BatchTextToVideo(ctx context.Context, in *BatchTextToVideoRequest) (Output, error)
	BatchImageToVideo(ctx context.Context, in *BatchImageToVideoRequest) (Output, error)
	GetVoiceList(ctx context.Context, in *GetVoiceListRequest) (Output, error)
	BatchTextToAudio(ctx context.Context, in *BatchTextToAudioRequest) (Output, error)
	BatchTextToMusic(ctx context.Context, in *BatchTextToMusicRequest) (Output, error)
	SynthesizeSpeech(ctx context.Context, in *SynthesizeSpeechRequest) (Output, error)
	BatchSynthesizeSpeech(ctx context.Context, in *BatchSynthesizeSpeechRequest) (Output, error)
	ListenAudio(ctx context.Context, in *ListenAudioRequest) (Output, error)
	ImagesUnderstand(ctx context.Context, in *ImagesUnderstandRequest) (Output, error)
	AudiosUnderstand(ctx context.Context, in *AudiosUnderstandRequest) (Output, error)
	VideosUnderstand(ctx context.Context, in *VideosUnderstandRequest) (Output, error)
	ExtractContentFromWebsites(ctx context.Context, in *ExtractContentFromWebsitesRequest) (Output, error)
	BatchWebSearch(ctx context.Context, in *BatchWebSearchRequest) (Output, error)
	ImageReverseSearch(ctx context.Context, in *ImageReverseSearchRequest) (Output, error)
	ImagesSearchAndDownload(ctx context.Context, in *ImagesSearchAndDownloadRequest) (Output, error)
	ImagesList(ctx context.Context, in *ImagesListRequest) (Output, error)
	Deploy(ctx context.Context, in *DeployRequest) (Output, error)
	InitReactProject(ctx context.Context, in *InitReactProjectRequest) (Output, error)
	DeployHTMLPresentation(ctx context.Context, in *DeployHTMLPresentationRequest) (Output, error)
	UploadToCDN(ctx context.Context, in *UploadToCDNRequest) (Output, error)
}
