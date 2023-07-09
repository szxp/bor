# fran
Börse Frankfurt scraper

Downloads master data from page urls and produces it in the specified format (i. e. CSV). Uses Chrome browser for scaping, so it must be installed.

## Run
```bash
git clone https://github.com/szxp/fran
cd fran
go build
./fran urls -out="eu.txt" -force "https://www.boerse-frankfurt.de/equities/search?REGIONS=Europe&TYPE=1002&FORM=2&MARKET=REGULATED&ORDER_BY=NAME&ORDER_DIRECTION=ASC"
./fran export -format="csv" -out="eu.csv" -force "eu.txt"
```

## Usage
```
Usage:
  fran <command> [<option>...] [<arg>...]

Commands:
  urls
    fran urls [-out=<file>] [--force] <search_url>...

    Collects page urls from search results.

    Example
    fran urls -out="eu.txt" "https://www.boerse-frankfurt.de/equities/search?REGIONS=Europe&TYPE=1002&FORM=2&MARKET=REGULATED&ORDER_BY=NAME&ORDER_DIRECTION=ASC"

  export
    fran export [-format=<format>] [-out=<file>] [--force] <urls_file>...

    Downloads master data from page urls and produces it in the specified format. See the supported formats at the -format option. Lines starting with hashmark (#) in the urls file are considered as comments and will be skipped.

    Example
    fran export -format="csv" -out="eu.csv" "eu.txt"

Options:
  -database string
        Database directory where the downloaded data will be saved and cached. (default "frandb")
  -force
        Overwrite output file if it already exists.
  -format string
        Format of the output file. Supported values are: csv. (default "csv")
  -out string
        Output file path. If not specified or an empty string is specified the output will be written to the standard output. Use the -format option to specify the format of the file.
```
