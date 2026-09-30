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
	Characters        *string          `json:"characters"` // nil for image-only radicals
	Meanings          []Meaning        `json:"meanings"`
	AuxiliaryMeanings []AuxMeaning     `json:"auxiliary_meanings"`
	Readings          []Reading        `json:"readings"`
	CharacterImages   []CharacterImage `json:"character_images"` // for radicals with no characters
	Level             int              `json:"level"`
	HiddenAt          *time.Time       `json:"hidden_at"`

	MeaningMnemonic     string            `json:"meaning_mnemonic"`
	MeaningHint         string            `json:"meaning_hint"` // null decodes to ""
	ReadingMnemonic     string            `json:"reading_mnemonic"`
	ReadingHint         string            `json:"reading_hint"`
	ContextSentences    []ContextSentence `json:"context_sentences"`
	PartsOfSpeech       []string          `json:"parts_of_speech"`
	ComponentSubjectIDs []int             `json:"component_subject_ids"`
}

type ContextSentence struct {
	En string `json:"en"`
	Ja string `json:"ja"`
}

type CharacterImage struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"` // only image/svg+xml still downloads
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
	SubjectID   int        `json:"subject_id"`
	SubjectType string     `json:"subject_type"`
	SRSStage    int        `json:"srs_stage"` // 0 lesson not done, 1-4 apprentice, 5-6 guru, 7 master, 8 enlightened, 9 burned
	StartedAt   *time.Time `json:"started_at"`
	PassedAt    *time.Time `json:"passed_at"`
	Hidden      bool       `json:"hidden"`
}

// Summary is what is available now and over the next day, grouped by hour.
type Summary struct {
	Lessons []SummaryEntry `json:"lessons"`
	Reviews []SummaryEntry `json:"reviews"`
}

type SummaryEntry struct {
	AvailableAt time.Time `json:"available_at"`
	SubjectIDs  []int     `json:"subject_ids"`
}

type StudyMaterial struct {
	SubjectID       int      `json:"subject_id"`
	MeaningSynonyms []string `json:"meaning_synonyms"`
}

type User struct {
	Username    string      `json:"username"`
	Level       int         `json:"level"`
	Preferences Preferences `json:"preferences"`
}

type Preferences struct {
	LessonsBatchSize int `json:"lessons_batch_size"`
}
