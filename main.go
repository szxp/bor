package main

import (
	"flag"
	"fmt"
	"sort"
	"time"
	//"log"
	"context"
	"net/url"
	"os"
	"path"
	//"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const flagUserAgent = "user-agent"

const defUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"

type cmd struct {
	cmd     string
	syntax  string
	desc    string
	example string
	fn      func(*flag.FlagSet) error
}

func main() {
	cmds := []*cmd{
		&cmd{
			cmd:     "csv",
			syntax:  "fran [<option>...] csv <search_url>...",
			desc:    "Collects page links from search results. Downloads master data from page links. Prints master data in comma separated values (CSV).",
			example: "fran csv \"https://www.boerse-frankfurt.de/equities/search?REGIONS=Europe&TYPE=1002&FORM=2&ORDER_BY=NAME&ORDER_DIRECTION=ASC\"",
			fn:      cmdCsv,
		},
	}

	cmd := flag.CommandLine
	addUserAgentFlag(cmd)

	flag.Usage = func() {
		printUsage(cmds, cmd)
		return
	}

	if len(os.Args) < 2 {
		fmt.Println("no command")
		printUsage(cmds, cmd)
		os.Exit(1)
	}

	flag.Parse()
	//fmt.Println("user-agent:", cmd.Lookup(flagUserAgent).Value)

	c0 := cmd.Args()[0]
	for _, c := range cmds {
		if c.cmd == c0 {
			err := c.fn(cmd)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			return
		}
	}

	fmt.Println("unexpected command")
	printUsage(cmds, cmd)
	os.Exit(1)
}

func cmdCsv(cmd *flag.FlagSet) error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-extensions", false),
	)

	actx, acancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer acancel()
	ctx, cancel := chromedp.NewContext(
		actx,
		//chromedp.WithDebugf(log.Printf),
	)
	defer cancel()

	urls := cmd.Args()[1:]
	links, err := getLinks(ctx, urls)
	if err != nil {
		return err
	}

	sort.Strings(links)
	for _, l := range links {
		fmt.Println(l)
	}
	return nil
}

func getLinks(ctx context.Context, urls []string) ([]string, error) {
	links := make(map[string]struct{}, 15000)
	res := make([]string, 0, 15000)

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
			return nil, err
		}

		rels, err := getLinksRel(ctx)
		if err != nil {
			return nil, err
		}

		uu, err := url.Parse(u)
		if err != nil {
			return nil, err
		}

		host := uu.Scheme + "://" + uu.Host
		for _, rel := range rels {
			l, err := url.Parse(path.Join(host, rel))
			if err != nil {
				return nil, err
			}
			links[l.String()] = struct{}{}
		}
	}

	for k, _ := range links {
		res = append(res, k)
	}

	return res, nil
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
		if err != nil {
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
	err := chromedp.Run(timeoutContext, tasks)
	if err != nil && err != context.DeadlineExceeded {
		return err
	}
	return nil
}

func addUserAgentFlag(cmd *flag.FlagSet) *string {
	return cmd.String(flagUserAgent, defUserAgent, "User-agent HTTP header value")
}

func printUsage(cmds []*cmd, cmd *flag.FlagSet) {
	fmt.Println("Usage:")
	fmt.Println("  fran [<option>...] <command> [<arg>...]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, c := range cmds {
		fmt.Println(" ", c.cmd)
		fmt.Println("   ", c.syntax)
		fmt.Println()
		fmt.Println("   ", c.desc)
		fmt.Println()
		fmt.Println("    Example")
		fmt.Println("   ", c.example)
	}
	fmt.Println()
	fmt.Println("Options:")
	cmd.PrintDefaults()
}
