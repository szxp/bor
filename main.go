package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	//"sort"
	"time"
	//"log"
	"context"
	"io"
	"net/url"
	"os"
	"path"
	//"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func main() {
	cmd, fs, err := parseCmd()
	if err != nil {
		fmt.Println(err)
		fmt.Println()
		printUsage(fs)
		os.Exit(1)
	}

	err = execCmd(cmd)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

const (
	flagOut    = "out"
	flagForce  = "force"
	flagFormat = "format"
)

type cmd struct {
	name   cmdName
	args   []string
	out    string
	force  bool
	format format
}

const (
	cnLinks  cmdName = "urls"
	cnExport cmdName = "export"
)

type cmdName string

func parseCmdName(name string) (cmdName, error) {
	switch name {
	case string(cnLinks):
		return cnLinks, nil
	case string(cnExport):
		return cnExport, nil
	default:
		return "", fmt.Errorf("unknown command: " + name)
	}
}

const (
	fXlsx format = "xlsx"
	fCsv  format = "csv"
)

type format string

func parseFormat(val string) (format, error) {
	switch val {
	case string(fXlsx):
		return fXlsx, nil
	case string(fCsv):
		return fCsv, nil
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

func parseCmd() (*cmd, *flag.FlagSet, error) {
	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.Usage = func() {
		printUsage(fs)
	}
	addOutFlag(fs)
	addForceFlag(fs)
	addFormatFlag(fs)

	if len(os.Args) < 2 {
		return nil, fs, fmt.Errorf("no command")
	}

	name, err := parseCmdName(os.Args[1])
	if err != nil {
		return nil, fs, err
	}

	err = fs.Parse(os.Args[2:])
	if err != nil {
		return nil, fs, err
	}

	cmd, err := parseCmdOptions(name, fs)
	if err != nil {
		return nil, fs, err
	}
	return cmd, fs, nil
}

func parseCmdOptions(name cmdName, fs *flag.FlagSet) (*cmd, error) {
	switch name {
	case cnLinks:
		return parseCmdLinks(fs)
	case cnExport:
		return parseCmdExport(fs)
	default:
		return nil, fmt.Errorf("unknown command: " + string(name))
	}
}

func parseCmdLinks(fs *flag.FlagSet) (*cmd, error) {
	cmd, err := parseCmdText(cnLinks, fs)
	if err != nil {
		return nil, err
	}

	if len(cmd.args) == 0 {
		return nil, fmt.Errorf("no search urls as arguments")
	}

	return cmd, nil
}

func parseCmdExport(fs *flag.FlagSet) (*cmd, error) {
	cmd, err := parseCmdText(cnExport, fs)
	if err != nil {
		return nil, err
	}

	if cmd.out == "" {
		return nil, fmt.Errorf("output file must be specified, see -out option")
	}

	if len(cmd.args) == 0 {
		return nil, fmt.Errorf("no link files as arguemnts")
	}

	err = assertExistFiles(cmd.args...)
	if err != nil {
		return nil, err
	}

	err = assertRegularFiles(cmd.args...)
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func parseCmdText(name cmdName, fs *flag.FlagSet) (*cmd, error) {
	var err error

	cmd := &cmd{
		name: name,
		args: fs.Args(),
	}

	fs.Visit(func(f *flag.Flag) {
		if err != nil {
			return
		}

		switch f.Name {
		case flagOut:
			cmd.out = f.Value.String()
		case flagForce:
			frc, e := parseForce(f.Value.String())
			if e != nil {
				err = e
				return
			}
			cmd.force = frc
		case flagFormat:
			frm, e := parseFormat(f.Value.String())
			if e != nil {
				err = e
				return
			}
			cmd.format = frm
		default:
			err = fmt.Errorf("unknown option: " + f.Name)
		}
	})

	if cmd.out != "" {
		exist, err := isExistFile(cmd.out)
		if err != nil {
			return nil, err
		}

		if exist {
			b, err := isRegularFile(cmd.out)
			if err != nil {
				return nil, err
			}
			if !b {
				return nil, fmt.Errorf("not a regular file: %v", cmd.out)
			}

			if !cmd.force {
				return nil, fmt.Errorf(
					"file already exists: %v, use the -force option to overwrite it",
					cmd.out)
			}
		}

	}

	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func addOutFlag(cmd *flag.FlagSet) *string {
	return cmd.String(
		flagOut,
		"",
		"Output file path. Use the -format option to specify the format.")
}

func addForceFlag(cmd *flag.FlagSet) *bool {
	return cmd.Bool(
		flagForce,
		false,
		"Overwrite output file if it already exists.")
}

func addFormatFlag(cmd *flag.FlagSet) *string {
	return cmd.String(
		flagFormat,
		"xlsx",
		"Format of the output file. Supported values are: xlsx.")
}

func execCmd(cmd *cmd) error {
	switch cmd.name {
	case cnLinks:
		return execCmdLinks(cmd)
	case cnExport:
		return execCmdExport(cmd)
	default:
		return fmt.Errorf("unexpected command: " + string(cmd.name))
	}
}

func execCmdLinks(cmd *cmd) error {
	w, closeFn, err := openOut(cmd.out, cmd.force)
	if err != nil {
		return err
	}
	defer closeFn()
	return getLinks(w, cmd.args)
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

func getLinks(w io.Writer, urls []string) error {
	ctx, cancel := newChromeContext(context.Background())
	defer cancel()

	for _, u := range urls {
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

		rels, err := getLinksRel(ctx)
		if err != nil {
			return err
		}

		uu, err := url.Parse(u)
		if err != nil {
			return err
		}

		host := uu.Scheme + "://" + uu.Host
		for _, rel := range rels {
			l, err := url.Parse(path.Join(host, rel))
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(w, l)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getLinksRel(ctx context.Context) ([]string, error) {
	urls := make(map[string]struct{}, 100)
	for {
		attrs := make([]map[string]string, 0, 100)

		err := chromedp.Run(ctx, chromedp.Tasks{
			chromedp.AttributesAll(
				`//app-equity-search-result-table//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//a`,
				&attrs,
				chromedp.BySearch),
		})
		if err != nil {
			return nil, err
		}

		for _, a := range attrs {
			rel, err := url.Parse(a["href"])
			if err != nil {
				return nil, err
			}
			urls[rel.String()] = struct{}{}
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

	res := make([]string, 0, len(urls))
	for k, _ := range urls {
		res = append(res, k)
	}

	return res, nil
}

func execCmdExport(cmd *cmd) error {
	w, closeFn, err := openOut(cmd.out, cmd.force)
	if err != nil {
		return err
	}
	defer closeFn()

	ctx, cancel := newChromeContext(context.Background())
	defer cancel()

	for _, p := range cmd.args {
		err := exportFile(ctx, w, p)
		if err != nil {
			return err
		}
	}

	return nil
}

func exportFile(ctx context.Context, w io.Writer, p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return exportReader(ctx, w, f)
}

func exportReader(ctx context.Context, w io.Writer, r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		u := strings.TrimSpace(sc.Text())
		if sc.Err() != nil {
			break
		}

		err := exportUrl(ctx, w, u)
		if err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}

	return nil
}

type sec struct {
	URL    string            `json:"url"`
	ISIN   string            `json:"isin"`
	Symbol string            `json:"symbol"`
	Type   string            `json:"type"`
	Name   string            `json:"name"`
	Master map[string]string `json:"master"`
}

func exportUrl(ctx context.Context, w io.Writer, u string) error {
	//fmt.Println(u)

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

	b, err := json.MarshalIndent(&sec, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(b))

	return nil
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
		name:    cnLinks,
		syntax:  `fran urls [-out=<file>] [--force] <search_url>...`,
		desc:    `Collects page urls from search results.`,
		example: `fran urls -out="urls-eu.txt" "https://www.boerse-frankfurt.de/equities/search?REGIONS=Europe&TYPE=1002&FORM=2&ORDER_BY=NAME&ORDER_DIRECTION=ASC"`,
	},
	&cmdHelp{
		name:    cnExport,
		syntax:  `fran export [-format=<format>] [-out=<file>] [--force] <urls_file>...`,
		desc:    `Downloads master data from page urls and produces it in the specified format. See the supported formats at the -format option.`,
		example: `fran export -format=xlsx -out="eu-and-us.xlsx" "urls-eu.txt" "urls-us.txt"`,
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
