// Package parser извлекает ссылки на изображения со страницы: сначала из
// статического HTML, а при необходимости — после рендера в headless-браузере.
package parser

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

// userAgent identifies GoPick to the remote server.
const userAgent = "GoPick/1.0"

// FetchImages делает HTTP GET страницы и возвращает дедуплицированный список
// абсолютных URL изображений из статического HTML.
func FetchImages(pageURL string) ([]string, error) {
	images, _, err := FetchImagesDetailed(pageURL)
	return images, err
}

// FetchImagesWithClient — FetchImages с подменяемым HTTP-клиентом, что делает
// парсер тестируемым без выхода в сеть.
func FetchImagesWithClient(pageURL string, client *http.Client) ([]string, error) {
	images, _, err := fetchImagesDetailedWithClient(pageURL, client)
	return images, err
}

// FetchImagesDetailed читает HTML и за один запрос возвращает и изображения, и
// отчёт о странице для лога.
func FetchImagesDetailed(pageURL string) ([]string, PageReport, error) {
	return fetchImagesDetailedWithClient(pageURL, defaultClient())
}

func fetchImagesDetailedWithClient(pageURL string, client *http.Client) ([]string, PageReport, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, PageReport{}, fmt.Errorf("некорректный URL страницы %q: %w", pageURL, err)
	}

	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, PageReport{}, fmt.Errorf("не удалось создать запрос: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "ru,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, PageReport{}, fmt.Errorf("не удалось загрузить %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, PageReport{}, fmt.Errorf("не удалось загрузить %q: неожиданный статус %s", pageURL, resp.Status)
	}

	// Страницы в windows-1251 и других кодировках приводим к UTF-8, иначе
	// не-ASCII символы в ссылках разбираются некорректно.
	body, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		body = resp.Body
	}

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, PageReport{}, fmt.Errorf("не удалось разобрать HTML: %w", err)
	}

	var raw []string
	attr := func(s *goquery.Selection, name string) {
		if v, ok := s.Attr(name); ok {
			raw = append(raw, v)
		}
	}
	srcset := func(s *goquery.Selection, name string) {
		v, ok := s.Attr(name)
		if !ok {
			return
		}
		for _, part := range strings.Split(v, ",") {
			if fields := strings.Fields(part); len(fields) > 0 {
				raw = append(raw, fields[0])
			}
		}
	}

	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		attr(s, "src")
		attr(s, "data-src")
		attr(s, "data-lazy-src")
		attr(s, "data-original")
		srcset(s, "srcset")
		srcset(s, "data-srcset")
	})

	doc.Find("source").Each(func(_ int, s *goquery.Selection) {
		attr(s, "src")
		srcset(s, "srcset")
	})

	doc.Find("video[poster]").Each(func(_ int, s *goquery.Selection) {
		attr(s, "poster")
	})

	doc.Find(`meta[property="og:image"], meta[name="og:image"], meta[property="twitter:image"], meta[name="twitter:image"]`).
		Each(func(_ int, s *goquery.Selection) {
			attr(s, "content")
		})

	var buttons, links []string
	seenLink := make(map[string]struct{})
	doc.Find("button, a, [role=button]").Each(func(_ int, s *goquery.Selection) {
		if text := strings.Join(strings.Fields(s.Text()), " "); text != "" && len(buttons) < 20 {
			buttons = append(buttons, text)
		}
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		if _, dup := seenLink[href]; dup || len(links) >= 40 {
			return
		}
		seenLink[href] = struct{}{}
		links = append(links, href)
	})

	// Витрины GameStores (React/Vite) помечают себя ссылками на свой CDN: по
	// этому признаку страницу можно разобрать через JSON-API, не запуская
	// браузер.
	gameStores := doc.Find(`script[src*="gamestores.app"], link[href*="gamestores.app"]`).Length() > 0

	// Текст собираем без содержимого <script>/<style>/<noscript>: goquery
	// включает их текст в body, из-за чего в отчёт попадал код счётчиков и
	// бутстрапа вместо текста страницы.
	doc.Find("script, style, noscript, template").Remove()
	text := strings.Join(strings.Fields(doc.Find("body").Text()), " ")

	// Нет текста, но есть контейнер SPA — контент дорисован JavaScript, и
	// статический HTML заведомо неполон.
	spa := text == "" && doc.Find("#root, #app, #__next, [data-reactroot]").Length() > 0

	if runes := []rune(text); len(runes) > 1200 {
		text = string(runes[:1200])
	}

	images := collect(base, raw)
	report := PageReport{
		URL:        pageURL,
		Mode:       "HTML",
		Title:      strings.TrimSpace(doc.Find("title").Text()),
		Text:       text,
		Buttons:    buttons,
		Links:      links,
		Images:     images,
		SPA:        spa,
		GameStores: gameStores,
		Generated:  time.Now(),
	}
	return images, report, nil
}

// collect превращает список найденных ссылок в дедуплицированный набор
// абсолютных http(s) URL.
func collect(base *url.URL, candidates []string) []string {
	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		abs, ok := resolve(base, candidate)
		if !ok {
			continue
		}
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

// resolve turns a possibly relative reference into an absolute http(s) URL.
func resolve(base *url.URL, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "javascript:") {
		return "", false
	}

	ref, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	abs := base.ResolveReference(ref)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return "", false
	}
	if abs.Host == "" {
		return "", false
	}
	return abs.String(), true
}

func defaultClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
