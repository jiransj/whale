package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/core"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestFetchTextAndHTML(t *testing.T) {
	ts, err := NewToolset(t.TempDir())
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	ts.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<html><head><title>T</title></head><body><h1>Hello</h1><p>World</p></body></html>`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	resText, err := ts.fetch(context.Background(), core.ToolCall{
		ID:    "1",
		Name:  "fetch",
		Input: `{"url":"https://example.com","format":"text"}`,
	})
	if err != nil || resText.IsError {
		t.Fatalf("fetch text failed err=%v res=%+v", err, resText)
	}
	if !strings.Contains(resText.Content, "Hello") || strings.Contains(resText.Content, "<h1>") {
		t.Fatalf("expected extracted text, got: %s", resText.Content)
	}

	resHTML, err := ts.fetch(context.Background(), core.ToolCall{
		ID:    "2",
		Name:  "fetch",
		Input: `{"url":"https://example.com","format":"html"}`,
	})
	if err != nil || resHTML.IsError {
		t.Fatalf("fetch html failed err=%v res=%+v", err, resHTML)
	}
	var htmlOut struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resHTML.Content), &htmlOut); err != nil {
		t.Fatalf("unmarshal html output: %v", err)
	}
	if !strings.Contains(htmlOut.Data.Content, "<h1>Hello</h1>") {
		t.Fatalf("expected raw html, got: %s", htmlOut.Data.Content)
	}
}

func TestFetchInvalidAndHTTPError(t *testing.T) {
	ts, err := NewToolset(t.TempDir())
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	res, err := ts.fetch(context.Background(), core.ToolCall{ID: "1", Name: "fetch", Input: `{"url":"ftp://example.com"}`})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "url scheme must be http or https") {
		t.Fatalf("expected invalid scheme error, got: %s", res.Content)
	}

	ts.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("boom")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	res2, err := ts.fetch(context.Background(), core.ToolCall{ID: "2", Name: "fetch", Input: `{"url":"https://example.com"}`})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res2.IsError || !strings.Contains(res2.Content, "http 502") {
		t.Fatalf("expected http error, got: %s", res2.Content)
	}
}

func TestFetchRegistryIncludesTool(t *testing.T) {
	ts, err := NewToolset(t.TempDir())
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	found := false
	for _, td := range ts.Tools() {
		if td.Name() == "fetch" {
			found = true
			if !core.DescribeTool(td).ReadOnly {
				t.Fatal("fetch should be readOnly")
			}
			break
		}
	}
	if !found {
		t.Fatal("fetch not registered")
	}
}

func TestFetchTruncationMarker(t *testing.T) {
	large := strings.Repeat("a", maxFetchBytes+100)
	ts, _ := NewToolset(t.TempDir())
	ts.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(large)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	res, err := ts.fetch(context.Background(), core.ToolCall{ID: "1", Name: "fetch", Input: `{"url":"https://example.com"}`})
	if err != nil || res.IsError {
		t.Fatalf("fetch failed err=%v res=%+v", err, res)
	}
	var out struct {
		Data struct {
			Truncated bool   `json:"truncated"`
			Content   string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Data.Truncated || !strings.Contains(out.Data.Content, "[truncated]") {
		t.Fatalf("expected truncation marker, got: %s", res.Content)
	}
}
