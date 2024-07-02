`io_elements_gen.go` - IO Elements generator.
Recursively parses descriptions of I/O elements from wiki pages.
by recursively following the links to other models and writes output to csv and go files.

```shell
go run io_elements_gen.go -h
go run io_elements_gen.go load-net -h
go run io_elements_gen.go load-csv -h
```

```text
Usage:
  io_elements_gen [OPTIONS] load-net [load-net-OPTIONS]
  io_elements_gen [OPTIONS] load-csv InputCsvFile

Application Options:
  -o, --gen-out=      output file path for I/O elements definitions list (default: ./ioelements_dump.go)
      --no-gen        disable go file generation
      --gen-pkg-name= package name for generated file (default: main)
      --gen-internal  generate file for internal usage in ioelements package

Help Options:
  -h, --help          Show this help message

Available commands:
  load-csv
  load-net

[load-net command options]
  -m, --model=     models to parse, can be specified multiple times (default: FMB920, FMC650)
    --csv-out=     csv output file path
    --cache=       path to http cache file (default: ./cache.bin)
    --no-cache     disable http cache
    --no-follow    disable recursive links following
    --url-pattern= url generation pattern from specified model names
                   substring '{model}' will be substituted
                   (default: https://wiki.teltonika-gps.com/view/{model}_Teltonika_Data_Sending_Parameters_ID)
```

You can generate the list yourself and use it via the `ioelements`
package by passing it to the NewParser function.

This command loads a list of I/O element definitions for FMB920 and FMB900 without
following links to other models, and then writes the result to a go file `my_ioelements.go`
with the package name `main`.

```shell
go run io_elements_gen.go load-net -m FMB920 -m FMB900 --no-follow -o my_ioelements.go --gen-pkg-name=main
```

Example usage
```go
package main

import (
    "fmt"

    "github.com/alim-zanibekov/teltonika/ioelements"
)

func main() {
    myParser := ioelements.NewParser(ioElementDefinitions) // ioElementDefinitions from the generated ./my_ioelements.go
    parsed, err := myParser.Parse("*", 1, []byte{1})
    if err != nil {
        panic(err)
    }
    fmt.Printf("%s\n", parsed)
}
```

Output

```text
Digital Input 1: true
```

Other examples

Load FMB920 and FMB900 and write results to the `io_elements_dump.csv`
```shell
go run io_elements_gen.go load-net -m FMB920 -m FMB900 --no-follow --no-gen --csv-out ./io_elements_dump.csv
```

Load data from the `io_elements_dump.csv` file and generate `my_ioelements.go` file
```shell
go run io_elements_gen.go load-csv ./io_elements_dump.csv -o my_ioelements.go --gen-pkg-name=main
```

The way how current `/ioelements/ioelements_dump.go` was generated
```shell
go run io_elements_gen.go load-net -m FMB920 -m FMC650 -m FMC225 -m FMB225 --csv-out ./io_elements_dump.csv -o ../ioelements/ioelements_dump.go --gen-internal --gen-pkg-name ioelements 
```