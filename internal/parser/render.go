package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/chromedp/chromedp"
)

type renderedPage struct {
	Title   string   `json:"title"`
	Height  int      `json:"height"`
	Text    string   `json:"text"`
	Buttons []string `json:"buttons"`
	Images  []string `json:"images"`
	Links   []string `json:"links"`
}

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

const (
	defaultRenderTimeout = 60 * time.Second
	defaultRenderWait    = 3 * time.Second
)

type RenderOptions struct {
	BrowserPath string
	Timeout     time.Duration
	Wait        time.Duration
	Click       []string
	ClickText   string
	ProfileDir  string
	Scroll      bool
}

const imageCollectorJS = `function gopickCollectImages() {
  var out = [];
  var push = function (v) { if (v) { out.push(String(v).trim()); } };
  var srcset = function (v) {
    if (!v) { return; }
    v.split(',').forEach(function (part) {
      var u = part.trim().split(/\s+/)[0];
      if (u) { push(u); }
    });
  };

  document.querySelectorAll('img').forEach(function (img) {
    push(img.currentSrc);
    push(img.getAttribute('src'));
    push(img.getAttribute('data-src'));
    push(img.getAttribute('data-lazy-src'));
    push(img.getAttribute('data-original'));
    srcset(img.getAttribute('srcset'));
    srcset(img.getAttribute('data-srcset'));
  });

  document.querySelectorAll('source').forEach(function (s) {
    push(s.getAttribute('src'));
    srcset(s.getAttribute('srcset'));
  });

  document.querySelectorAll('video[poster]').forEach(function (v) {
    push(v.getAttribute('poster'));
  });

  document.querySelectorAll('*').forEach(function (el) {
    var bg = getComputedStyle(el).backgroundImage;
    if (bg && bg !== 'none') {
      var re = /url\(["']?([^"')]+)["']?\)/g;
      var m;
      while ((m = re.exec(bg)) !== null) { push(m[1]); }
    }
  });

  var og = document.querySelector('meta[property="og:image"]');
  if (og) { push(og.getAttribute('content')); }

  return out;
}`

const buttonCollectorJS = `function gopickCollectButtons() {
  var out = [];
  var seen = {};
  var nodes = document.querySelectorAll('button, a, [role="button"], li');
  for (var i = 0; i < nodes.length; i++) {
    var el = nodes[i];
    var t = (el.innerText || '').replace(/\s+/g, ' ').trim();
    if (!t || t.length > 40 || seen[t]) { continue; }
    var r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) { continue; }
    seen[t] = true;
    out.push(t);
    if (out.length >= 20) { break; }
  }
  return out;
}`

func openPage(pageURL string, opts RenderOptions, fn func(ctx context.Context) error) error {
	execPath, err := findBrowser(opts.BrowserPath)
	if err != nil {
		return err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultRenderTimeout
	}
	wait := opts.Wait
	if wait <= 0 {
		wait = defaultRenderWait
	}

	allocOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts,
		chromedp.ExecPath(execPath),
		chromedp.UserAgent(browserUserAgent),
		chromedp.WindowSize(1920, 1080),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
	)
	if opts.ProfileDir != "" {
		allocOpts = append(allocOpts, chromedp.UserDataDir(opts.ProfileDir))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	if err := chromedp.Do(ctx,
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(wait),
	); err != nil {
		return fmt.Errorf("не удалось отрендерить %q: %w", pageURL, err)
	}

	return fn(ctx)
}

func FetchImagesRendered(pageURL string, opts RenderOptions) ([]string, error) {
	images, _, err := FetchImagesRenderedDetailed(pageURL, opts)
	return images, err
}

func FetchImagesRenderedDetailed(pageURL string, opts RenderOptions) ([]string, PageReport, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, PageReport{}, fmt.Errorf("некорректный URL страницы %q: %w", pageURL, err)
	}

	var page renderedPage
	err = openPage(pageURL, opts, func(ctx context.Context) error {

		for _, selector := range opts.Click {
			ok, err := clickSelector(ctx, selector)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("на странице %q не найден элемент %q для клика", pageURL, selector)
			}
		}
		if opts.ClickText != "" {
			ok, err := clickText(ctx, opts.ClickText)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("на странице %q не найдена кнопка с текстом %q", pageURL, opts.ClickText)
			}
		}
		if len(opts.Click) > 0 || opts.ClickText != "" {
			if err := chromedp.Do(ctx, chromedp.Sleep(2*time.Second)); err != nil {
				return err
			}
		}

		if opts.Scroll {
			if err := autoScroll(ctx); err != nil {
				return err
			}
		}

		raw, err := chromedp.Run(ctx, chromedp.Evaluate[string](inspectScript))
		if err != nil {
			return fmt.Errorf("не удалось собрать изображения со страницы %q: %w", pageURL, err)
		}
		return json.Unmarshal([]byte(raw), &page)
	})
	if err != nil {
		return nil, PageReport{}, err
	}

	images := collect(base, page.Images)
	return images, PageReport{
		URL:       pageURL,
		Mode:      "рендер (headless-браузер)",
		Title:     page.Title,
		Text:      page.Text,
		Height:    page.Height,
		Buttons:   page.Buttons,
		Links:     page.Links,
		Images:    images,
		Generated: time.Now(),
	}, nil
}

func ClickCandidates(pageURL string, opts RenderOptions) ([]string, error) {
	opts.Click = nil
	opts.ClickText = ""
	opts.Scroll = false

	var texts []string
	err := openPage(pageURL, opts, func(ctx context.Context) error {
		found, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](clickCandidatesScript))
		if err != nil {
			return fmt.Errorf("не удалось прочитать кнопки на странице %q: %w", pageURL, err)
		}
		texts = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return texts, nil
}

const clickCandidatesScript = `(function () {
  ` + buttonCollectorJS + `
  return gopickCollectButtons();
})()`

const inspectScript = `(function () {
  ` + imageCollectorJS + `
  ` + buttonCollectorJS + `
  var links = [];
  var seen = {};
  var anchors = document.querySelectorAll('a[href]');
  for (var i = 0; i < anchors.length && links.length < 40; i++) {
    var href = anchors[i].getAttribute('href');
    if (!href || seen[href]) { continue; }
    seen[href] = true;
    links.push(href);
  }
  return JSON.stringify({
    title: document.title || '',
    height: document.body ? document.body.scrollHeight : 0,
    text: (document.body ? document.body.innerText : '').replace(/\s+/g, ' ').trim().slice(0, 1200),
    buttons: gopickCollectButtons(),
    images: gopickCollectImages(),
    links: links
  });
})()`

func clickSelector(ctx context.Context, selector string) (bool, error) {
	return chromedp.Run(ctx, chromedp.Evaluate[bool](`(function (sel) {
  var el = document.querySelector(sel);
  if (!el) { return false; }
  el.scrollIntoView({ block: 'center' });
  el.click();
  return true;
})(`+strconv.Quote(selector)+`)`))
}

func clickText(ctx context.Context, text string) (bool, error) {
	return chromedp.Run(ctx, chromedp.Evaluate[bool](`(function (needle) {
  var want = needle.trim().toLowerCase();
  var nodes = document.querySelectorAll('button, a, [role="button"], li, span, div');
  var best = null;
  var bestDepth = -1;
  for (var i = 0; i < nodes.length; i++) {
    var el = nodes[i];
    var t = (el.innerText || '').trim().toLowerCase();
    if (t.indexOf(want) === -1) { continue; }
    var depth = 0;
    for (var p = el; p; p = p.parentElement) { depth++; }
    if (depth > bestDepth) { best = el; bestDepth = depth; }
  }
  if (!best) { return false; }
  best.scrollIntoView({ block: 'center' });
  best.click();
  return true;
})(`+strconv.Quote(text)+`)`))
}

func autoScroll(ctx context.Context) error {
	const steps = 30
	for i := 0; i < steps; i++ {
		more, err := chromedp.Run(ctx, chromedp.Evaluate[bool](
			`(function () {
				var before = window.scrollY;
				window.scrollBy(0, window.innerHeight);
				return window.scrollY > before && (window.innerHeight + window.scrollY) < document.body.scrollHeight;
			})()`))
		if err != nil {
			return fmt.Errorf("не удалось прокрутить страницу: %w", err)
		}
		if err := chromedp.Do(ctx, chromedp.Sleep(300*time.Millisecond)); err != nil {
			return fmt.Errorf("не удалось прокрутить страницу: %w", err)
		}
		if !more {
			break
		}
	}
	return nil
}
