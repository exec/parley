package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
)

func handleGiphySearch(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("GIPHY_API_KEY")
	if apiKey == "" {
		http.Error(w, "giphy not configured", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query().Get("q")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "20"
	}
	rating := r.URL.Query().Get("rating")
	if rating == "" {
		rating = "g"
	}

	giphyURL := "https://api.giphy.com/v1/gifs/search?" + url.Values{
		"api_key": {apiKey},
		"q":       {q},
		"limit":   {limit},
		"rating":  {rating},
	}.Encode()

	proxyGiphy(w, giphyURL)
}

func handleGiphyTrending(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("GIPHY_API_KEY")
	if apiKey == "" {
		http.Error(w, "giphy not configured", http.StatusServiceUnavailable)
		return
	}
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "20"
	}
	rating := r.URL.Query().Get("rating")
	if rating == "" {
		rating = "g"
	}

	giphyURL := "https://api.giphy.com/v1/gifs/trending?" + url.Values{
		"api_key": {apiKey},
		"limit":   {limit},
		"rating":  {rating},
	}.Encode()

	proxyGiphy(w, giphyURL)
}

func proxyGiphy(w http.ResponseWriter, giphyURL string) {
	resp, err := http.Get(giphyURL) //nolint:noctx
	if err != nil {
		http.Error(w, "giphy request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}
