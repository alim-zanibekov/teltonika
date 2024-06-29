`io_elements_parser.go` - parses IO Elements descriptions recursively
starting from [this wiki page](https://wiki.teltonika-gps.com/view/FMB920_Teltonika_Data_Sending_Parameters_ID) by
default.

```shell
go run io_elements_parser.go -h 
```

```text
Usage of ./io_elements_parser
  -address value
        addresses to parse, can be specified multiple times
        (default https://wiki.teltonika-gps.com/view/FMB920_Teltonika_Data_Sending_Parameters_ID)
  -cache-output string
        http cache file (default "./cache.bin")
  -csv-output string
        csv file path (default "./io_elements_dump.csv")
  -output string
        output file for I/O elements definitions list (default "./ioelements_dump.go")
  -output-pkg-name string
        package name for output (default "ioelements")
```

You can generate the list yourself and use it via the `ioelements`
package by passing it to the NewParser function

```shell
go run io_elements_parser.go -output ./my_ioelements.go -output-pkg-name main
```

```go
package main

import (
    "fmt"

    "github.com/alim-zanibekov/teltonika/ioelements"
)

func main() {
    myParser := ioelements.NewParser(ioElementDefinitions) // ioElementDefinitions from ./my_ioelements.go
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