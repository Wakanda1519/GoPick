package parser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// BrowserAvailable проверяет, что браузер для режима рендера доступен, не
// запуская его.
func BrowserAvailable(explicit string) error {
	_, err := findBrowser(explicit)
	return err
}

// findBrowser возвращает путь к Chrome/Chromium/Edge для headless-рендера.
// Явный путь имеет приоритет, затем переменные окружения, затем типовые места
// установки и, наконец, поиск в PATH.
func findBrowser(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("не найден браузер по пути %q: %w", explicit, err)
		}
		return explicit, nil
	}

	for _, env := range []string{"GOPICK_BROWSER", "CHROME_PATH", "CHROME_BIN", "EDGE_PATH"} {
		if v := os.Getenv(env); v != "" {
			if _, err := os.Stat(v); err == nil {
				return v, nil
			}
		}
	}

	for _, path := range browserCandidates() {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"microsoft-edge", "msedge", "chrome", "brave-browser",
	} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", errors.New("не найден Chrome/Chromium/Edge: установите браузер или укажите путь через -browser")
}

// browserCandidates перечисляет типовые пути установки браузеров для текущей ОС.
func browserCandidates() []string {
	rel := []string{
		filepath.Join("Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join("Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join("BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		filepath.Join("Chromium", "Application", "chrome.exe"),
	}

	switch runtime.GOOS {
	case "windows":
		roots := []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LOCALAPPDATA"),
		}
		var out []string
		for _, root := range roots {
			if root == "" {
				continue
			}
			for _, r := range rel {
				out = append(out, filepath.Join(root, r))
			}
		}
		return out

	case "darwin":
		var out []string
		for _, app := range []string{
			filepath.Join("Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
			filepath.Join("Microsoft Edge.app", "Contents", "MacOS", "Microsoft Edge"),
			filepath.Join("Chromium.app", "Contents", "MacOS", "Chromium"),
			filepath.Join("Brave Browser.app", "Contents", "MacOS", "Brave Browser"),
		} {
			out = append(out, filepath.Join("/Applications", app))
			out = append(out, filepath.Join(os.Getenv("HOME"), "Applications", app))
		}
		return out

	default:
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/microsoft-edge",
			"/snap/bin/chromium",
		}
	}
}
