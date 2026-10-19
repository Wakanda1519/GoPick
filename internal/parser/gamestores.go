package parser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const gameStoresAPIPath = "/api/v1"

type gsInfoResponse struct {
	Result string `json:"result"`
	Data   struct {
		Store struct {
			StoreName  string `json:"storeName"`
			StoreTitle string `json:"storeTitle"`
		} `json:"data"`
		Design struct {
			SiteLogo       string `json:"siteLogo"`
			SiteBackground string `json:"siteBackground"`
		} `json:"design"`
		Servers []struct {
			Name string `json:"name"`
		} `json:"servers"`
	} `json:"data"`
}

type gsProductsResponse struct {
	Result string `json:"result"`
	Data   struct {
		Categories []struct {
			Name string `json:"name"`
		} `json:"categories"`
		Products []struct {
			Name  string `json:"name"`
			Image string `json:"img"`
			About string `json:"about"`
		} `json:"products"`
	} `json:"data"`
}

func FetchGameStoresImages(pageURL string) ([]string, PageReport, bool, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, PageReport{}, false, fmt.Errorf("некорректный URL страницы %q: %w", pageURL, err)
	}

	client := defaultClient()

	var info gsInfoResponse
	if err := getJSON(client, gameStoresURL(base, "/info"), &info); err != nil {
		return nil, PageReport{}, false, nil
	}
	name := strings.TrimSpace(info.Data.Store.StoreName)
	if info.Result != "success" || name == "" {
		return nil, PageReport{}, false, nil
	}

	raw := []string{
		info.Data.Design.SiteLogo,
		info.Data.Design.SiteBackground,
	}

	var categories []string
	var productCount int
	var products gsProductsResponse
	if err := getJSON(client, gameStoresURL(base, "/products.list"), &products); err == nil && products.Result == "success" {
		productCount = len(products.Data.Products)
		for _, category := range products.Data.Categories {
			if category.Name != "" {
				categories = append(categories, category.Name)
			}
		}
		for _, product := range products.Data.Products {
			raw = append(raw, product.Image)
			raw = append(raw, imagesInHTML(product.About)...)
		}
	}

	images := collect(base, raw)
	text := fmt.Sprintf("Витрина GameStores «%s»: товаров %d, категорий %d, серверов %d.",
		name, productCount, len(categories), len(info.Data.Servers))

	return images, PageReport{
		URL:        pageURL,
		Mode:       "API (GameStores)",
		Title:      name,
		Text:       text,
		Buttons:    categories,
		Images:     images,
		GameStores: true,
		Generated:  time.Now(),
	}, true, nil
}

func gameStoresURL(base *url.URL, path string) string {
	u := *base
	u.Path = gameStoresAPIPath + path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func getJSON(client *http.Client, pageURL string, dst any) error {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "ru,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("неожиданный статус %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func imagesInHTML(fragment string) []string {
	if !strings.Contains(fragment, "<img") {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fragment))
	if err != nil {
		return nil
	}
	var out []string
	doc.Find("img[src]").Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("src"); ok && v != "" {
			out = append(out, v)
		}
	})
	return out
}
