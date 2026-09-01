package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// bundleUnseenHandler answers the bundle unseen route the way haystack's
// Postings::Bundles::UnseenController does: the bundled contact and one page of unseen
// postings, with a geared Link cursor while there are pages below.
func bundleUnseenHandler(t *testing.T, pages map[string]string, links map[string]string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/postings/9/bundles/unseen.json" {
			t.Errorf("request = %s %s, want GET /postings/9/bundles/unseen.json", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		cursor := r.URL.Query().Get("page")
		body, ok := pages[cursor]
		if !ok {
			t.Errorf("no page set up for cursor %q", cursor)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if next, ok := links[cursor]; ok {
			w.Header().Set("Link", fmt.Sprintf("<http://%s/postings/9/bundles/unseen.json?page=%s>; rel=\"next\"", r.Host, next))
		}
		_, _ = io.WriteString(w, body)
	}
}

func TestBundleViewListsTheUnseenThreads(t *testing.T) {
	response, err := runJSONCommand(t, bundleUnseenHandler(t,
		map[string]string{"": `{"contact":{"id":5,"name":"GitHub"},"postings":[
			{"id":301,"summary":"CI failed","app_url":"https://app.hey.com/topics/881"},
			{"id":302,"summary":"CI fixed","app_url":"https://app.hey.com/topics/882"}]}`},
		map[string]string{"": "cursor-2"},
	), "bundle", "view", "9")
	if err != nil {
		t.Fatalf("execute bundle view: %v", err)
	}

	if response.Summary != "2 unseen threads bundled from GitHub" {
		t.Errorf("summary = %q", response.Summary)
	}
	data, ok := response.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %T", response.Data)
	}
	contact, _ := data["contact"].(map[string]any)
	if contact["id"] != float64(5) {
		t.Errorf("contact = %v, want id 5 for hey contact threads", contact)
	}
	postings, _ := data["postings"].([]any)
	if len(postings) != 2 {
		t.Fatalf("postings = %v", postings)
	}
	first, _ := postings[0].(map[string]any)
	if first["topic_id"] != float64(881) {
		t.Errorf("first posting = %v, want topic_id 881", first)
	}
	if data["next_page"] != "cursor-2" {
		t.Errorf("next_page = %v", data["next_page"])
	}
}

func TestBundleViewFollowsTheCursorWithAll(t *testing.T) {
	response, err := runJSONCommand(t, bundleUnseenHandler(t,
		map[string]string{
			"":         `{"contact":{"id":5,"name":"GitHub"},"postings":[{"id":301,"app_url":"https://app.hey.com/topics/881"}]}`,
			"cursor-2": `{"contact":{"id":5,"name":"GitHub"},"postings":[{"id":302,"app_url":"https://app.hey.com/topics/882"}]}`,
		},
		map[string]string{"": "cursor-2"},
	), "bundle", "view", "9", "--all")
	if err != nil {
		t.Fatalf("execute bundle view --all: %v", err)
	}

	if response.Summary != "2 unseen threads bundled from GitHub" {
		t.Errorf("summary = %q", response.Summary)
	}
	data, _ := response.Data.(map[string]any)
	if next, ok := data["next_page"]; ok {
		t.Errorf("next_page = %v, want none after the last page", next)
	}
}

// A bundle with no unseen threads is not empty — it has been read — and the listing
// says where its mail lives instead of leaving zero rows to read as no mail.
func TestBundleViewSaysWhereAReadBundlesMailLives(t *testing.T) {
	response, err := runJSONCommand(t, bundleUnseenHandler(t,
		map[string]string{"": `{"contact":{"id":5,"name":"GitHub"},"postings":[]}`},
		nil,
	), "bundle", "view", "9")
	if err != nil {
		t.Fatalf("execute bundle view: %v", err)
	}
	if !strings.Contains(response.Notice, "no unseen threads") || !strings.Contains(response.Notice, "hey contact threads 5") {
		t.Errorf("notice = %q, want the contact threads pointer", response.Notice)
	}
}

// The unseen route answers only for postings that are bundles, so a not-found means
// the ID was something else — and the error says what the route wants.
func TestBundleViewRefusesANonBundleID(t *testing.T) {
	_, err := runJSONCommand(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}), "bundle", "view", "9")
	if err == nil || !strings.Contains(err.Error(), `bundle "9" not found`) {
		t.Fatalf("error = %v, want a bundle not-found", err)
	}
}

func TestContactThreadsListsEveryThread(t *testing.T) {
	response, err := runJSONCommand(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/contacts/5.json" {
			t.Errorf("request = %s %s, want GET /contacts/5.json", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if cursor := r.URL.Query().Get("page"); cursor != "cursor-2" {
			t.Errorf("cursor = %q, want cursor-2", cursor)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":5,"name":"GitHub","email_address":"noreply@github.com",
			"entries_title":"All threads with GitHub",
			"postings":[{"id":401,"summary":"CI failed","app_url":"https://app.hey.com/topics/881"}]}`)
	}), "contact", "threads", "5", "--page", "cursor-2")
	if err != nil {
		t.Fatalf("execute contact threads: %v", err)
	}

	if response.Summary != "1 thread with GitHub" {
		t.Errorf("summary = %q", response.Summary)
	}
	data, ok := response.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %T", response.Data)
	}
	if data["entries_title"] != "All threads with GitHub" {
		t.Errorf("entries_title = %v", data["entries_title"])
	}
	postings, _ := data["postings"].([]any)
	if len(postings) != 1 {
		t.Fatalf("postings = %v", postings)
	}
	first, _ := postings[0].(map[string]any)
	if first["topic_id"] != float64(881) {
		t.Errorf("first posting = %v, want topic_id 881", first)
	}
}

// A thread read that 404s on a bundle's own id says what the id really is and where
// the mail lives, instead of leaving "not found" to read as "no content".
func TestThreadReadNamesABundleMisread(t *testing.T) {
	_, err := runJSONCommand(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/topics/9/entries.json":
			http.NotFound(w, r)
		case "/postings/9/bundles/unseen.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"contact":{"id":5,"name":"GitHub"},"postings":[]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}), "thread", "read", "9")
	if err == nil || !strings.Contains(err.Error(), "9 is a bundle, not a thread") {
		t.Fatalf("error = %v, want the bundle named", err)
	}
	if !strings.Contains(err.Error(), "GitHub") {
		t.Errorf("error = %v, want the bundled contact named", err)
	}
}

// An id that is neither a thread nor a bundle keeps its own not-found: the probe stays
// on the error path and changes nothing it cannot improve.
func TestThreadReadKeepsAPlainNotFound(t *testing.T) {
	_, err := runJSONCommand(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}), "thread", "read", "9")
	if err == nil || strings.Contains(err.Error(), "bundle") {
		t.Fatalf("error = %v, want the original not-found", err)
	}
}
