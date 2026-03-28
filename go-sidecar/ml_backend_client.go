package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const mlBackendRequestTimeout = 30 * time.Minute

type MLBackendClient struct {
	baseURL string
	client  *http.Client
}

func NewMLBackendClient(baseURL string) *MLBackendClient {
	return &MLBackendClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: mlBackendRequestTimeout,
		},
	}
}

func (c *MLBackendClient) Capabilities() (*CapabilitiesResponse, error) {
	resp, err := c.client.Get(c.baseURL + "/capabilities")
	if err != nil {
		return nil, fmt.Errorf("ml-backend capabilities request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ml-backend capabilities returned status %d: %s", resp.StatusCode, string(body))
	}

	var result CapabilitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ml-backend capabilities: %w", err)
	}

	return &result, nil
}

type asrTranscribeRequest struct {
	InputVideo string  `json:"input_video"`
	SourceLang *string `json:"source_lang,omitempty"`
	ModelID    string  `json:"model_id,omitempty"`
	BeamSize   int     `json:"beam_size,omitempty"`
	VADFilter  bool    `json:"vad_filter,omitempty"`
}

func (c *MLBackendClient) Transcribe(videoPath string, sourceLang *string, modelID string, beamSize int, vadFilter bool) (*TranscriptionResult, error) {
	bodyBytes, err := json.Marshal(asrTranscribeRequest{
		InputVideo: videoPath,
		SourceLang: sourceLang,
		ModelID:    modelID,
		BeamSize:   beamSize,
		VADFilter:  vadFilter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ml-backend ASR request: %w", err)
	}

	resp, err := c.client.Post(
		c.baseURL+"/asr/transcribe",
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("ml-backend ASR request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ml-backend ASR returned status %d: %s", resp.StatusCode, string(body))
	}

	var result TranscriptionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ml-backend ASR response: %w", err)
	}

	return &result, nil
}

type translateSegmentsRequest struct {
	Segments   []Segment `json:"segments"`
	SourceLang string    `json:"source_lang"`
	TargetLang string    `json:"target_lang"`
	ModelID    string    `json:"model_id,omitempty"`
}

type translateSegmentsResponse struct {
	Segments []Segment `json:"segments"`
}

func (c *MLBackendClient) TranslateSegments(segments []Segment, sourceLang, targetLang, modelID string) ([]Segment, error) {
	bodyBytes, err := json.Marshal(translateSegmentsRequest{
		Segments:   segments,
		SourceLang: sourceLang,
		TargetLang: targetLang,
		ModelID:    modelID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ml-backend translation request: %w", err)
	}

	resp, err := c.client.Post(
		c.baseURL+"/translation/translate_segments",
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("ml-backend translation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ml-backend translation returned status %d: %s", resp.StatusCode, string(body))
	}

	var result translateSegmentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ml-backend translation response: %w", err)
	}

	return result.Segments, nil
}

type diarizationAnnotateRequest struct {
	AudioPath string    `json:"audio_path"`
	Segments  []Segment `json:"segments"`
}

type diarizationAnnotateResponse struct {
	Segments     []Segment `json:"segments"`
	SpeakerCount int       `json:"speaker_count"`
}

func (c *MLBackendClient) AnnotateDiarization(audioPath string, segments []Segment) ([]Segment, int, error) {
	bodyBytes, err := json.Marshal(diarizationAnnotateRequest{
		AudioPath: audioPath,
		Segments:  segments,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal diarization request: %w", err)
	}

	resp, err := c.client.Post(
		c.baseURL+"/diarization/annotate",
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("ml-backend diarization request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("ml-backend diarization returned status %d: %s", resp.StatusCode, string(body))
	}

	var result diarizationAnnotateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("failed to decode diarization response: %w", err)
	}

	return result.Segments, result.SpeakerCount, nil
}
