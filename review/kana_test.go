package review

import "testing"

func TestToHiragana(t *testing.T) {
	tests := []struct {
		in    string
		final bool
		want  string
	}{
		{"kanji", true, "かんじ"},
		{"onna", true, "おんな"},
		{"konnichiha", true, "こんにちは"},
		{"kitte", true, "きって"},
		{"zasshi", true, "ざっし"},
		{"matcha", true, "まっちゃ"},
		{"gakkou", true, "がっこう"},
		{"shinnyuu", true, "しんにゅう"},
		{"kyou", true, "きょう"},
		{"KANA", true, "かな"},
		{"ra-men", true, "らーめん"},
		{"n'a", true, "んあ"},
		{"hon", true, "ほん"},
		{"hon", false, "ほn"},
		{"honn", false, "ほnn"},
		{"ky", false, "ky"},
		{"かn", true, "かん"},
		{"shimbun", true, "しんぶん"},
		{"sampo", true, "さんぽ"},
		{"vu", true, "ゔ"},
		{"dya", true, "ぢゃ"},
		{"dyu", true, "ぢゅ"},
		{"xtsu", true, "っ"},
		{"wi", true, "うぃ"},
		{"shimb", false, "しんb"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ToHiragana(tt.in, tt.final); got != tt.want {
				t.Errorf("ToHiragana(%q, %v) = %q, want %q", tt.in, tt.final, got, tt.want)
			}
		})
	}
}

// TestLiveTyping feeds one key at a time, the way the text input does.
func TestLiveTyping(t *testing.T) {
	for _, tt := range []struct{ keys, want string }{
		{"onna", "おんな"},
		{"kitte", "きって"},
		{"shinnyuu", "しんにゅう"},
		{"hon", "ほん"},
		{"shimbun", "しんぶん"},
		{"xtsu", "っ"},
	} {
		v := ""
		for _, k := range tt.keys {
			v = ToHiragana(v+string(k), false)
		}
		if got := ToHiragana(v, true); got != tt.want {
			t.Errorf("typing %q gave %q, want %q", tt.keys, got, tt.want)
		}
	}
}

func TestKatakanaToHiragana(t *testing.T) {
	if got := KatakanaToHiragana("ラーメン"); got != "らーめん" {
		t.Errorf("got %q", got)
	}
}
