package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blitui/blit/btest"
)

// Session tests: record a scripted flow, save it to testdata/sessions,
// and assert the file round-trips via LoadSession. These act as a
// committed regression baseline — any breakage in the UI event pipeline
// that changes the step output trips the test on CI.
//
// Set GITSTREAM_UPDATE_SESSIONS=1 to re-record the golden files.

const sessionsDir = "../../testdata/sessions"

func sessionPath(name string) string {
	return filepath.Join(sessionsDir, name+".tuisess")
}

func recordAndVerify(t *testing.T, name string, play func(r *btest.SessionRecorder)) {
	t.Helper()
	path := sessionPath(name)
	force := os.Getenv("GITSTREAM_UPDATE_SESSIONS") == "1"

	if _, err := os.Stat(path); force || os.IsNotExist(err) {
		tm, _ := testApp(t, testEvents())
		rec := btest.NewSessionRecorder(tm)
		play(rec)
		if err := rec.Save(path); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}

	sess, err := btest.LoadSession(path)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	if len(sess.Steps) == 0 {
		t.Errorf("session %s has no steps", name)
	}
	if sess.Cols == 0 || sess.Lines == 0 {
		t.Errorf("session %s has zero viewport: %dx%d", name, sess.Cols, sess.Lines)
	}
}

// TestSession_RepoFeed covers the primary navigation flow: land on the
// event stream, move the cursor down, and return to the top.
func TestSession_RepoFeed(t *testing.T) {
	recordAndVerify(t, "repo_feed", func(r *btest.SessionRecorder) {
		r.Key("down").Key("down").Key("up")
	})
}

// TestSession_TypeFilter covers the filter cycling flow: advance through
// two type filters and clear back to the default.
func TestSession_TypeFilter(t *testing.T) {
	recordAndVerify(t, "type_filter", func(r *btest.SessionRecorder) {
		r.Key("t").Key("t").Key("0")
	})
}

// TestSession_SortToggle covers toggling between oldest-first and newest-first.
func TestSession_SortToggle(t *testing.T) {
	recordAndVerify(t, "sort_toggle", func(r *btest.SessionRecorder) {
		r.Key("s").Key("s")
	})
}
