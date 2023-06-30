package main

import (
	"bufio"
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
	cnLinks  cmdName = "links"
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
	if val == "" {
		return true, nil
	}

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
		return false, fmt.Errorf("unknown logical value: " + val)
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
	return parseCmdText(cnLinks, fs)
}

func parseCmdExport(fs *flag.FlagSet) (*cmd, error) {
	return parseCmdText(cnExport, fs)
}

func parseCmdText(name cmdName, fs *flag.FlagSet) (*cmd, error) {
	cmd := &cmd{
		name: name,
		args: fs.Args(),
	}
	var err error

	fs.Visit(func(f *flag.Flag) {
		if err != nil {
			return
		}

		switch f.Name {
		case flagOut:
			cmd.out = flagVal(fs, flagOut)
		case flagForce:
			frc, e := parseForce(flagVal(fs, flagForce))
			if e != nil {
				err = e
				return
			}
			cmd.force = frc
		case flagFormat:
			frm, e := parseFormat(flagVal(fs, flagFormat))
			if e != nil {
				err = e
				return
			}
			cmd.format = frm
		default:
			err = fmt.Errorf("unexpected option: " + f.Name)
		}
	})

	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func addOutFlag(cmd *flag.FlagSet) *string {
	return cmd.String(flagOut, "", "Output file path. Use the -format option to specify the format.")
}

func addForceFlag(cmd *flag.FlagSet) *bool {
	return cmd.Bool(flagForce, false, "Overwrite output file if it already exists.")
}

func addFormatFlag(cmd *flag.FlagSet) *string {
	return cmd.String(flagFormat, "xlsx", "Format of the output file. Supported values are: xlsx.")
}

func flagVal(fs *flag.FlagSet, name string) string {
	return fs.Lookup(name).Value.String()
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
			chromedp.WaitVisible(`//app-equity-search-result-table//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//a`, chromedp.BySearch),
			chromedp.WaitNotPresent(`//app-loading-spinner`, chromedp.BySearch),
			chromedp.Click(`//app-page-bar//button[contains(text(), '100')]`, chromedp.BySearch),
			chromedp.WaitVisible(`//app-loading-spinner`, chromedp.BySearch),
			chromedp.WaitNotPresent(`//app-loading-spinner`, chromedp.BySearch),
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
	links := make(map[string]struct{}, 100)
	for {
		attrs := make([]map[string]string, 0, 100)
		nodes := make([]*cdp.Node, 0, 100)

		err := chromedp.Run(ctx, chromedp.Tasks{
			chromedp.AttributesAll(`//app-equity-search-result-table//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//a`, &attrs, chromedp.BySearch),
		})
		if err != nil {
			return nil, err
		}

		for _, a := range attrs {
			rel, err := url.Parse(a["href"])
			if err != nil {
				return nil, err
			}
			links[rel.String()] = struct{}{}
		}

		err = runWithTimeOut(
			ctx,
			3*time.Second,
			chromedp.Tasks{
				chromedp.Nodes(`//app-page-bar[1]//button[contains(@class, 'active') and contains(@class,'page-bar-type-button-width-auto')]/following-sibling::button[contains(@class, 'page-bar-type-button-width-auto')]`, &nodes, chromedp.BySearch),
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
			chromedp.Click(`//app-page-bar[1]//button[contains(@class, 'active') and contains(@class,'page-bar-type-button-width-auto')]/following-sibling::button[contains(@class, 'page-bar-type-button-width-auto')][1]`, chromedp.BySearch),
			chromedp.WaitVisible(`//app-loading-spinner`, chromedp.BySearch),
			chromedp.WaitNotPresent(`//app-loading-spinner`, chromedp.BySearch),
		})
		if err != nil {
			return nil, err
		}
	}

	res := make([]string, 0, len(links))
	for k, _ := range links {
		res = append(res, k)
	}

	return res, nil
}

func execCmdExport(cmd *cmd) error {
	files := cmd.args
	if len(files) == 0 {
		return fmt.Errorf("no link files")
	}

	if cmd.out == "" {
		return fmt.Errorf("output file path must be specified, use the -out option")
	}
	w, closeFn, err := openOut(cmd.out, cmd.force)
	if err != nil {
		return err
	}
	defer closeFn()

	ctx, cancel := newChromeContext(context.Background())
	defer cancel()

	for _, p := range files {
		err := exportLinksFile(ctx, w, p)
		if err != nil {
			return err
		}
	}

	return nil
}

func exportLinksFile(ctx context.Context, w io.Writer, p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return exportLinksReader(ctx, w, f)
}

func exportLinksReader(ctx context.Context, w io.Writer, r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		fmt.Println(l)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}

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
		syntax:  "fran links [-out <file>] [--force] <search_url>...",
		desc:    "Collects page links from search results.",
		example: "fran links -out links-eu.txt \"https://www.boerse-frankfurt.de/equities/search?REGIONS=Europe&TYPE=1002&FORM=2&ORDER_BY=NAME&ORDER_DIRECTION=ASC\"",
	},
	&cmdHelp{
		name:    cnExport,
		syntax:  "fran export [-format <format>] [-out <file>] [--force] <links_file>...",
		desc:    "Downloads master data from page links and produces it in the specified format. See the supported formats at the -format option.",
		example: "fran export -format xlsx -out eu-and-us.xlsx links-eu.txt links-us.txt",
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
