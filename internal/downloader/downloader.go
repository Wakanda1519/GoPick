package downloader

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Wakanda1519/GoPick/internal/storage"
)

const DefaultWorkers = 8

const userAgent = "GoPick/1.0"

type Result struct {
	URL  string
	Path string
	Err  error
}

type Summary struct {
	Total     int
	Succeeded int
	Failed    int
	Results   []Result
}

type Progress struct {
	Done      int
	Total     int
	Succeeded int
	Failed    int
	Last      Result
}

func Download(urls []string, store *storage.Storage, workers int) Summary {
	return DownloadContext(context.Background(), urls, store, workers)
}

func DownloadWithProgress(urls []string, store *storage.Storage, workers int, onProgress func(Progress)) Summary {
	return download(context.Background(), urls, store, workers, onProgress)
}

func DownloadContext(ctx context.Context, urls []string, store *storage.Storage, workers int) Summary {
	return download(ctx, urls, store, workers, nil)
}

func download(ctx context.Context, urls []string, store *storage.Storage, workers int, onProgress func(Progress)) Summary {
	if workers < 1 {
		workers = DefaultWorkers
	}
	if workers > len(urls) && len(urls) > 0 {
		workers = len(urls)
	}

	client := &http.Client{Timeout: 60 * time.Second}

	jobs := make(chan string)
	results := make(chan Result)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				results <- downloadOne(ctx, client, store, u)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, u := range urls {
			select {
			case jobs <- u:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	summary := Summary{Total: len(urls)}
	for res := range results {
		if res.Err != nil {
			summary.Failed++
		} else {
			summary.Succeeded++
		}
		summary.Results = append(summary.Results, res)
		if onProgress != nil {
			onProgress(Progress{
				Done:      summary.Succeeded + summary.Failed,
				Total:     summary.Total,
				Succeeded: summary.Succeeded,
				Failed:    summary.Failed,
				Last:      res,
			})
		}
	}
	return summary
}

func downloadOne(ctx context.Context, client *http.Client, store *storage.Storage, rawURL string) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{URL: rawURL, Err: fmt.Errorf("не удалось создать запрос: %w", err)}
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return Result{URL: rawURL, Err: fmt.Errorf("ошибка запроса: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{URL: rawURL, Err: fmt.Errorf("неожиданный статус %s", resp.Status)}
	}

	path, err := store.Save(rawURL, resp.Body)
	if err != nil {
		return Result{URL: rawURL, Err: err}
	}
	return Result{URL: rawURL, Path: path}
}
