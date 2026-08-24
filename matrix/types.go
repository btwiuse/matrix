// Package matrix implements a high-fidelity replica of the MiniMax matrix
// MCP server, built on the modelcontextprotocol/go-sdk.
package matrix

// ImageRequest is one entry of the image_synthesize requests array.
type ImageRequest struct {
	Prompt      string   `json:"prompt"`
	InputFiles  []string `json:"input_files,omitempty"`
	InputURLs   []string `json:"input_urls,omitempty"`
	OutputFile  string   `json:"output_file"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
}

// ImageSynthesizeRequest is the input of image_synthesize.
type ImageSynthesizeRequest struct {
	Requests []ImageRequest `json:"requests"`
}

// VideoRequest is one entry of the gen_videos video_requests array.
type VideoRequest struct {
	Prompt        string `json:"prompt"`
	OutputFile    string `json:"output_file"`
	ImageFile     string `json:"image_file,omitempty"`
	ReferenceType string `json:"reference_type,omitempty"`
	Duration      int    `json:"duration,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
}

// GenVideosRequest is the input of gen_videos.
type GenVideosRequest struct {
	VideoRequests []VideoRequest `json:"video_requests"`
}

// BatchTextToVideoRequest is the input of batch_text_to_video.
type BatchTextToVideoRequest struct {
	Count          int      `json:"count"`
	PromptList     []string `json:"prompt_list"`
	OutputFileList []string `json:"output_file_list"`
	DurationList   []int    `json:"duration_list,omitempty"`
	ResolutionList []string `json:"resolution_list,omitempty"`
}

// BatchImageToVideoRequest is the input of batch_image_to_video.
type BatchImageToVideoRequest struct {
	Count             int      `json:"count"`
	ImageFileList     []string `json:"image_file_list"`
	OutputFileList    []string `json:"output_file_list"`
	PromptList        []string `json:"prompt_list,omitempty"`
	ReferenceTypeList []string `json:"reference_type_list,omitempty"`
	DurationList      []int    `json:"duration_list,omitempty"`
	ResolutionList    []string `json:"resolution_list,omitempty"`
}

// BatchTextToAudioRequest is the input of batch_text_to_audio.
type BatchTextToAudioRequest struct {
	Count          int       `json:"count"`
	TextList       []string  `json:"text_list"`
	OutputFileList []string  `json:"output_file_list"`
	SpeedList      []float64 `json:"speed_list,omitempty"`
	VolumeList     []float64 `json:"volume_list,omitempty"`
	PitchList      []float64 `json:"pitch_list,omitempty"`
	VoiceList      []string  `json:"voice_list,omitempty"`
	EmotionList    []string  `json:"emotion_list,omitempty"`
}

// BatchTextToMusicRequest is the input of batch_text_to_music.
type BatchTextToMusicRequest struct {
	Count          int      `json:"count"`
	PromptList     []string `json:"prompt_list"`
	LyricsList     []string `json:"lyrics_list,omitempty"`
	OutputFileList []string `json:"output_file_list"`
	SampleRateList []int    `json:"sample_rate_list,omitempty"`
	BitrateList    []int    `json:"bitrate_list,omitempty"`
	FormatList     []string `json:"format_list,omitempty"`
}

// SynthesizeSpeechRequest is the input of synthesize_speech.
type SynthesizeSpeechRequest struct {
	Text       string   `json:"text"`
	OutputFile string   `json:"output_file"`
	VoiceID    string   `json:"voice_id,omitempty"`
	Speed      *float64 `json:"speed,omitempty"`
	Volume     *float64 `json:"volume,omitempty"`
	Pitch      *int     `json:"pitch,omitempty"`
}

// BatchSynthesizeSpeechRequest is the input of batch_synthesize_speech.
type BatchSynthesizeSpeechRequest struct {
	Count          int       `json:"count"`
	TextList       []string  `json:"text_list"`
	OutputFileList []string  `json:"output_file_list"`
	VoiceIDList    []string  `json:"voice_id_list,omitempty"`
	SpeedList      []float64 `json:"speed_list,omitempty"`
	VolumeList     []float64 `json:"volume_list,omitempty"`
	PitchList      []int     `json:"pitch_list,omitempty"`
}

// ListenAudioRequest is the input of listen_audio.
type ListenAudioRequest struct {
	File string `json:"file,omitempty"`
	URL  string `json:"url,omitempty"`
}

// MediaInfo is one entry of the *_understand image_info/audio_info/video_info arrays.
type MediaInfo struct {
	File   string `json:"file,omitempty"`
	URL    string `json:"url,omitempty"`
	Prompt string `json:"prompt"`
}

// ImagesUnderstandRequest is the input of images_understand.
type ImagesUnderstandRequest struct {
	ImageInfo []MediaInfo `json:"image_info"`
}

// AudiosUnderstandRequest is the input of audios_understand.
type AudiosUnderstandRequest struct {
	AudioInfo []MediaInfo `json:"audio_info"`
}

// VideosUnderstandRequest is the input of videos_understand.
type VideosUnderstandRequest struct {
	VideoInfo []MediaInfo `json:"video_info"`
}

// ExtractTask is one entry of the extract_content_from_websites tasks array.
type ExtractTask struct {
	URL      string `json:"url"`
	Prompt   string `json:"prompt"`
	TaskName string `json:"task_name,omitempty"`
}

// ExtractContentFromWebsitesRequest is the input of extract_content_from_websites.
type ExtractContentFromWebsitesRequest struct {
	Tasks []ExtractTask `json:"tasks"`
	Mode  string        `json:"mode,omitempty"`
}

// SearchQuery is one entry of the batch_web_search queries array.
type SearchQuery struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results,omitempty"`
	Cursor     int    `json:"cursor,omitempty"`
	DataRange  string `json:"data_range,omitempty"`
}

// BatchWebSearchRequest is the input of batch_web_search.
type BatchWebSearchRequest struct {
	Queries    []SearchQuery `json:"queries"`
	SearchType string        `json:"search_type,omitempty"`
}

// ImageReverseSearchRequest is the input of image_reverse_search.
type ImageReverseSearchRequest struct {
	ImageURL   string `json:"image_url"`
	OutputFile string `json:"output_file"`
}

// DownloadQuery is one entry of the images_search_and_download queries array.
type DownloadQuery struct {
	Query    string `json:"query"`
	Prompt   string `json:"prompt,omitempty"`
	TaskName string `json:"task_name,omitempty"`
}

// ImagesSearchAndDownloadRequest is the input of images_search_and_download.
type ImagesSearchAndDownloadRequest struct {
	Queries []DownloadQuery `json:"queries"`
}

// ImagesListRequest is the input of images_list.
type ImagesListRequest struct {
	Start  int `json:"start,omitempty"`
	Number int `json:"number,omitempty"`
}

// DeployRequest is the input of deploy.
type DeployRequest struct {
	ProjectName string `json:"project_name,omitempty"`
	DistDir     string `json:"dist_dir"`
	ProjectType string `json:"project_type,omitempty"`
}

// RemoteDeployRequest is the input of remote_deploy. Exactly one of
// ArchiveURL and ArchiveData must be set: the archive (.tar.gz or .zip)
// carries the built site with index.html at its root, and is deployed the
// same way deploy publishes a workspace directory.
type RemoteDeployRequest struct {
	// ArchiveURL is an http(s) URL the server downloads the archive from.
	ArchiveURL string `json:"archive_url,omitempty"`
	// ArchiveData is the base64-encoded archive, for callers that cannot
	// expose a public URL.
	ArchiveData string `json:"archive_data,omitempty"`
	// ProjectName is advisory only, like deploy's project_name.
	ProjectName string `json:"project_name,omitempty"`
}

// InitReactProjectRequest is the input of init_react_project.
type InitReactProjectRequest struct {
	ProjectName string `json:"project_name"`
	TargetDir   string `json:"target_dir"`
}

// DeployHTMLPresentationRequest is the input of deploy_html_presentation.
type DeployHTMLPresentationRequest struct {
	SlidesDir string   `json:"slides_dir"`
	HTMLFiles []string `json:"html_files,omitempty"`
}

// UploadToCDNRequest is the input of upload_to_cdn.
type UploadToCDNRequest struct {
	FilePath string `json:"file_path"`
}

// GetVoiceListRequest is the (empty) input of get_voice_list.
type GetVoiceListRequest struct{}
