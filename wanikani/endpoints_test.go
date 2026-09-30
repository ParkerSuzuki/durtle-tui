package wanikani

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSubjectsFollowsPages(t *testing.T) {
	var base string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_after_id") == "" {
			if got := r.URL.Query().Get("updated_after"); got != "2026-01-02T03:04:05Z" {
				t.Errorf("updated_after = %q", got)
			}
			fmt.Fprintf(w, `{"pages":{"next_url":%q},"data":[{"id":1,"object":"kanji","data":{"characters":"大"}}]}`,
				base+"subjects?page_after_id=1")
			return
		}
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":2,"object":"radical","data":{"characters":null}}]}`)
	})
	base = c.base
	got, err := c.Subjects(context.Background(), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || *got[0].Data.Characters != "大" || got[1].Data.Characters != nil {
		t.Errorf("got %+v", got)
	}
}

func TestReviewAssignmentsQuery(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assignments" || !strings.Contains(r.URL.RawQuery, "immediately_available_for_review") {
			t.Errorf("request = %s", r.URL)
		}
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":9,"object":"assignment","data":{"subject_id":1,"subject_type":"kanji"}}]}`)
	})
	got, err := c.ReviewAssignments(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != 9 || got[0].Data.SubjectID != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestSubmitReviewBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/reviews" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		var body struct {
			Review map[string]int `json:"review"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		want := map[string]int{"assignment_id": 9, "incorrect_meaning_answers": 1, "incorrect_reading_answers": 2}
		for k, v := range want {
			if body.Review[k] != v {
				t.Errorf("%s = %d, want %d", k, body.Review[k], v)
			}
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	})
	if err := c.SubmitReview(context.Background(), 9, 1, 2); err != nil {
		t.Fatal(err)
	}
}

func TestSummary(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/summary" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"object":"report","data":{"lessons":[{"available_at":"2026-09-30T16:00:00.000000Z","subject_ids":[1,2]}],"reviews":[{"available_at":"2026-09-30T16:00:00.000000Z","subject_ids":[3]},{"available_at":"2026-09-30T17:00:00.000000Z","subject_ids":[]}]}}`)
	})
	s, err := c.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Lessons) != 1 || len(s.Lessons[0].SubjectIDs) != 2 || len(s.Reviews) != 2 || s.Reviews[0].SubjectIDs[0] != 3 {
		t.Errorf("got %+v", s)
	}
}

func TestAssignmentsSince(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assignments" || r.URL.Query().Get("updated_after") == "" {
			t.Errorf("request = %s", r.URL)
		}
		fmt.Fprint(w, `{"pages":{"next_url":null},"data":[{"id":5,"object":"assignment","data":{"subject_id":1,"srs_stage":5,"started_at":"2026-01-01T00:00:00Z","passed_at":"2026-02-01T00:00:00Z","hidden":false}}]}`)
	})
	got, err := c.Assignments(context.Background(), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if a := got[0].Data; a.SRSStage != 5 || a.StartedAt == nil || a.PassedAt == nil {
		t.Errorf("decoded %+v", a)
	}
}

func TestSubjectLevelAndHidden(t *testing.T) {
	var s Subject
	if err := json.Unmarshal([]byte(`{"level":12,"hidden_at":"2026-01-01T00:00:00Z"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Level != 12 || s.HiddenAt == nil {
		t.Errorf("decoded %+v", s)
	}
}
