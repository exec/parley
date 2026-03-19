package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

var giphyAllowedRatings = map[string]bool{"g": true, "pg": true, "pg-13": true, "r": true}

func giphyParams(r *http.Request) (limit, rating string) {
	limit = r.URL.Query().Get("limit")
	if n, err := strconv.Atoi(limit); err != nil || n < 1 || n > 50 {
		limit = "20"
	}
	rating = r.URL.Query().Get("rating")
	if !giphyAllowedRatings[rating] {
		rating = "g"
	}
	return
}

func handleGiphySearch(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("GIPHY_API_KEY")
	if apiKey == "" {
		http.Error(w, "giphy not configured", http.StatusServiceUnavailable)
		return
	}
	limit, rating := giphyParams(r)
	giphyURL := "https://api.giphy.com/v1/gifs/search?" + url.Values{
		"api_key": {apiKey},
		"q":       {r.URL.Query().Get("q")},
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
	limit, rating := giphyParams(r)
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
