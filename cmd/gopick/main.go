package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wakanda1519/GoPick/internal/downloader"
	"github.com/Wakanda1519/GoPick/internal/parser"
	"github.com/Wakanda1519/GoPick/internal/storage"
	"github.com/Wakanda1519/GoPick/internal/ui"
)

const version = "1.0"

const banner = `
	 ██████╗   ██████╗  ██████╗ ██╗  ██████╗ ██╗  ██╗
	██╔════╝  ██╔═══██╗ ██╔══██╗ ██║ ██╔════╝  ██║ ██╔╝
	██║  ███╗ ██║   ██║ ██████╔╝ ██║ ██║       █████╔╝
	██║   ██║ ██║   ██║ ██╔═══╝  ██║ ██║       ██╔═██╗
	╚██████╔╝ ╚██████╔╝ ██║      ██║ ╚██████╗  ██║  ██╗
	 ╚═════╝  ╚═════╝  ╚═╝      ╚═╝  ╚═════╝  ╚═╝  ╚═╝
	        G o P i c k   ·   загрузчик изображений   ·   v` + version + `
	     сортировка: png | svg | jpg | webp | gif | unknown
	     режимы: авто (HTML + JS) | -render | -no-render
	           github.com/Wakanda1519
`

func main() {
	enableUTF8Console()
	enableANSIConsole()
	ui.Init(isTerminal(os.Stdout))

	args := os.Args[1:]
	interactive := len(args) == 0

	if interactive || isTerminal(os.Stdout) {
		fmt.Print(banner)
	}

	if interactive {
		runInteractive()
		return
	}

	if err := run(args); err != nil {
		fmt.Fprintf(os.Stderr, "\nGoPick: %v\n", err)
		os.Exit(1)
	}
}

type menuItem struct {
	flag  string
	title string
	hint  string
	value string
}

var menu = []menuItem{
	{"-click-text", "Кликнуть по кнопке с текстом", "например, выбрать сервер или открыть каталог", "текст кнопки"},
	{"-click", "Кликнуть по элементу (CSS)", "точное попадание в нужный элемент", "CSS-селектор"},
	{"-list", "Только показать ссылки", "ничего не скачивать", ""},
}

func runInteractive() {
	flags := chooseFlags(nil)

	fmt.Println()
	fmt.Println("  Введите адрес сайта — начнётся скачивание.")
	fmt.Println("  " + ui.Dim("Enter без адреса — выход, слово «настройки» — сменить флаги."))

	for {
		fmt.Println()
		fmt.Println("  " + ui.Bold("Сейчас:") + " " + describeFlags(flags))
		line := readLine("  Адрес: ")
		if line == "" {
			fmt.Println("Выход.")
			return
		}
		if line == "настройки" || line == "флаги" {
			flags = chooseFlags(flags)
			continue
		}

		var args []string
		if strings.HasPrefix(line, "-") || len(splitArgs(line)) > 1 {
			args = splitArgs(line)
		} else {
			args = append(append([]string{}, flags...), line)
		}

		if err := run(args); err != nil {
			fmt.Fprintf(os.Stderr, "GoPick: %v\n", err)
		}
	}
}

func chooseFlags(current []string) []string {
	enabled := splitFlags(current)

	fmt.Println()
	fmt.Println("  " + ui.Bold("НАСТРОЙКА"))
	fmt.Println("  Обычно ничего выбирать не нужно: нажмите Enter и введите адрес.")
	fmt.Println("  Утилита сама читает HTML, а если картинок нет — открывает страницу в браузере.")
	fmt.Println()

	for i, item := range menu {
		mark := " "
		hint := item.hint
		if value, ok := enabled[item.flag]; ok {
			mark = ui.Green("✓")
			if value != "" {
				hint += ", сейчас: «" + value + "»"
			}
		}
		fmt.Printf("   %s %s  %-30s %s\n", mark, ui.Green(fmt.Sprintf("%d)", i+1)), item.title, ui.Dim("— "+hint))
	}

	fmt.Println()
	fmt.Printf("     %s  %-30s %s\n", ui.Green("0)"), "Сбросить настройки", ui.Dim("— как при первом запуске"))
	fmt.Println()
	fmt.Println("  " + ui.Bold("Сейчас:") + " " + describeFlags(current))

	line := readLine("\n  Номера (Enter — оставить как есть): ")
	if line == "" {
		return current
	}
	if line == "0" {
		fmt.Println("   " + ui.Green("✓") + " настройки сброшены")
		return nil
	}

	var chosen []string
	for _, token := range strings.Split(line, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		n, err := strconv.Atoi(token)
		if err != nil || n < 1 || n > len(menu) {
			fmt.Printf("   %s «%s» пропущено: нужно число от 1 до %d\n", ui.Red("!"), token, len(menu))
			continue
		}
		item := menu[n-1]
		if item.value == "" {
			chosen = append(chosen, item.flag)
			continue
		}
		value := readLine("    " + item.value + ": ")
		if value == "" {
			fmt.Println("    значение не задано — пункт пропущен")
			continue
		}
		chosen = append(chosen, item.flag, value)
	}

	fmt.Println()
	fmt.Println("  " + ui.Bold("Выбрано:") + " " + describeFlags(chosen))
	if len(chosen) > 0 {
		fmt.Println("  " + ui.Dim("команда: GoPick "+formatArgs(chosen)+" <адрес>"))
	}
	return chosen
}

func splitFlags(args []string) map[string]string {
	out := make(map[string]string, len(args))
	for i := 0; i < len(args); i++ {
		flag := args[i]
		value := ""
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		}
		out[flag] = value
	}
	return out
}

func describeFlags(args []string) string {
	if len(args) == 0 {
		return "настройки по умолчанию (HTML → авто-рендер, прокрутка, 8 потоков)"
	}

	parts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		flag := args[i]
		value := ""
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		}

		title := flag
		for _, item := range menu {
			if item.flag == flag {
				title = item.title
				break
			}
		}
		if value != "" {
			title += " «" + value + "»"
		}
		parts = append(parts, title)
	}
	return strings.Join(parts, "; ")
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return "(нет)"
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			parts = append(parts, strconv.Quote(a))
			continue
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

var stdin = bufio.NewReader(os.Stdin)

func readLine(prompt string) string {
	fmt.Print(prompt)
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

func splitArgs(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false

	flush := func() {
		if current.Len() > 0 {
			args = append(args, current.String())
			current.Reset()
		}
	}

	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return args
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func run(args []string) error {
	fs := flag.NewFlagSet("GoPick", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Использование: GoPick [флаги] <url>")
		fmt.Fprintln(os.Stderr, "\nСкачивает все изображения страницы, раскладывая их по папкам форматов.")
		fmt.Fprintln(os.Stderr, "Сначала читается HTML, а если картинок в нём нет — страница рендерится")
		fmt.Fprintln(os.Stderr, "в headless-браузере, поэтому работает и на сайтах на JavaScript.")
		fmt.Fprintln(os.Stderr, "\nФлаги:")
		fs.PrintDefaults()
	}
	workers := fs.Int("w", downloader.DefaultWorkers, "максимальное число одновременных загрузок")
	render := fs.Bool("render", false, "сразу рендерить страницу в headless-браузере, минуя разбор HTML")
	noRender := fs.Bool("no-render", false, "не переходить в режим рендера, если в HTML картинок нет")
	wait := fs.Duration("wait", 3*time.Second, "пауза после загрузки страницы в режиме рендера, чтобы SPA успел запросить данные")
	browserPath := fs.String("browser", "", "путь к Chrome/Chromium/Edge для режима рендера (по умолчанию ищется автоматически)")
	click := fs.String("click", "", "CSS-селектор элемента, по которому кликнуть перед сбором (несколько — через запятую)")
	clickText := fs.String("click-text", "", "текст кнопки для клика перед сбором, например выбор сервера или открытие каталога")
	profile := fs.String("profile", "", "каталог профиля браузера: сохраняет cookies и авторизацию между запусками")
	scroll := fs.Bool("scroll", true, "прокручивать страницу в режиме рендера, чтобы догрузились ленивые картинки")
	noScroll := fs.Bool("no-scroll", false, "не прокручивать страницу в режиме рендера")
	list := fs.Bool("list", false, "только показать найденные ссылки, ничего не скачивая")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("ожидается ровно один URL, получено: %d", fs.NArg())
	}

	pageURL, err := normalizeURL(fs.Arg(0))
	if err != nil {
		return err
	}

	renderOpts := parser.RenderOptions{
		BrowserPath: *browserPath,
		Wait:        *wait,
		Click:       splitList(*click),
		ClickText:   *clickText,
		ProfileDir:  *profile,
		Scroll:      *scroll && !*noScroll,
	}

	totalSteps := 3
	if *list {
		totalSteps = 1
	}

	ui.Step(1, totalSteps, "Сканирование страницы")
	ui.OK("адрес: %s", pageURL)

	images, report, mode, err := scan(pageURL, *render, *noRender, renderOpts)
	if err != nil {
		return err
	}
	ui.OK("режим: %s", mode)

	report.Mode = mode
	writeLog(report)

	if len(images) == 0 {
		ui.Warn("изображения не найдены")
		hintClickCandidates(pageURL, renderOpts)
		return nil
	}
	ui.OK("изображений: %d", len(images))

	if *list {
		for i, u := range images {
			ui.Info("%d. %s", i+1, u)
		}
		return nil
	}

	ui.Step(2, totalSteps, "Подготовка каталога")

	root, err := storage.New(rootDir(pageURL))
	if err != nil {
		return err
	}
	ui.OK("каталог: %s", root.Root())

	ui.Step(3, totalSteps, "Скачивание изображений")
	ui.OK("к загрузке: %d, потоков: %d", len(images), *workers)

	summary := downloader.DownloadWithProgress(images, root, *workers, func(p downloader.Progress) {
		ui.Progress(p.Done, p.Total, p.Succeeded, p.Failed)
	})
	ui.ProgressDone()

	for _, res := range summary.Results {
		if res.Err != nil {
			ui.Warn("%s: %v", res.URL, res.Err)
		}
	}
	ui.OK("сохранено: %d из %d, ошибок %d", summary.Succeeded, summary.Total, summary.Failed)
	ui.Info("папка: %s", root.Root())

	if summary.Succeeded == 0 {
		return errors.New("не удалось скачать ни одного изображения")
	}
	return nil
}

func writeLog(report parser.PageReport) {
	entry := fmt.Sprintf("\n===== %s =====\n%s\n",
		report.Generated.Format("2006-01-02 15:04:05"), report.String())

	path, err := appendLog(logPath(), entry)
	if err != nil {

		path, err = appendLog("log.txt", entry)
		if err != nil {
			ui.Warn("не удалось записать лог: %v", err)
			return
		}
	}
	ui.OK("лог: %s", path)
}

func appendLog(path, text string) (string, error) {
	needBOM := true
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		needBOM = false
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if needBOM {
		text = "\ufeff" + text
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs, nil
	}
	return path, nil
}

func logPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "log.txt"
	}
	return filepath.Join(filepath.Dir(exe), "log.txt")
}

func scan(pageURL string, forceRender, noRender bool, opts parser.RenderOptions) ([]string, parser.PageReport, string, error) {
	if forceRender {
		images, report, err := parser.FetchImagesRenderedDetailed(pageURL, opts)
		return images, report, "рендер (headless-браузер)", err
	}

	images, report, err := parser.FetchImagesDetailed(pageURL)
	if err != nil {
		return nil, parser.PageReport{}, "HTML", err
	}

	if len(images) > 0 && !report.SPA {
		return images, report, "HTML", nil
	}
	if noRender {
		return images, report, "HTML", nil
	}

	if report.GameStores || len(images) == 0 {
		if apiImages, apiReport, ok, apiErr := parser.FetchGameStoresImages(pageURL); ok && apiErr == nil && len(apiImages) > 0 {
			return apiImages, apiReport, "API (GameStores)", nil
		}
	}

	if err := parser.BrowserAvailable(opts.BrowserPath); err != nil {
		ui.Info("готовых изображений нет, а браузер для рендера не найден")
		ui.Info("%s", "установите Chrome/Chromium/Edge или укажите путь: GoPick -browser <путь> "+pageURL)
		return images, report, "HTML", nil
	}

	ui.Info("готовых изображений нет — включаю рендер (JS/SPA)")
	images, report, err = parser.FetchImagesRenderedDetailed(pageURL, opts)
	return images, report, "HTML + рендер (headless-браузер)", err
}

func hintClickCandidates(pageURL string, opts parser.RenderOptions) {
	if err := parser.BrowserAvailable(opts.BrowserPath); err != nil {
		return
	}

	candidates, err := parser.ClickCandidates(pageURL, opts)
	if err != nil || len(candidates) == 0 {
		return
	}

	ui.Info("похоже, картинки появляются только после действия на странице")
	ui.Info("доступные кнопки: %s", strings.Join(candidates, " | "))
	ui.Info("%s", "попробуйте: GoPick -render -click-text \"<текст кнопки>\" "+pageURL)
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("пустой URL")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("некорректный URL %q: %w", raw, err)
	}
	if u.Scheme == "" {
		u, err = url.Parse("https://" + raw)
		if err != nil {
			return "", fmt.Errorf("некорректный URL %q: %w", raw, err)
		}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("неподдерживаемая схема %q: допускаются только http и https", u.Scheme)
	}
	if u.Host == "" || !strings.Contains(u.Host, ".") && u.Host != "localhost" {
		return "", fmt.Errorf("некорректный хост в URL %q", raw)
	}
	return u.String(), nil
}

func rootDir(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil || u.Host == "" {
		return "site"
	}

	host := strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_").Replace(u.Host)
	host = strings.Trim(host, ".")
	if host == "" {
		return "site"
	}
	return host
}
