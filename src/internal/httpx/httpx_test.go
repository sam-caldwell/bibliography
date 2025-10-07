package httpx

import (
    "net/http"
    "testing"
)

func TestSetUA(t *testing.T) {
    req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
    if hv := req.Header.Get("User-Agent"); hv != "" {
        t.Fatalf("precondition: UA not empty: %q", hv)
    }
    SetUA(req)
    if hv := req.Header.Get("User-Agent"); hv != ChromeUA {
        t.Fatalf("SetUA: want %q, got %q", ChromeUA, hv)
    }
    // idempotent
    SetUA(req)
    if hv := req.Header.Get("User-Agent"); hv != ChromeUA {
        t.Fatalf("SetUA idempotent: want %q, got %q", ChromeUA, hv)
    }
}

