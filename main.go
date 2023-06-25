package main

import (
	"flag"
	"fmt"
	"os"
	//"context"
	//"github.com/chromedp/cdproto/runtime"
	//"github.com/chromedp/chromedp"
)


const flagUserAgent = "user-agent"

const defUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"


type cmd struct {
	cmd string
	syntax string
	desc string
	fn func(*flag.FlagSet)
}

func main() {
	cmds := []*cmd{
		&cmd{
			cmd: "csv",
			syntax: "fox csv [option...] search_url...",
			desc: "Collects page links from search results. Downloads master data page links. Prints master data in comma separated values (CSV).",
			fn: cmdCsv,
		},
	}

	flags := flag.NewFlagSet("flags", flag.ExitOnError)
	addUserAgentFlag(flags)

	if len(os.Args) < 2 {
		fmt.Println("no command")
		printUsage(cmds, flags)
		os.Exit(1)
	}


	flags.Parse(os.Args[2:])
	//fmt.Println("user-agent:", flags.Lookup(flagUserAgent).Value )

	cmd := os.Args[1]
	for _, c := range cmds {
		if c.cmd == cmd {
			c.fn(flags)
			return
		}
	}

	fmt.Println("unexpected command")
	printUsage(cmds, flags)
	os.Exit(1)
}

func cmdLinks(flags *flag.FlagSet) {
	fmt.Println("links")
}

func cmdPages(flags *flag.FlagSet) {
	fmt.Println("pages")
}

func cmdCsv(flags *flag.FlagSet) {
	fmt.Println("csv")
}

func addUserAgentFlag(cmd *flag.FlagSet) *string {
	return cmd.String(flagUserAgent, defUserAgent, "User agent header value")
}

func printUsage(cmds []*cmd, flags *flag.FlagSet) {
	fmt.Println("Commands:")
	for _, c := range cmds {
		fmt.Println(" ", c.cmd)
		fmt.Println("   ", c.syntax)
		fmt.Println()
		fmt.Println("   ", c.desc)
	}
	fmt.Println()
	fmt.Println("Options:")
	flags.PrintDefaults()
}

