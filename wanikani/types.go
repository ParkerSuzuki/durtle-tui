package wanikani

import "time"

// Resource is the envelope every WaniKani object arrives in.
type Resource[T any] struct {
	ID            int       `json:"id"`
	Object        string    `json:"object"` // "radical", "kanji", "vocabulary", "kana_vocabulary", "assignment", ...
	DataUpdatedAt time.Time `json:"data_updated_at"`
	Data          T         `json:"data"`
}

// page is one page of a collection. NextURL is empty on the last page.
type page[T any] struct {
	Pages struct {
		NextURL string `json:"next_url"`
	} `json:"pages"`
	Data []Resource[T] `json:"data"`
}

type Subject struct {
	Characters        *string      `json:"characters"` // nil for image-only radicals
	Meanings          []Meaning    `json:"meanings"`
	AuxiliaryMeanings []AuxMeaning `json:"auxiliary_meanings"`
	Readings          []Reading    `json:"readings"`
}

type Meaning struct {
	Meaning        string `json:"meaning"`
	Primary        bool   `json:"primary"`
	AcceptedAnswer bool   `json:"accepted_answer"`
}

type AuxMeaning struct {
	Meaning string `json:"meaning"`
	Type    string `json:"type"` // "whitelist" or "blacklist"
}

type Reading struct {
	Reading        string `json:"reading"`
	Primary        bool   `json:"primary"`
	AcceptedAnswer bool   `json:"accepted_answer"`
	Type           string `json:"type"` // kanji only: "onyomi", "kunyomi", "nanori"
}

type Assignment struct {
	SubjectID   int    `json:"subject_id"`
	SubjectType string `json:"subject_type"`
}

type StudyMaterial struct {
	SubjectID       int      `json:"subject_id"`
	MeaningSynonyms []string `json:"meaning_synonyms"`
}

type User struct {
	Username string `json:"username"`
	Level    int    `json:"level"`
}
