package parser

import (
	"fmt"
	"strings"
	"time"
)

// PageReport — то, что утилита увидела на странице. Отчёт помогает понять,
// почему нужных картинок нет и по какой кнопке имеет смысл кликнуть.
type PageReport struct {
	URL       string
	Mode      string
	Title     string
	Text      string
	Height    int
	Buttons   []string
	Links     []string
	Images    []string
	Generated time.Time
}

// InspectPage собирает отчёт: сначала пробует отрисовать страницу в браузере,
// а если браузера нет — читает обычный HTML.
func InspectPage(pageURL string, opts RenderOptions) (PageReport, error) {
	_, report, err := FetchImagesRenderedDetailed(pageURL, opts)
	if err == nil {
		return report, nil
	}

	static, staticErr := inspectStatic(pageURL)
	if staticErr != nil {
		return PageReport{}, fmt.Errorf("%v; HTML тоже не прочитан: %w", err, staticErr)
	}
	return static, nil
}

// inspectStatic собирает отчёт из обычного HTML, когда браузер недоступен.
func inspectStatic(pageURL string) (PageReport, error) {
	_, report, err := fetchImagesDetailedWithClient(pageURL, defaultClient())
	return report, err
}

// String формирует текст отчёта для сохранения в лог.
func (r PageReport) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Адрес:     %s\n", r.URL)
	fmt.Fprintf(&b, "Режим:     %s\n", r.Mode)
	if r.Title != "" {
		fmt.Fprintf(&b, "Заголовок: %s\n", r.Title)
	}
	if r.Height > 0 {
		fmt.Fprintf(&b, "Высота:    %d px\n", r.Height)
	}
	fmt.Fprintf(&b, "Найдено изображений: %d\n", len(r.Images))
	for _, img := range r.Images {
		fmt.Fprintf(&b, "  %s\n", img)
	}

	b.WriteString("\nКнопки и ссылки, по которым можно кликнуть:\n")
	if len(r.Buttons) == 0 {
		b.WriteString("  (не найдено)\n")
	}
	for _, button := range r.Buttons {
		fmt.Fprintf(&b, "  - %s\n", button)
	}

	b.WriteString("\nСсылки на странице:\n")
	if len(r.Links) == 0 {
		b.WriteString("  (не найдено)\n")
	}
	for _, link := range r.Links {
		fmt.Fprintf(&b, "  - %s\n", link)
	}

	b.WriteString("\nТекст страницы (начало):\n")
	if r.Text == "" {
		b.WriteString("  (пусто — возможно, всё рисует JavaScript)\n")
	} else {
		fmt.Fprintf(&b, "  %s\n", r.Text)
	}

	b.WriteString("\nЕсли нужных картинок нет среди найденных, значит они появляются\n")
	b.WriteString("только после действия на странице. Запустите с кликом:\n")
	fmt.Fprintf(&b, "  GoPick -click-text \"<текст кнопки>\" %s\n", r.URL)
	fmt.Fprintf(&b, "  GoPick -click \"<CSS-селектор>\" %s\n", r.URL)

	return b.String()
}
