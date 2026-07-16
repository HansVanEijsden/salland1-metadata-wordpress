package handlers

import (
	"fmt"
	"net/http"

	"salland1-metadata-wordpress/internal/cache"
	"salland1-metadata-wordpress/internal/fetcher"
	"salland1-metadata-wordpress/internal/parser"
	"salland1-metadata-wordpress/pkg/utils"
)

type Handlers struct {
	cache   *cache.Cache
	fetcher *fetcher.Fetcher
}

func New(cache *cache.Cache, fetcher *fetcher.Fetcher) *Handlers {
	return &Handlers{
		cache:   cache,
		fetcher: fetcher,
	}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	// Check if HTTP server is running (implicitly true if handler is called)
	// Check if at least one successful fetch has completed
	// Check if cached data is available

	if h.fetcher.HasSuccessfulFetch() && h.cache.IsValid() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","cached":true,"fetches_completed":true}`))
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"unhealthy","cached":false,"fetches_completed":false}`))
	}
}

func (h *Handlers) getParsedData() (parser.ParsedData, bool) {
	data, ok := h.cache.Get()
	if !ok {
		return parser.ParsedData{}, false
	}
	return parser.Parse(data), true
}

func (h *Handlers) RadioFmPty(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.getParsedData()
	if !ok {
		http.Error(w, "No data available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(parsed.FmRdsPty))
}

func (h *Handlers) RadioFmPtyn(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.getParsedData()
	if !ok {
		http.Error(w, "No data available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(parsed.FmRdsPtyn))
}

func (h *Handlers) RadioFmProgramme(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.getParsedData()
	if !ok {
		http.Error(w, "No data available", http.StatusServiceUnavailable)
		return
	}

	if parsed.ShowName == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	hostStr := utils.FormatHosts(parsed.HostNames)

	var response string
	if hostStr != "" {
		response = fmt.Sprintf("15s:Dit is \\+33%s\\- met \\+36%s\\- op Salland1/15s:Check \\+WWSalland1.nl\\- voor het Sallandse nieuws",
			parsed.ShowName, hostStr)
	} else {
		response = fmt.Sprintf("15s:Nu \\+33%s\\- op \\+31Salland1\\-/15s:Check \\+WWSalland1.nl\\- voor het Sallandse nieuws",
			parsed.ShowName)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(response))
}

func (h *Handlers) RadioStreamProgramme(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.getParsedData()
	if !ok {
		http.Error(w, "No data available", http.StatusServiceUnavailable)
		return
	}

	if parsed.ShowName == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	hostStr := utils.FormatHosts(parsed.HostNames)

	var response string
	if hostStr != "" {
		response = fmt.Sprintf("%s met %s op Salland1 Radio", parsed.ShowName, hostStr)
	} else {
		response = fmt.Sprintf("Nu %s op Salland1 Radio", parsed.ShowName)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(response))
}

func (h *Handlers) RadioDabProgramme(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.getParsedData()
	if !ok {
		http.Error(w, "No data available", http.StatusServiceUnavailable)
		return
	}

	if parsed.ShowName == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Format next show time
	nextTimeFormatted := ""
	if parsed.NextShowTime != "" {
		nextTimeFormatted = utils.FormatTime(parsed.NextShowTime)
	}

	hostStr := utils.FormatHosts(parsed.HostNames)

	var response string
	if hostStr != "" {
		response = fmt.Sprintf("%s;%s;%s;%s", nextTimeFormatted, parsed.NextShowName, parsed.ShowName, hostStr)
	} else {
		response = fmt.Sprintf("%s;%s;%s", nextTimeFormatted, parsed.NextShowName, parsed.ShowName)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(response))
}

func (h *Handlers) RadioTvProgramme(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.getParsedData()
	if !ok {
		http.Error(w, "No data available", http.StatusServiceUnavailable)
		return
	}

	if parsed.ShowName == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(parsed.ShowName))
}

func (h *Handlers) RadioTvHost(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.getParsedData()
	if !ok {
		http.Error(w, "No data available", http.StatusServiceUnavailable)
		return
	}

	if parsed.ShowName == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	hostStr := utils.FormatHosts(parsed.HostNames)
	if hostStr == "" {
		hostStr = "Non-Stop"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(hostStr))
}
