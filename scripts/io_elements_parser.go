package main

import (
	"bytes"
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/slices"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type ElementType uint8

const (
	IOElementSigned ElementType = iota
	IOElementUnsigned
	IOElementHEX
	IOElementASCII
)

type IOElementDefinition struct {
	Id              uint16      `json:"id"`
	Name            string      `json:"name"`
	NumBytes        int         `json:"numBytes"`
	Type            ElementType `json:"type"`
	Min             float64     `json:"min"`
	Max             float64     `json:"max"`
	Multiplier      float64     `json:"multiplier"`
	Units           string      `json:"units"`
	Description     string      `json:"description"`
	SupportedModels []string    `json:"supportedModels"`
	Groups          []string    `json:"groups"`
}

var typesMap = map[string]ElementType{
	"unsigned":          IOElementUnsigned,
	"signed":            IOElementSigned,
	"hex":               IOElementHEX,
	"ascii":             IOElementASCII,
	"unsigned long int": IOElementUnsigned,
	"unsiged":           IOElementUnsigned,
	"<string>":          IOElementASCII,
}

type stringArrayFlag []string

func (i *stringArrayFlag) String() string {
	return strings.Join(*i, ", ")
}

func (i *stringArrayFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var addresses stringArrayFlag
var ioElementsDumpOutput string
var ioElementsPackageName string
var csvOutput string
var cacheOutput string

func main() {
	addresses = []string{"https://wiki.teltonika-gps.com/view/FMB920_Teltonika_Data_Sending_Parameters_ID"}
	flag.Var(&addresses, "address", "addresses to parse, can be specified multiple times\n"+
		"(default https://wiki.teltonika-gps.com/view/FMB920_Teltonika_Data_Sending_Parameters_ID)")
	flag.StringVar(&ioElementsDumpOutput, "output", "./ioelements_dump.go", "output file for I/O elements definitions list")
	flag.StringVar(&ioElementsPackageName, "output-pkg-name", "ioelements", "package name for output")
	flag.StringVar(&csvOutput, "csv-output", "./io_elements_dump.csv", "csv file path")
	flag.StringVar(&cacheOutput, "cache-output", "./cache.bin", "http cache file")
	flag.Parse()
	print(addresses)
	elements := parseElementsNetwork()
	slices.SortStableFunc(elements, func(a, b *IOElementDefinition) int {
		return int(a.Id) - int(b.Id)
	})
	writeCsv(elements)
	generateFromCsv()
}

//goland:noinspection GoUnhandledErrorResult
func generateFromCsv() {
	f, err := os.Open(csvOutput)
	if err != nil {
		log.Fatal(err)
	}
	csvReader := csv.NewReader(f)
	defer f.Close()

	rows, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	f, err = os.Create(ioElementsDumpOutput)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var typesMapStr = map[string]string{
		"unsigned": "IOElementUnsigned",
		"signed":   "IOElementSigned",
		"hex":      "IOElementHEX",
		"ascii":    "IOElementASCII",
	}
	f.WriteString("package " + ioElementsPackageName + "\n\n")
	prefix := ""
	if ioElementsPackageName != "ioelements" {
		f.WriteString(`import "github.com/alim-zanibekov/teltonika/ioelements"` + "\n\n")
		prefix = "ioelements."
	}
	f.WriteString("var ioElementDefinitions = []" + prefix + "IOElementDefinition{\n")
	wrapStr := func(s interface{}) string {
		str, _ := json.Marshal(s)
		return string(str)
	}
	var itemMap = map[string]map[int]int{}
	for i, cols := range rows[1:] {
		id, _ := strconv.Atoi(cols[0])
		models := strings.Split(cols[9], ",")
		for _, model := range models {
			_, ok := itemMap[model]
			if !ok {
				itemMap[model] = map[int]int{}
			}
			_, ok = itemMap[model][id]
			if !ok {
				itemMap[model][id] = i
			}
		}

		f.WriteString(
			fmt.Sprintf("\t{%s, %s, %s, %s, %s, %s, %s, %s, %s, []string{%s}, []string{%s}},\n",
				cols[0], wrapStr(cols[1]), cols[2], prefix+typesMapStr[cols[3]], cols[4], cols[5],
				cols[6], wrapStr(cols[7]), wrapStr(cols[8]),
				strings.TrimSuffix(wrapStr(models)[1:], "]"),
				strings.TrimSuffix(wrapStr(strings.Split(cols[10], ","))[1:], "]")),
		)
	}
	f.WriteString("}\n")

	if false {
		f.WriteString("\nvar supportedModels = []string{\n")
		for key := range itemMap {
			f.WriteString("    ")
			f.WriteString(wrapStr(key))
			f.WriteString(",\n")
		}
		f.WriteString("}\n")

		f.WriteString("\nvar indexByModelName = map[string]map[uint16]*IOElementDefinition{\n")
		for key, value := range itemMap {
			f.WriteString("    ")
			f.WriteString(wrapStr(key))
			f.WriteString(": {\n")
			for id, index := range value {
				f.WriteString(fmt.Sprintf("        %d: &d[%d],\n", id, index))
			}
			f.WriteString("    },\n")
		}

		f.WriteString("}\n")
	}
}

//goland:noinspection GoUnhandledErrorResult
func writeCsv(data []*IOElementDefinition) {
	f, err := os.Create(csvOutput)
	if err != nil {
		log.Fatal(err)
	}
	csvWriter := csv.NewWriter(f)
	defer func() {
		csvWriter.Flush()
		f.Close()
	}()

	var typesMapRev = map[ElementType]string{
		IOElementUnsigned: "unsigned",
		IOElementSigned:   "signed",
		IOElementHEX:      "hex",
		IOElementASCII:    "ascii",
	}
	csvWriter.Write([]string{
		"Property ID",
		"Property Name",
		"Bytes",
		"Type",
		"Min",
		"Max",
		"Multiplier",
		"Units",
		"Description",
		"SupportedModels",
		"Parameter Groups",
	})
	for _, item := range data {
		cols := []string{
			fmt.Sprint(item.Id),
			fmt.Sprint(item.Name),
			fmt.Sprint(item.NumBytes),
			typesMapRev[item.Type],
			strings.TrimSuffix(fmt.Sprintf("%.3f", item.Min), ".000"),
			strings.TrimSuffix(fmt.Sprintf("%.3f", item.Max), ".000"),
			fmt.Sprint(item.Multiplier),
			item.Units,
			item.Description,
			strings.Join(item.SupportedModels, ","),
			strings.Join(item.Groups, ","),
		}
		csvWriter.Write(cols)
	}
}

func parseElementsNetwork() []*IOElementDefinition {
	urlsToProcess := make([]string, 0)
	for _, address := range addresses {
		urlsToProcess = append(urlsToProcess, address)
	}

	results := make([]*IOElementDefinition, 0)
	for i := 0; i < len(urlsToProcess); i++ {
		log.Printf("Parsing %s, %d remaining", urlsToProcess[i], len(urlsToProcess)-1-i)
		walkRows(urlsToProcess[i], func(cols []string, tds *goquery.Selection) {
			df := parsePageRow(cols, tds)
			if df == nil {
				return
			}
			links := tds.Eq(9).Find("a").Map(func(_ int, it *goquery.Selection) string {
				link := it.AttrOr("href", "")
				if link == "" {
					return ""
				}
				return "https://wiki.teltonika-gps.com" + link + "_Teltonika_Data_Sending_Parameters_ID"
			})
			for _, link := range links {
				if link != "" && !slices.Contains(urlsToProcess, link) {
					urlsToProcess = append(urlsToProcess, link)
				}
			}
			if len(df.SupportedModels) == 0 {
				log.Printf("Skipping %d, no supported models found [page: %s]", df.Id, urlsToProcess[i])
				return
			}

			for i, it := range results {
				if it.Id == df.Id {
					c, ok := compareDefs(it, df)
					if !ok {
						if !overlap(it.SupportedModels, df.SupportedModels) {
							continue
						}
						if !cmp.Equal(it.SupportedModels, df.SupportedModels) &&
							!(df.NumBytes == it.NumBytes && df.Type == it.Type && df.Max == it.Max && df.Min == it.Min) {
							log.Printf("Two definitions with the same id and overlapping SupportedModels are not equal to each other")
							log.Printf("%v", string(ignoreError(json.Marshal(it))))
							log.Printf("%v", string(ignoreError(json.Marshal(df))))
						}
					}
					if c <= 0 {
						return
					} else {
						results = append(results[:i], results[i+1:]...)
					}
				}
			}
			results = append(results, df)
		})
		log.Printf("%d remaining", len(urlsToProcess))
	}

	return results
}

func parsePageRow(cols []string, tds *goquery.Selection) *IOElementDefinition {
	id, err := strconv.Atoi(cols[0])
	if err != nil {
		log.Fatalf("[%v] unable to parse id '%s'", id, cols[0])
	}

	name := cols[1]
	rawNumBytes := cols[2]
	rawType := cols[3]
	rawMin := cols[4]
	rawMax := cols[5]
	rawMultiplier := cols[6]

	var numBytes int
	if _, err := fmt.Sscan(rawNumBytes, &numBytes); err != nil {
		numBytes = -1
		if !strings.Contains(strings.ToLower(rawNumBytes), "variable") {
			log.Printf("[%v] invalid num bytes", id)
		}
	}
	elementType, ok := typesMap[strings.ToLower(rawType)]
	if !ok {
		log.Printf("[%v] invalid type '%s', set to unsigned", id, rawType)
		elementType = IOElementUnsigned
		numBytes = -1
	}

	var min, max float64
	if rawMin != "" && rawMax != "" && !strings.HasPrefix(rawMax, "0x") && !strings.HasPrefix(rawMin, "0x") {
		min = asFloat(rawMin)
		max = asFloat(rawMax)
	}

	multiplier := 1.0
	if rawMultiplier != "" {
		if _, err := fmt.Sscan(rawMultiplier, &multiplier); err != nil {
			log.Printf("[%d] Unable to parse '%v' as float64", id, rawMultiplier)
		}
	}

	supportedModels := make([]string, 0)
	tds.Eq(9).Find("a").Each(func(i int, it *goquery.Selection) {
		if asText(it) == "" {
			return
		}
		supportedModels = append(supportedModels, asText(it))
	})

	groups := make([]string, 0)
	if len(cols) >= 11 {
		groupsRaw := strings.Split(cols[10], ",")
		for i := range groupsRaw {
			groupsRaw[i] = strings.Trim(groupsRaw[i], " \n\t")
		}
		groups = groupsRaw
	}
	slices.Sort(supportedModels)
	slices.Sort(groups)
	if (numBytes == -1 || numBytes > 8) && elementType != IOElementHEX && elementType != IOElementASCII {
		elementType = IOElementHEX
	}
	units := cols[7]
	if id == 9 {
		units = "V"
	}
	def := &IOElementDefinition{
		Id:              uint16(id),
		Name:            name,
		NumBytes:        numBytes,
		Type:            elementType,
		Min:             min,
		Max:             max,
		Multiplier:      multiplier,
		Units:           units,
		Description:     cols[8],
		SupportedModels: supportedModels,
		Groups:          groups,
	}

	return def
}

func walkRows(url string, handler func(cols []string, tds *goquery.Selection)) {
	var decodedMap = make(map[string][]byte)
	if _, err := os.Stat(cacheOutput); !errors.Is(err, os.ErrNotExist) {
		f, err := os.Open("cache.bin")
		if err != nil {
			log.Fatal("Failed to open file", err)
		}
		err = gob.NewDecoder(f).Decode(&decodedMap)
		if err != nil {
			log.Fatal("Failed to decode map", err)
		}
	}
	var data []byte
	if decodedMap[url] == nil {
		res, err := http.Get(url)
		if err != nil {
			log.Fatal("Failed to connect to the target page", err)
		}
		//goland:noinspection GoUnhandledErrorResult
		defer res.Body.Close()
		if res.StatusCode != 200 {
			log.Printf("HTTP Error %d: %s", res.StatusCode, res.Status)
			decodedMap[url] = []byte("<html></html>")
			f, _ := os.Create(cacheOutput)
			//goland:noinspection GoUnhandledErrorResult
			defer f.Close()
			err = gob.NewEncoder(f).Encode(decodedMap)
			if err != nil {
				log.Fatal("Failed to encode map", err)
			}
			return
		}

		data, err = io.ReadAll(res.Body)
		if err != nil {
			log.Fatal("Failed to read response body", err)
		}
		decodedMap[url] = data

		f, err := os.Create(cacheOutput)
		if err != nil {
			log.Fatal("Failed to connect to the target page", err)
		}
		//goland:noinspection GoUnhandledErrorResult
		defer f.Close()
		err = gob.NewEncoder(f).Encode(decodedMap)
		if err != nil {
			log.Fatal("Failed to encode map", err)
		}
	} else {
		data = decodedMap[url]
	}

	document, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		log.Fatal("Failed to parse the HTML document", err)
	}

	document.Find(".mw-parser-output").First().Find("table").Each(func(i int, it *goquery.Selection) {
		it.Find("tr").Each(func(i int, tr *goquery.Selection) {
			cols := tr.Find("td")
			if cols.Size() == 0 {
				return
			}
			handler(cols.Map(func(_ int, it *goquery.Selection) string {
				return asText(it)
			}), cols)
		})
	})
}

func asText(it *goquery.Selection) string {
	str := strings.Trim(it.Text(), " \n\t")
	if str == "-" || str == "–" {
		return ""
	}
	ws := regexp.MustCompile(`[\s ]+`)
	return ws.ReplaceAllString(str, " ")
}

func asFloat(s string) float64 {
	var i float64
	if _, err := fmt.Sscan(s, &i); err != nil {
		log.Fatalf("Unable to parse string %v as float64", s)
	}
	return i
}

func ignoreError[T any](val T, _ error) T {
	return val
}

func sliceAll[T any](val []T, check func(T) bool) bool {
	for _, v := range val {
		if !check(v) {
			return false
		}
	}
	return true
}

func sliceAny[T any](val []T, check func(T) bool) bool {
	for _, v := range val {
		if check(v) {
			return true
		}
	}
	return false
}

func overlap[T comparable](a1 []T, a2 []T) bool {
	return sliceAny(a1, func(t T) bool { return slices.Contains(a2, t) }) ||
		sliceAny(a2, func(t T) bool { return slices.Contains(a1, t) })
}

func compareDefs(d1 *IOElementDefinition, d2 *IOElementDefinition) (int, bool) {
	p := 0

	//|| d1.Min != d2.Min
	if d1.Max > d2.Max {
		p -= 1
	}

	if d2.Max > d1.Max {
		p += 1
	}

	if d1.Min > d2.Min {
		p += 1
	}

	if d2.Min > d1.Min {
		p -= 1
	}

	if len(d1.Description) > len(d2.Description) {
		p -= 1
	}
	if len(d2.Description) > len(d1.Description) {
		p += 1
	}

	if len(d1.Units) > len(d2.Units) {
		p -= 1
	}
	if len(d2.Units) > len(d1.Units) {
		p += 1
	}

	if !cmp.Equal(d1.Groups, d2.Groups) {
		if sliceAll(d1.Groups, func(s string) bool { return slices.Contains(d2.Groups, s) }) {
			p += 10
		} else if sliceAll(d2.Groups, func(s string) bool { return slices.Contains(d1.Groups, s) }) {
			p -= 10
		} else {
			p += len(d2.Groups) - len(d1.Groups)
			return p, false
		}
	}

	if !cmp.Equal(d1.SupportedModels, d2.SupportedModels) {
		if sliceAll(d1.SupportedModels, func(s string) bool { return slices.Contains(d2.SupportedModels, s) }) {
			p += 5
		} else if sliceAll(d2.SupportedModels, func(s string) bool { return slices.Contains(d1.SupportedModels, s) }) {
			p -= 5
		} else {
			p += len(d2.SupportedModels) - len(d1.SupportedModels)
			return p, false
		}
	}

	if d1.Type != d2.Type || d1.NumBytes != d2.NumBytes || d1.Multiplier != d2.Multiplier {
		return p, false
	}

	return p, true
}
