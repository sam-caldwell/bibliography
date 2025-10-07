package httpx

import "net/http"

// Doer is the minimal HTTP client interface used across packages.
type Doer interface {
    Do(req *http.Request) (*http.Response, error)
}

// ChromeUA is a consistent, modern desktop Chrome User-Agent for all outbound HTTP.
const ChromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// SetUA sets the ChromeUA header on the request.
func SetUA(req *http.Request) {
    if req != nil {
        req.Header.Set("User-Agent", ChromeUA)
    }
}
