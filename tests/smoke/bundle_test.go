package smoke_test

import (
	"testing"
)

type smokeBundleRow struct {
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Creator struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"creator"`
}

type smokeBundle struct {
	ID      int `json:"id"`
	Contact struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"contact"`
	Postings []struct {
		ID      int `json:"id"`
		TopicID int `json:"topic_id"`
	} `json:"postings"`
	NextPage string `json:"next_page"`
}

func imboxRows(t *testing.T) []smokeBundleRow {
	t.Helper()
	data := dataAs[struct {
		Postings []smokeBundleRow `json:"postings"`
	}](t, heyJSON(t, "box", "imbox", "--all"))
	return data.Postings
}

func scanForBundleRow(t *testing.T) (smokeBundleRow, bool) {
	t.Helper()
	for _, row := range imboxRows(t) {
		if row.Kind == "bundle" {
			return row, true
		}
	}
	return smokeBundleRow{}, false
}

// findBundleRow answers with an Imbox row of kind "bundle" — one sender's mail grouped
// into a single row. When the seed data has none, it bundles the sender of an Imbox
// thread to make one, and unbundles them again in cleanup.
func findBundleRow(t *testing.T) smokeBundleRow {
	t.Helper()
	if row, ok := scanForBundleRow(t); ok {
		return row
	}

	contactID := 0
	for _, row := range imboxRows(t) {
		if row.Kind != "bundle" && row.Creator.ID != 0 {
			contactID = row.Creator.ID
			break
		}
	}
	if contactID == 0 {
		skipf(t, "no imbox sender available to bundle")
	}
	id := intStr(contactID)
	_, stderr, code := hey(t, "contact", "bundle", id, "--json")
	if code != 0 {
		skipf(t, "contact bundle unavailable (exit %d): %s", code, stderr)
	}
	t.Cleanup(func() {
		_, cleanupStderr, cleanupCode := hey(t, "contact", "unbundle", id)
		if cleanupCode != 0 {
			t.Logf("could not unbundle contact %s: %s", id, cleanupStderr)
		}
	})
	if row, ok := scanForBundleRow(t); ok {
		return row
	}
	skipf(t, "bundling contact %d produced no bundle row in the Imbox", contactID)
	return smokeBundleRow{}
}

func TestBundleView(t *testing.T) {
	row := findBundleRow(t)
	bundle := dataAs[smokeBundle](t, heyJSON(t, "bundle", "view", intStr(row.ID)))
	if bundle.ID != row.ID || bundle.Contact.ID == 0 {
		t.Errorf("bundle view returned id=%d contact=%d, want row %d with its contact", bundle.ID, bundle.Contact.ID, row.ID)
	}
	for _, posting := range bundle.Postings {
		if posting.TopicID == 0 {
			t.Errorf("posting %d has no topic_id for hey thread read", posting.ID)
		}
	}

	limited := dataAs[smokeBundle](t, heyJSON(t, "bundle", "view", intStr(row.ID), "--limit", "1"))
	if len(limited.Postings) > 1 {
		t.Errorf("expected at most 1 posting with --limit 1, got %d", len(limited.Postings))
	}

	// The bundled contact's own list pages every thread, seen and unseen.
	threads := dataAs[struct {
		ID int `json:"id"`
	}](t, heyJSON(t, "contact", "threads", intStr(bundle.Contact.ID)))
	if threads.ID != bundle.Contact.ID {
		t.Errorf("contact threads returned id %d, want the bundled contact %d", threads.ID, bundle.Contact.ID)
	}
}

// A bundle row's own id names no topic, and hey thread read says so instead of
// answering "not found".
func TestThreadReadNamesABundleRowID(t *testing.T) {
	row := findBundleRow(t)
	_, stderr := heyFail(t, "thread", "read", intStr(row.ID), "--json")
	assertContains(t, stderr, "bundle")
}

func TestBundleNoArgumentShowsHelp(t *testing.T) {
	stdout := heyOK(t, "bundle")
	assertContains(t, stdout, "hey bundle view")
}

func TestBundleViewValidatesInput(t *testing.T) {
	heyFail(t, "bundle", "view", "not-an-id")
	heyFail(t, "bundle", "view", "0")
}
