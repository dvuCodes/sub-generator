package main

func buildBackendSummary(asrBackend, translationBackend string, diarizationEnabled bool) string {
	summary := asrBackend
	if translationBackend != "" && translationBackend != "none" {
		summary += " + " + translationBackend
	}
	if diarizationEnabled {
		summary += " + diarization"
	}
	return summary
}

func listCapabilities() CapabilitiesResponse {
	return defaultCapabilities()
}
