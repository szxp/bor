package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"
	//"log"
	"context"
	"encoding/csv"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
)

func main() {
	env := &env{}

	fs, err := parseCmd(env)
	if err != nil {
		fmt.Println(err)
		fmt.Println()
		printUsage(fs)
		os.Exit(1)
	}

	err = env.execCmd()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type env struct {
	cmd      cmdName
	args     []string
	out      string
	force    bool
	format   format
	database string
}

type cmdName string

const (
	cnUrls   cmdName = "urls"
	cnExport cmdName = "export"
)

func parseCmdName(name string) (cmdName, error) {
	switch name {
	case string(cnUrls):
		return cnUrls, nil
	case string(cnExport):
		return cnExport, nil
	default:
		return "", fmt.Errorf("unknown command: " + name)
	}
}

type format string

const (
	fCsv format = "csv"
	//fXlsx format = "xlsx"
)

func parseFormat(val string) (format, error) {
	switch val {
	case string(fCsv):
		return fCsv, nil
	//case string(fXlsx):
	//	return fXlsx, nil
	default:
		return "", fmt.Errorf("unknown format: " + val)
	}
}

func parseForce(val string) (bool, error) {
	f, err := parseBool(val)
	if err != nil {
		return false, fmt.Errorf("unknown force option: %v", err)
	}
	return f, nil
}

func parseBool(val string) (bool, error) {
	switch val {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("unknown value: " + val)
	}
}

func parseCmd(env *env) (*flag.FlagSet, error) {
	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.Usage = func() {
		printUsage(fs)
	}
	addOutFlag(fs)
	addForceFlag(fs)
	addFormatFlag(fs)
	addDatabaseFlag(fs)

	if len(os.Args) < 2 {
		return fs, fmt.Errorf("no command")
	}

	cmdName, err := parseCmdName(os.Args[1])
	if err != nil {
		return fs, err
	}
	env.cmd = cmdName

	err = fs.Parse(os.Args[2:])
	if err != nil {
		return fs, err
	}

	err = parseCmdOptions(env, fs)
	if err != nil {
		return fs, err
	}
	return fs, nil
}

func parseCmdOptions(env *env, fs *flag.FlagSet) error {
	switch env.cmd {
	case cnUrls:
		return parseCmdUrls(env, fs)
	case cnExport:
		return parseCmdExport(env, fs)
	default:
		return fmt.Errorf("unknown command: " + string(env.cmd))
	}
}

func parseCmdUrls(env *env, fs *flag.FlagSet) error {
	err := parseCmdText(env, fs)
	if err != nil {
		return err
	}

	if len(env.args) == 0 {
		return fmt.Errorf("no search urls as arguments")
	}

	return nil
}

func parseCmdExport(env *env, fs *flag.FlagSet) error {
	err := parseCmdText(env, fs)
	if err != nil {
		return err
	}

	if len(env.args) == 0 {
		return fmt.Errorf("no link files as arguemnts")
	}

	err = assertExistFiles(env.args...)
	if err != nil {
		return err
	}

	err = assertRegularFiles(env.args...)
	if err != nil {
		return err
	}

	return nil
}

func parseCmdText(env *env, fs *flag.FlagSet) error {
	var err error
	env.args = fs.Args()

	fs.VisitAll(func(f *flag.Flag) {
		if err != nil {
			return
		}

		switch f.Name {
		case flagOut:
			env.out = strings.TrimSpace(f.Value.String())
		case flagForce:
			frc, e := parseForce(strings.TrimSpace(f.Value.String()))
			if e != nil {
				err = e
				return
			}
			env.force = frc
		case flagFormat:
			frm, e := parseFormat(strings.TrimSpace(f.Value.String()))
			if e != nil {
				err = e
				return
			}
			env.format = frm
		case flagDatabase:
			env.database = strings.TrimSpace(f.Value.String())
		default:
			err = fmt.Errorf("unknown option: " + f.Name)
		}
	})

	if env.out != "" {
		exist, err := isExistFile(env.out)
		if err != nil {
			return err
		}

		if exist {
			b, err := isRegularFile(env.out)
			if err != nil {
				return err
			}
			if !b {
				return fmt.Errorf("not a regular file: %v", env.out)
			}

			if !env.force {
				return fmt.Errorf(
					"file already exists: %v, use the -force option to overwrite it",
					env.out)
			}
		}

	}

	return err
}

const (
	flagOut      = "out"
	flagForce    = "force"
	flagFormat   = "format"
	flagDatabase = "database"
)

func addOutFlag(fs *flag.FlagSet) *string {
	return fs.String(
		flagOut,
		"",
		"Output file path. If not specified or an empty string is specified the output will be written to the standard output. Use the -format option to specify the format of the file.")
}

func addForceFlag(fs *flag.FlagSet) *bool {
	return fs.Bool(
		flagForce,
		false,
		"Overwrite output file if it already exists.")
}

func addFormatFlag(fs *flag.FlagSet) *string {
	return fs.String(
		flagFormat,
		"csv",
		"Format of the output file. Supported values are: csv.")
}

func addDatabaseFlag(fs *flag.FlagSet) *string {
	return fs.String(
		flagDatabase,
		"frandb",
		"Database directory where the downloaded data will be saved and cached.")
}
func (env *env) execCmd() error {
	switch env.cmd {
	case cnUrls:
		return env.execCmdUrls()
	case cnExport:
		return env.execCmdExport()
	default:
		return fmt.Errorf("unexpected command: " + string(env.cmd))
	}
}

func (env *env) execCmdUrls() error {
	w, closeFn, err := openOut(env.out, env.force)
	if err != nil {
		return err
	}
	defer closeFn()
	return env.getUrls(w)
}

func openOut(out string, force bool) (io.Writer, func(), error) {
	if out != "" {
		flags := os.O_WRONLY | os.O_CREATE
		if force {
			flags |= os.O_TRUNC
		} else {
			flags |= os.O_EXCL
		}

		f, err := os.OpenFile(out, flags, 0755)
		if err != nil {
			return nil, nil, err
		}
		closeFn := func() {
			f.Close()
		}
		return f, closeFn, nil
	}

	b := bufio.NewWriter(os.Stdout)
	closeFn := func() {
		b.Flush()
	}
	return b, closeFn, nil
}

func newChromeContext(ctx0 context.Context) (context.Context, func()) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-extensions", false),
	)

	actx, acancel := chromedp.NewExecAllocator(ctx0, opts...)
	ctx, cancel := chromedp.NewContext(
		actx,
		//chromedp.WithDebugf(log.Printf),
	)

	return ctx, func() {
		cancel()
		acancel()
	}
}

func (env *env) getUrls(w io.Writer) error {
	ctx, cancel := newChromeContext(context.Background())
	defer cancel()

	for _, u := range env.args {
		err := chromedp.Run(ctx, chromedp.Tasks{
			chromedp.Navigate(u),
			chromedp.WaitVisible(
				`//app-equity-search-result-table//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//a`,
				chromedp.BySearch),
			chromedp.WaitNotPresent(
				`//app-loading-spinner`,
				chromedp.BySearch),
			chromedp.Click(
				`//app-page-bar//button[contains(text(), '100')]`,
				chromedp.BySearch),
			chromedp.WaitVisible(
				`//app-loading-spinner`,
				chromedp.BySearch),
			chromedp.WaitNotPresent(
				`//app-loading-spinner`,
				chromedp.BySearch),
		})
		if err != nil {
			return err
		}

		rels, err := env.getUrlsRel(ctx)
		if err != nil {
			return err
		}

		uu, err := url.Parse(u)
		if err != nil {
			return err
		}

		host := uu.Scheme + "://" + uu.Host
		for _, rel := range rels {
			_, err = fmt.Fprintln(w, host+rel)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (env *env) getUrlsRel(ctx context.Context) ([]string, error) {
	urls := make([]string, 0, 100)
	for {
		anchors := make([]*cdp.Node, 0, 100)

		err := chromedp.Run(ctx, chromedp.Tasks{
			chromedp.Nodes(
				`//app-equity-search-result-table//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//a`,
				&anchors,
				chromedp.BySearch),
		})
		if err != nil {
			return nil, err
		}

		for _, a := range anchors {
			href, ok := a.Attribute("href")
			if ok {
				urls = append(urls, href)
			}
		}

		nodes := make([]*cdp.Node, 0, 100)
		err = runWithTimeOut(
			ctx,
			3*time.Second,
			chromedp.Tasks{
				chromedp.Nodes(
					`//app-page-bar[1]//button[contains(@class, 'active') and contains(@class,'page-bar-type-button-width-auto')]/following-sibling::button[contains(@class, 'page-bar-type-button-width-auto')]`,
					&nodes,
					chromedp.BySearch),
			},
		)
		if err != nil && err != context.DeadlineExceeded {
			return nil, err
		}

		//fmt.Println(len(nodes))
		if len(nodes) == 0 {
			break
		}

		err = chromedp.Run(ctx, chromedp.Tasks{
			chromedp.Click(
				`//app-page-bar[1]//button[contains(@class, 'active') and contains(@class,'page-bar-type-button-width-auto')]/following-sibling::button[contains(@class, 'page-bar-type-button-width-auto')][1]`,
				chromedp.BySearch),
			chromedp.WaitVisible(
				`//app-loading-spinner`,
				chromedp.BySearch),
			chromedp.WaitNotPresent(`//app-loading-spinner`,
				chromedp.BySearch),
		})
		if err != nil {
			return nil, err
		}
	}

	return urls, nil
}

func (env *env) execCmdExport() error {
	err := os.MkdirAll(env.database, 0755)
	if err != nil {
		return err
	}

	w, closeFn, err := openOut(env.out, env.force)
	if err != nil {
		return err
	}
	defer closeFn()

	secs, err := env.loadSecsFiles()
	if err != nil {
		return err
	}

	env.sort(secs)
	return env.export(w, secs)
}

func (env *env) sort(secs []*sec) {
	sort.Sort(bySubsector(secs))
}

type bySubsector []*sec

func (a bySubsector) Len() int {
	return len(a)
}

func (a bySubsector) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a bySubsector) Less(i, j int) bool {
	sec1 := a[i].Master["sector"]
	sec2 := a[j].Master["sector"]
	if sec1 < sec2 {
		return true
	}
	if sec1 > sec2 {
		return false
	}

	sub1 := a[i].Master["subsector"]
	sub2 := a[j].Master["subsector"]
	if sub1 < sub2 {
		return true
	}
	if sub1 > sub2 {
		return false
	}

	nam1 := a[i].Name
	nam2 := a[j].Name
	return nam1 < nam2
}

func (env *env) export(w io.Writer, secs []*sec) error {
	return env.exportCSV(w, secs)
}

func (env *env) exportCSV(w io.Writer, secs []*sec) error {
	enc, flushFn := env.newCSVWriter(w)
	defer flushFn()

	headers := []string{
		"Name",
		"ISIN",
		"Symbol",
		"Type",
		"Form",
		"Market",
		"Subsector",
		"Sector",
		"Industry",
	}
	err := enc.Write(headers)
	if err != nil {
		return err
	}

	for _, sec := range secs {
		rec := []string{
			sec.Name,
			sec.ISIN,
			sec.Symbol,
			sec.Master["type"],
			sec.Master["form"],
			sec.Master["market"],
			sec.Master["subsector"],
			sec.Master["sector"],
			sec.Master["industry"],
		}
		err := enc.Write(rec)
		if err != nil {
			return err
		}
	}

	return nil
}

func (env *env) newCSVWriter(w io.Writer) (*csv.Writer, func()) {
	enc := csv.NewWriter(w)
	enc.Comma = ';'
	return enc, func() { enc.Flush() }
}

func (env *env) loadSecsFiles() ([]*sec, error) {
	ctx, cancel := newChromeContext(context.Background())
	defer cancel()

	secs := make([]*sec, 0, 100)
	for _, p := range env.args {
		a, err := env.loadSecsFile(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("reading file: %v: %v", p, err)
		}
		secs = append(secs, a...)
	}

	return secs, nil
}

func (env *env) loadSecsFile(ctx context.Context, p string) ([]*sec, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return env.loadSecsReader(f)
}

func (env *env) loadSecsReader(r io.Reader) ([]*sec, error) {
	ctx, cancel := newChromeContext(context.Background())
	defer cancel()

	secs := make([]*sec, 0, 100)

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		u := strings.TrimSpace(sc.Text())
		if err := sc.Err(); err != nil {
			return nil, err
		}

		if u == "" || strings.HasPrefix(u, "#") {
			continue
		}

		p := dbFilepath(env.database, u)
		exists, err := isExistFile(p)
		if err != nil {
			return nil, err
		}
		if !exists {
			err := env.downloadUrl(ctx, p, u)
			if err != nil {
				return nil, err
			}
		}

		sec, err := env.loadSec(p)
		if err != nil {
			return nil, err
		}

		secs = append(secs, sec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return secs, nil
}

func (env *env) loadSec(p string) (*sec, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sec := &sec{}
	dec := json.NewDecoder(f)
	err = dec.Decode(sec)
	if err != nil {
		return nil, err
	}

	return sec, nil
}

func dbFilepath(dir, url string) string {
	bname := path.Base(url)
	return filepath.Join(dir, bname+".json")
}

func (env *env) downloadUrl(ctx context.Context, p, u string) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	f, err := os.OpenFile(p, flags, 0755)
	if err != nil {
		return err
	}
	defer f.Close()
	return env.downloadUrlWriter(ctx, f, u)
}

func (env *env) downloadUrlWriter(ctx context.Context, w io.Writer, u string) error {
	sec := &sec{
		URL:    u,
		Master: make(map[string]string),
	}

	nameNodes := make([]*cdp.Node, 0, 1)
	isinNodes := make([]*cdp.Node, 0, 1)
	symNodes := make([]*cdp.Node, 0, 1)
	typNodes := make([]*cdp.Node, 0, 1)
	labNodes := make([]*cdp.Node, 0, 100)
	valNodes := make([]*cdp.Node, 0, 100)

	err := chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Navigate(u),
		// wait for data to appear
		chromedp.WaitVisible(
			`//app-widget-performance//div[contains(@class, 'table-responsive')] `,
			chromedp.BySearch),
		chromedp.Nodes(
			`//h1[contains(@class, 'instrument-name')]//text()`,
			&nameNodes,
			chromedp.BySearch),
		chromedp.Nodes(
			`//span[contains(text(), 'ISIN:')]//text()`,
			&isinNodes,
			chromedp.BySearch),
		chromedp.Nodes(
			`//span[contains(text(), 'Symbol:')]//text()`,
			&symNodes,
			chromedp.BySearch),
		chromedp.Nodes(
			`//span[contains(text(), 'Type:')]//text()`,
			&typNodes,
			chromedp.BySearch),
		chromedp.Nodes(
			`//app-widget-equity-master-data//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//text()`,
			&labNodes,
			chromedp.BySearch),
		chromedp.Nodes(
			`//app-widget-equity-master-data//div[contains(@class, 'table-responsive')]//tbody//tr//td[2]//text()`,
			&valNodes,
			chromedp.BySearch),
	})
	if err != nil {
		return err
	}

	sec.Name = strings.TrimSpace(nameNodes[0].NodeValue)

	isin := strings.TrimSpace(isinNodes[0].NodeValue)
	isin = strings.TrimPrefix(isin, "ISIN:")
	isin = strings.TrimSpace(isin)
	sec.ISIN = isin

	sym := strings.TrimSpace(symNodes[0].NodeValue)
	sym = strings.TrimPrefix(sym, "|")
	sym = strings.TrimSpace(sym)
	sym = strings.TrimPrefix(sym, "Symbol:")
	sym = strings.TrimSpace(sym)
	sec.Symbol = sym

	typ := strings.TrimSpace(typNodes[0].NodeValue)
	typ = strings.TrimPrefix(typ, "|")
	typ = strings.TrimSpace(typ)
	typ = strings.TrimPrefix(typ, "Type:")
	typ = strings.TrimSpace(typ)
	sec.Type = typ

	for i := 0; i < len(labNodes); i++ {
		lab := strings.TrimSpace(labNodes[i].NodeValue)
		lab = strings.ToLower(lab)
		val := strings.TrimSpace(valNodes[i].NodeValue)
		sec.Master[lab] = val
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(&sec)
}

type sec struct {
	URL    string            `json:"url"`
	ISIN   string            `json:"isin"`
	Symbol string            `json:"symbol"`
	Type   string            `json:"type"`
	Name   string            `json:"name"`
	Master map[string]string `json:"master"`
}

func runWithTimeOut(
	ctx context.Context,
	timeout time.Duration,
	tasks chromedp.Tasks,
) error {
	timeoutContext, cancel := context.WithTimeout(
		ctx,
		timeout,
	)
	defer cancel()
	return chromedp.Run(timeoutContext, tasks)
}

type cmdHelp struct {
	name    cmdName
	syntax  string
	desc    string
	example string
}

var cmdHelps = []*cmdHelp{
	&cmdHelp{
		name:    cnUrls,
		syntax:  `fran urls [-out=<file>] [--force] <search_url>...`,
		desc:    `Collects page urls from search results.`,
		example: `fran urls -out="eu.txt" -force "https://www.boerse-frankfurt.de/equities/search?REGIONS=Europe&TYPE=1002&FORM=2&MARKET=REGULATED&ORDER_BY=NAME&ORDER_DIRECTION=ASC"`,
	},
	&cmdHelp{
		name:    cnExport,
		syntax:  `fran export [-format=<format>] [-out=<file>] [--force] <urls_file>...`,
		desc:    `Downloads master data from page urls and produces it in the specified format. See the supported formats at the -format option.`,
		example: `fran export -format="csv" -out="eu.csv" -force "eu.txt"`,
	},
}

func printUsage(fs *flag.FlagSet) {
	fmt.Println("Usage:")
	fmt.Println("  fran <command> [<option>...] [<arg>...]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, c := range cmdHelps {
		fmt.Println(" ", string(c.name))
		fmt.Println("   ", c.syntax)
		fmt.Println()
		fmt.Println("   ", c.desc)
		fmt.Println()
		fmt.Println("    Example")
		fmt.Println("   ", c.example)
		fmt.Println()
	}
	fmt.Println("Options:")
	fs.PrintDefaults()
}

func assertRegularFiles(ps ...string) error {
	for _, p := range ps {
		b, err := isRegularFile(p)
		if err != nil {
			return err
		}
		if !b {
			return fmt.Errorf("not a regular file: %v", p)
		}
	}
	return nil
}

func isRegularFile(p string) (bool, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return false, err
	}
	return fi.Mode().IsRegular(), nil
}

func assertExistFiles(ps ...string) error {
	for _, p := range ps {
		b, err := isExistFile(p)
		if err != nil {
			return err
		}
		if !b {
			return fmt.Errorf("file not exist: %v", p)
		}
	}
	return nil
}

func isExistFile(p string) (bool, error) {
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
