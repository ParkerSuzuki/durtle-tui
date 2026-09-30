package review

import (
	"strings"
	"unicode/utf8"
)

// romaji maps syllables to hiragana. Keys are at most 4 bytes; ToHiragana
// tries the longest match first.
var romaji = map[string]string{
	"a": "あ", "i": "い", "u": "う", "e": "え", "o": "お",
	"ka": "か", "ki": "き", "ku": "く", "ke": "け", "ko": "こ",
	"ga": "が", "gi": "ぎ", "gu": "ぐ", "ge": "げ", "go": "ご",
	"sa": "さ", "si": "し", "shi": "し", "su": "す", "se": "せ", "so": "そ",
	"za": "ざ", "zi": "じ", "ji": "じ", "zu": "ず", "ze": "ぜ", "zo": "ぞ",
	"ta": "た", "ti": "ち", "chi": "ち", "tu": "つ", "tsu": "つ", "te": "て", "to": "と",
	"da": "だ", "di": "ぢ", "du": "づ", "de": "で", "do": "ど",
	"na": "な", "ni": "に", "nu": "ぬ", "ne": "ね", "no": "の",
	"ha": "は", "hi": "ひ", "hu": "ふ", "fu": "ふ", "he": "へ", "ho": "ほ",
	"ba": "ば", "bi": "び", "bu": "ぶ", "be": "べ", "bo": "ぼ",
	"pa": "ぱ", "pi": "ぴ", "pu": "ぷ", "pe": "ぺ", "po": "ぽ",
	"ma": "ま", "mi": "み", "mu": "む", "me": "め", "mo": "も",
	"ya": "や", "yu": "ゆ", "yo": "よ",
	"ra": "ら", "ri": "り", "ru": "る", "re": "れ", "ro": "ろ",
	"wa": "わ", "wo": "を",
	"kya": "きゃ", "kyu": "きゅ", "kyo": "きょ",
	"gya": "ぎゃ", "gyu": "ぎゅ", "gyo": "ぎょ",
	"sha": "しゃ", "shu": "しゅ", "sho": "しょ", "she": "しぇ",
	"sya": "しゃ", "syu": "しゅ", "syo": "しょ",
	"ja": "じゃ", "ju": "じゅ", "jo": "じょ", "je": "じぇ",
	"jya": "じゃ", "jyu": "じゅ", "jyo": "じょ",
	"zya": "じゃ", "zyu": "じゅ", "zyo": "じょ",
	"cha": "ちゃ", "chu": "ちゅ", "cho": "ちょ", "che": "ちぇ",
	"tya": "ちゃ", "tyu": "ちゅ", "tyo": "ちょ",
	"nya": "にゃ", "nyu": "にゅ", "nyo": "にょ",
	"hya": "ひゃ", "hyu": "ひゅ", "hyo": "ひょ",
	"bya": "びゃ", "byu": "びゅ", "byo": "びょ",
	"pya": "ぴゃ", "pyu": "ぴゅ", "pyo": "ぴょ",
	"mya": "みゃ", "myu": "みゅ", "myo": "みょ",
	"rya": "りゃ", "ryu": "りゅ", "ryo": "りょ",
	"fa": "ふぁ", "fi": "ふぃ", "fe": "ふぇ", "fo": "ふぉ",
	"xa": "ぁ", "xi": "ぃ", "xu": "ぅ", "xe": "ぇ", "xo": "ぉ",
	"xya": "ゃ", "xyu": "ゅ", "xyo": "ょ", "xtu": "っ", "ltu": "っ", "xtsu": "っ",
	"va": "ゔぁ", "vi": "ゔぃ", "vu": "ゔ", "ve": "ゔぇ", "vo": "ゔぉ",
	"dya": "ぢゃ", "dyu": "ぢゅ", "dyo": "ぢょ",
	"wi": "うぃ", "we": "うぇ",
	"-": "ー",
}

func isVowel(c byte) bool { return strings.IndexByte("aiueo", c) >= 0 }

// ToHiragana converts romaji in s to hiragana, leaving anything it does not
// recognize (including kana already converted) untouched. While the user is
// still typing (final is false), a trailing "n" or "nn" stays as romaji,
// because the next key decides whether it is ん or the start of な, にゃ, etc.
func ToHiragana(s string, final bool) string {
	in := strings.ToLower(s)
	var b strings.Builder
	for i := 0; i < len(in); {
		rest := in[i:]
		c := rest[0]

		if c == 'n' {
			switch {
			case rest == "n" || rest == "nn":
				if !final {
					b.WriteString(rest)
					return b.String()
				}
				b.WriteString("ん")
				i += len(rest)
				continue
			case isVowel(rest[1]) || rest[1] == 'y':
				// na, nya...: handled by the table below.
			case rest[1] == 'n' && len(rest) > 2 && (isVowel(rest[2]) || rest[2] == 'y'):
				b.WriteString("ん") // "nna" is ん + な
				i++
				continue
			case rest[1] == 'n' || rest[1] == '\'':
				b.WriteString("ん")
				i += 2
				continue
			default:
				b.WriteString("ん")
				i++
				continue
			}
		}

		// Hepburn spells ん as m before b, p, and m: shimbun, sampo.
		if c == 'm' && len(rest) > 1 && strings.IndexByte("bpm", rest[1]) >= 0 {
			b.WriteString("ん")
			i++
			continue
		}

		// A doubled consonant (kk, ss, tt...) or "tch" becomes a small っ.
		if len(rest) > 1 && c >= 'a' && c <= 'z' && !isVowel(c) &&
			(rest[1] == c || strings.HasPrefix(rest, "tch")) {
			b.WriteString("っ")
			i++
			continue
		}

		matched := false
		for n := min(4, len(rest)); n >= 1; n-- {
			if kana, ok := romaji[rest[:n]]; ok {
				b.WriteString(kana)
				i += n
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// Not romaji: copy one whole UTF-8 character through unchanged.
		r, size := utf8.DecodeRuneInString(rest)
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

// KatakanaToHiragana shifts katakana (ァ..ヶ) down to the matching hiragana.
func KatakanaToHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ァ' && r <= 'ヶ' {
			return r - ('ァ' - 'ぁ')
		}
		return r
	}, s)
}

func containsKana(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r >= 'ぁ' && r <= 'ヿ' })
}

func containsLatin(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' })
}
