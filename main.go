package main

import (
	"flag"
	"fmt"
	//"log"
	"context"
	"net/url"
	"os"
	"path"
	//"github.com/chromedp/cdproto/runtime"
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

	fmt.Println(links)
	return nil
}

func getLinks(ctx context.Context, urls []string) ([]string, error) {
	links := make(map[string]struct{}, 15000)
	for _, u := range urls {
		attrs := make([]map[string]string, 0, 100)
		err := chromedp.Run(ctx, chromedp.Tasks{
			chromedp.Navigate(u),
			//chromedp.WaitVisible(`//button[contains(text(),'Ablehnen')]`, chromedp.BySearch),
			//chromedp.Click(`//button[contains(text(),'Ablehnen')]`, chromedp.BySearch),
			// search results loaded
			chromedp.WaitVisible(`//app-equity-search-result-table//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//a`, chromedp.BySearch),
			chromedp.Click(`//app-page-bar//button[contains(text(), '100')]`, chromedp.BySearch),
			chromedp.WaitVisible(`//app-loading-spinner`, chromedp.BySearch),
			chromedp.WaitNotPresent(`//app-loading-spinner`, chromedp.BySearch),
			chromedp.AttributesAll(`//app-equity-search-result-table//div[contains(@class, 'table-responsive')]//tbody//tr//td[1]//a`, &attrs, chromedp.BySearch),
		})
		if err != nil {
			return nil, err
		}

		uu, err := url.Parse(u)
		if err != nil {
			return nil, err
		}

		host := uu.Scheme + "://" + uu.Host
		for _, a := range attrs {
			l := path.Join(host, a["href"])
			links[l] = struct{}{}
		}
	}

	res := make([]string, 0, len(links))
	for k, _ := range links {
		res = append(res, k)
	}

	return res, nil
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
