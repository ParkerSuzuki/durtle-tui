package wanikani

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// getAll fetches every page of a collection, following pages.next_url.
func getAll[T any](ctx context.Context, c *Client, path string) ([]Resource[T], error) {
	var all []Resource[T]
	for next := c.base + path; next != ""; {
		var p page[T]
		if err := c.do(ctx, http.MethodGet, next, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		next = p.Pages.NextURL
	}
	return all, nil
}

func updatedAfter(path string, t time.Time) string {
	if t.IsZero() {
		return path
	}
	return path + "?updated_after=" + url.QueryEscape(t.UTC().Format(time.RFC3339))
}

// User fetches the token owner's profile. Used to check a token is valid.
func (c *Client) User(ctx context.Context) (User, error) {
	var r Resource[User]
	err := c.do(ctx, http.MethodGet, c.base+"user", nil, &r)
	return r.Data, err
}

// Subjects fetches subjects changed after t (all of them if t is zero).
func (c *Client) Subjects(ctx context.Context, t time.Time) ([]Resource[Subject], error) {
	return getAll[Subject](ctx, c, updatedAfter("subjects", t))
}

// StudyMaterials fetches the user's notes and synonyms changed after t.
func (c *Client) StudyMaterials(ctx context.Context, t time.Time) ([]Resource[StudyMaterial], error) {
	return getAll[StudyMaterial](ctx, c, updatedAfter("study_materials", t))
}

// ReviewAssignments fetches assignments that are due for review right now.
func (c *Client) ReviewAssignments(ctx context.Context) ([]Resource[Assignment], error) {
	return getAll[Assignment](ctx, c, "assignments?immediately_available_for_review=true&hidden=false")
}

// Summary fetches lessons and reviews available now and per hour for the next day.
func (c *Client) Summary(ctx context.Context) (Summary, error) {
	var r Resource[Summary]
	err := c.do(ctx, http.MethodGet, c.base+"summary", nil, &r)
	return r.Data, err
}

// Assignments fetches assignments changed after t (all of them if t is zero).
func (c *Client) Assignments(ctx context.Context, t time.Time) ([]Resource[Assignment], error) {
	return getAll[Assignment](ctx, c, updatedAfter("assignments", t))
}

// SubmitReview records a finished review for one assignment.
func (c *Client) SubmitReview(ctx context.Context, assignmentID, incorrectMeaning, incorrectReading int) error {
	body := map[string]any{"review": map[string]int{
		"assignment_id":             assignmentID,
		"incorrect_meaning_answers": incorrectMeaning,
		"incorrect_reading_answers": incorrectReading,
	}}
	return c.do(ctx, http.MethodPost, c.base+"reviews", body, nil)
}

// StartAssignment starts a lesson: the item moves to Apprentice 1 and
// enters the review queue. Needs the assignments:start token permission.
func (c *Client) StartAssignment(ctx context.Context, assignmentID int) error {
	body := map[string]any{"assignment": map[string]any{}}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("%sassignments/%d/start", c.base, assignmentID), body, nil)
}
