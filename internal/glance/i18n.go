package glance

import (
	"embed"
	"fmt"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localesFS embed.FS

var supportedLanguages = []string{"en", "zh-CN"}

var currentLanguage = "en"
var currentTranslations = map[string]string{}

func setLocale(languageTag string) error {
	translations := map[string]string{}
	lang := language.English

	if languageTag != "" && languageTag != "en" {
		exists := false
		for _, l := range supportedLanguages {
			if l == languageTag {
				exists = true
				break
			}
		}

		if !exists {
			return fmt.Errorf("language %q is not supported, supported languages: %v", languageTag, supportedLanguages)
		}

		contents, err := localesFS.ReadFile("locales/" + languageTag + ".yaml")
		if err != nil {
			return fmt.Errorf("reading locale file: %v", err)
		}

		if err = yaml.Unmarshal(contents, &translations); err != nil {
			return fmt.Errorf("parsing locale file: %v", err)
		}

		lang = language.Make(languageTag)
	}

	currentLanguage = languageTag
	currentTranslations = translations
	intl = message.NewPrinter(lang)

	return nil
}

// T returns the translation for the given string in the currently set language.
// When the string is not translated, it is returned as is. If any additional
// arguments are provided, the result is formatted with them.
func T(s string, args ...any) string {
	if translated, ok := currentTranslations[s]; ok {
		s = translated
	}

	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}

	return s
}

func isZhLanguage() bool {
	return strings.HasPrefix(currentLanguage, "zh")
}
