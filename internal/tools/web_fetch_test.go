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

func TestWebFetchExtractsTitleAndText(t *testing.T) {
	ts, _ := NewToolset(t.TempDir())
	ts.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`
<html>
<head><title> Example Page </title></head>
<body><nav>skip me</nav><main><h1>Hello</h1><p>World</p></main></body>
</html>`)),
			Header:  make(http.Header),
			Request: req,
		}, nil
	})}
	res, err := ts.webFetch(context.Background(), core.ToolCall{ID: "1", Name: "web_fetch", Input: `{"url":"https://example.com"}`})
	if err != nil || res.IsError {
		t.Fatalf("web_fetch failed err=%v res=%+v", err, res)
	}

	var out struct {
		Data struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Data.Title != "Example Page" {
		t.Fatalf("bad title: %q", out.Data.Title)
	}
	if !strings.Contains(out.Data.Content, "Hello") || strings.Contains(out.Data.Content, "skip me") {
		t.Fatalf("unexpected extracted content: %q", out.Data.Content)
	}
}

func TestWebFetchInvalidAndHTTPError(t *testing.T) {
	ts, _ := NewToolset(t.TempDir())
	res, err := ts.webFetch(context.Background(), core.ToolCall{ID: "1", Name: "web_fetch", Input: `{"url":"ftp://example.com"}`})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "url scheme must be http or https") {
		t.Fatalf("expected invalid scheme, got: %s", res.Content)
	}

	ts.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("x")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	res2, err := ts.webFetch(context.Background(), core.ToolCall{ID: "2", Name: "web_fetch", Input: `{"url":"https://example.com"}`})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res2.IsError || !strings.Contains(res2.Content, "http 500") {
		t.Fatalf("expected http error, got: %s", res2.Content)
	}
}

func TestWebFetchRegistryIncludesTool(t *testing.T) {
	ts, _ := NewToolset(t.TempDir())
	found := false
	for _, td := range ts.Tools() {
		if td.Name() == "web_fetch" {
			found = true
			if !core.DescribeTool(td).ReadOnly {
				t.Fatal("web_fetch should be readOnly")
			}
			break
		}
	}
	if !found {
		t.Fatal("web_fetch not registered")
	}
}
