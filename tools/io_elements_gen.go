package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocarina/gocsv"
	"github.com/google/go-cmp/cmp"
	"github.com/jessevdk/go-flags"
	"golang.org/x/exp/slices"
)

type ElementType uint8
type Preference uint8
type StringSlice []string

const (
	PreferFirst Preference = iota
	PreferSecond
	PreferNone
)

const (
	IOElementSigned ElementType = iota
	IOElementUnsigned
	IOElementHEX
	IOElementASCII
)

type IOElementDefinition struct {
	Id              uint16      `json:"id" csv:"Id"`
	Name            string      `json:"name" csv:"Name"`
	NumBytes        int         `json:"numBytes" csv:"NumBytes"`
	Type            ElementType `json:"type" csv:"Type"`
	Min             float64     `json:"min" csv:"Min"`
	Max             float64     `json:"max" csv:"Max"`
	Multiplier      float64     `json:"multiplier" csv:"Multiplier"`
	Units           string      `json:"units" csv:"Units"`
	Description     string      `json:"description" csv:"Description"`
	SupportedModels StringSlice `json:"supportedModels" csv:"SupportedModels"`
	Groups          StringSlice `json:"groups" csv:"Groups"`
}

type NetworkParserOptions struct {
	Models     []string       `short:"m" long:"model" description:"models to parse, can be specified multiple times" default:"FMB920" default:"FMC650" required:"yes"`
	CsvOutput  flags.Filename `long:"csv-out" description:"csv output file path"`
	Cache      flags.Filename `long:"cache" description:"path to http cache file" default:"./cache.bin"`
	NoCache    bool           `long:"no-cache" description:"disable http cache"`
	NoFollow   bool           `long:"no-follow" description:"disable recursive links following"`
	UrlPattern string         `long:"url-pattern" description:"url generation pattern from specified model names\n substring '{model}' will be substituted\n" default:"https://wiki.teltonika-gps.com/view/{model}_Teltonika_Data_Sending_Parameters_ID"`
}

type CsvParserOptions struct {
	Arg struct {
		InputFile flags.Filename `positional-arg-name:"InputCsvFile"`
	} `positional-args:"yes" required:"yes"`
}

type Options struct {
	ParseNetwork NetworkParserOptions `command:"load-net" optional:"true"`
	ParseCsv     CsvParserOptions     `command:"load-csv" optional:"true"`
	GenOutput    flags.Filename       `short:"o" long:"gen-out" default:"./ioelements_dump.go" description:"output file path for I/O elements definitions list"`
	NoGen        bool                 `long:"no-gen" description:"disable go file generation"`
	GenPkgName   string               `long:"gen-pkg-name" default:"main" description:"package name for generated file"`
	GenInternal  bool                 `long:"gen-internal" description:"generate file for internal usage in ioelements package"`
}

var options Options
var parser = flags.NewParser(&options, flags.Default)

func main() {
	log.SetFlags(0)
	if _, err := parser.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(1)
	}
	switch parser.Command.Active.Name {
	case "load-net":
		res := collectDefinitionsNetwork(options.ParseNetwork.Models)
		if options.ParseNetwork.CsvOutput != "" {
			dumpCsv(res, string(options.ParseNetwork.CsvOutput))
		}
		if !options.NoGen {
			generate(res, string(options.GenOutput))
		}
	case "load-csv":
		res := readCsv(string(options.ParseCsv.Arg.InputFile))
		if !options.NoGen {
			generate(res, string(options.GenOutput))
		}
	}
}

func (r *StringSlice) MarshalCSV() (string, error) {
	return strings.Join(*r, ", "), nil
}

func (r *StringSlice) UnmarshalCSV(csv string) (err error) {
	res := strings.Split(csv, ",")
	for i := range res {
		res[i] = strings.TrimSpace(res[i])
	}
	*r = res
	return nil
}

func (r *ElementType) MarshalCSV() (string, error) {
	switch *r {
	case IOElementASCII:
		return "ASCII", nil
	case IOElementSigned:
		return "Signed", nil
	case IOElementUnsigned:
		return "Unsigned", nil
	case IOElementHEX:
		return "Hex", nil
	}
	return "", fmt.Errorf("unknown element type: %v", *r)
}

func (r *ElementType) UnmarshalCSV(csv string) (err error) {
	switch csv {
	case "ASCII":
		*r = IOElementASCII
	case "Signed":
		*r = IOElementSigned
	case "Unsigned":
		*r = IOElementUnsigned
	case "Hex":
		*r = IOElementHEX
	default:
		return fmt.Errorf("unknown element type: %v", csv)
	}
	return nil
}

func generate(data []*IOElementDefinition, output string) {
	allSupportedModelsMap := map[string]bool{}
	allSupportedModels := make([]string, 0)
	for _, it := range data {
		for _, model := range it.SupportedModels {
			if !allSupportedModelsMap[model] {
				allSupportedModels = append(allSupportedModels, model)
			}
			allSupportedModelsMap[model] = true
		}
	}
	slices.Sort(allSupportedModels)

	wrapStr := func(s interface{}) string {
		str, _ := json.Marshal(s)
		return string(str)
	}

	var sb strings.Builder
	sb.WriteString("package " + options.GenPkgName + "\n\n")
	prefix := ""
	if !options.GenInternal {
		sb.WriteString(`import "github.com/alim-zanibekov/teltonika/ioelements"` + "\n\n")
		prefix = "ioelements."
	}

	sb.WriteString("var supportedModels = map[string]bool{")
	k := 0

	for _, model := range allSupportedModels {
		if k == 0 {
			sb.WriteString(wrapStr(model) + ": true")
		} else {
			sb.WriteString(", " + wrapStr(model) + ": true")
		}
		k++
	}
	sb.WriteString("}\n\n")
	sb.WriteString("var ioElementDefinitions = []" + prefix + "IOElementDefinition{\n")
	var typesMap = map[ElementType]string{
		IOElementUnsigned: "IOElementUnsigned",
		IOElementSigned:   "IOElementSigned",
		IOElementHEX:      "IOElementHEX",
		IOElementASCII:    "IOElementASCII",
	}
	for _, m := range data {
		supportedModels := make([]string, 0)
		for _, it := range m.SupportedModels {
			if it != "" {
				supportedModels = append(supportedModels, wrapStr(it))
			}
		}
		groups := make([]string, 0)
		for _, it := range m.Groups {
			if it != "" {
				groups = append(groups, wrapStr(it))
			}
		}

		line := fmt.Sprintf("\t{%d, %s, %d, %s, %s, %s, %s, %s, %s, []string{%s}, []string{%s}},\n",
			m.Id, wrapStr(m.Name), m.NumBytes, prefix+typesMap[m.Type],
			strconv.FormatFloat(m.Min, 'f', -1, 64),
			strconv.FormatFloat(m.Max, 'f', -1, 64),
			strconv.FormatFloat(m.Multiplier, 'f', -1, 64),
			wrapStr(m.Units), wrapStr(m.Description),
			strings.Join(supportedModels, ", "), strings.Join(groups, ", "),
		)

		sb.WriteString(line)
	}
	sb.WriteString("}\n")

	f, err := os.Create(output)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString(sb.String())
	if err != nil {
		log.Fatalf("error writing to %s: %v", options.GenOutput, err)
	}
}

func dumpCsv(data []*IOElementDefinition, path string) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()
	err = gocsv.Marshal(data, f)
	if err != nil {
		log.Fatalf("error marshalling csv: %v", err)
	}
}

func readCsv(path string) []*IOElementDefinition {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()
	var res []*IOElementDefinition
	err = gocsv.Unmarshal(f, &res)
	if err != nil {
		log.Fatalf("error marshalling csv: %v", err)
	}
	return res
}

func collectDefinitionsNetwork(modelNames []string) []*IOElementDefinition {
	result := make([]*IOElementDefinition, 0)

	mergeSupportedModels := func(a *IOElementDefinition, b *IOElementDefinition) {
		res := unique(append(a.SupportedModels, b.SupportedModels...))
		sort.Strings(res)
		a.SupportedModels = res
		b.SupportedModels = res
	}

	loadPagesRecursively(modelNames, func(current *IOElementDefinition) {
		toReplace := make([]int, 0)
		for i, definition := range result {
			if definition.Id != current.Id {
				continue
			}
			if isDefinitionsEqual(definition, current) {
				return
			}

			if isDefinitionsEqualExceptSupportedModels(definition, current) {
				mergeSupportedModels(definition, current)
				return
			}

			if !overlap(definition.SupportedModels, current.SupportedModels) {
				continue
			}

			if containsAll(definition.SupportedModels, current.SupportedModels) {
				return
			}

			if containsAll(current.SupportedModels, definition.SupportedModels) {
				toReplace = append(toReplace, i)
				continue
			}

			almostEqual, preference := isDefinitionsEqualExceptMinMaxMultiplier(definition, current)
			if almostEqual && preference != PreferNone {
				if preference == PreferSecond {
					toReplace = append(toReplace, i)
					mergeSupportedModels(definition, current)
					continue
				} else {
					mergeSupportedModels(definition, current)
					return
				}
			}

			almostEqual, preference = preferOneThatHasMultiplierAndMaxAndMinAndDescriptionAndUnits(definition, current)
			if almostEqual && preference != PreferNone {
				if preference == PreferSecond {
					toReplace = append(toReplace, i)
					mergeSupportedModels(definition, current)
					continue
				} else {
					mergeSupportedModels(definition, current)
					return
				}
			}

			// normalization
			// 32512 - https://wiki.teltonika-gps.com/view/FMB150_Teltonika_Data_Sending_Parameters_ID
			// 16128 - https://wiki.teltonika-gps.com/view/FMB920_Teltonika_Data_Sending_Parameters_ID
			if current.Id == 90 && current.Max == 32512 && definition.Max == 16128 {
				toReplace = append(toReplace, i)
				continue
			}
			if definition.Id == 90 && definition.Max == 32512 && current.Max == 16128 {
				return
			}

			log.Printf("\n1. %s\n2. %s\n%v", ignoreError(json.Marshal(definition)), ignoreError(json.Marshal(current)), almostEqual)
			return
		}

		if len(toReplace) > 0 {
			for _, i := range toReplace {
				result = append(result[:i], result[i+1:]...)
			}
		}
		result = append(result, current)
	})

	slices.SortStableFunc(result, func(a, b *IOElementDefinition) int {
		return int(a.Id) - int(b.Id)
	})

	return result
}

func isDefinitionsEqualExceptSupportedModels(a *IOElementDefinition, b *IOElementDefinition) bool {
	scalarFieldsEqual := a.Type == b.Type &&
		a.NumBytes == b.NumBytes &&
		a.Multiplier == b.Multiplier &&
		a.Min == b.Min &&
		a.Max == b.Max &&
		a.Units == b.Units

	return scalarFieldsEqual
}

func isDefinitionsEqual(a *IOElementDefinition, b *IOElementDefinition) bool {
	return isDefinitionsEqualExceptSupportedModels(a, b) && cmp.Equal(a.SupportedModels, b.SupportedModels)
}

func preferOneThatHasMultiplierAndMaxAndMinAndDescriptionAndUnits(
	a *IOElementDefinition,
	b *IOElementDefinition,
) (bool, Preference) {
	mainFieldsEqual := a.Type == b.Type &&
		a.NumBytes == b.NumBytes

	if !mainFieldsEqual {
		return false, PreferNone
	}

	if a.Max != 0 && a.Units != "" && a.Description != "" &&
		b.Max == 0 && b.Units == "" && b.Description == "" {
		return true, PreferFirst
	}

	if b.Max != 0 && b.Units != "" && b.Description != "" &&
		a.Max == 0 && a.Units == "" && a.Description == "" {
		return true, PreferSecond
	}

	return false, PreferNone
}

func isDefinitionsEqualExceptMinMaxMultiplier(
	a *IOElementDefinition,
	b *IOElementDefinition,
) (bool, Preference) {
	scalarFieldsEqual := a.Type == b.Type &&
		a.NumBytes == b.NumBytes &&
		a.Units == b.Units

	if scalarFieldsEqual && cmp.Equal(a.SupportedModels, b.SupportedModels) {
		aCost := 0
		bCost := 0
		if a.Multiplier != b.Multiplier {
			if a.Multiplier != 1.0 && b.Multiplier != 1.0 {
				return true, PreferNone
			}
			if a.Multiplier != 1.0 {
				aCost++
			} else {
				bCost++
			}
		}

		if a.Min != b.Min {
			if a.Min != 0.0 && b.Min != 0.0 {
				return true, PreferNone
			}
			if a.Min != 0.0 {
				aCost++
			} else {
				bCost++
			}
		}

		if a.Max != b.Max {
			if a.Min != 0.0 && b.Max != 0.0 {
				return true, PreferNone
			}
			if a.Max != 0.0 {
				aCost++
			} else {
				bCost++
			}
		}

		if aCost == bCost {
			return true, PreferNone
		}
		if aCost > bCost {
			return true, PreferFirst
		}
		return true, PreferSecond
	}

	return false, PreferNone
}

func modelNameToUrl(modelName string) string {
	res := strings.ReplaceAll(options.ParseNetwork.UrlPattern, "{model}", modelName)

	if modelName == "TMT250" || modelName == "GH5200" || modelName == "TFT100" || modelName == "TST100" ||
		modelName == "TAT100" || modelName == "TAT140" || modelName == "TAT141" || modelName == "TAT240" {
		res = strings.ReplaceAll(res, "_Teltonika_Data_Sending_Parameters_ID", "_AVL_ID_List")
	}

	return res
}

// Load pages recursively by parsing "HW Support" column and adding links to another models to the stack
func loadPagesRecursively(modelNames []string, onDefinition func(it *IOElementDefinition)) {
	urlsToProcess := make([]string, 0)
	for _, it := range modelNames {
		urlsToProcess = append(urlsToProcess, modelNameToUrl(it))
	}

	for i := 0; i < len(urlsToProcess); i++ {
		log.Printf("Parsing %s, %d pages remaining", urlsToProcess[i], len(urlsToProcess)-1-i)

		loadPage(urlsToProcess[i], func(page string) {
			modelName := ""
			processPageTable(page, func(document *goquery.Document, tds *goquery.Selection) {
				if !options.ParseNetwork.NoFollow {
					otherPages := tds.Eq(9).Find("a").Map(func(_ int, it *goquery.Selection) string {
						link := it.AttrOr("href", "")
						if link == "" {
							return ""
						}
						pUrl, err := url.Parse(link)
						if err != nil {
							log.Printf("Error parsing link %s: %v", link, err)
							return ""
						}
						title := pUrl.Query().Get("title")
						if pUrl.Path == "/index.php" {
							if title != "" {
								return modelNameToUrl(title)
							}
							return ""
						}
						p := strings.Split(pUrl.Path, "/")
						return modelNameToUrl(p[len(p)-1])
					})

					for _, link := range otherPages {
						if link != "" && !slices.Contains(urlsToProcess, link) {
							urlsToProcess = append(urlsToProcess, link)
						}
					}
				}

				if modelName == "" {
					modelName = strings.Split(document.Find("#firstHeading").First().Text(), " ")[0]
				}

				items := parseTableRow(modelName, tds)
				if items == nil {
					return
				}

				for _, item := range items {
					onDefinition(item)
				}
			})
		})
	}
}

func loadPage(url string, onPage func(data string)) {
	var decodedMap = make(map[string][]byte)
	if !options.ParseNetwork.NoCache {
		if _, err := os.Stat(string(options.ParseNetwork.Cache)); !errors.Is(err, os.ErrNotExist) {
			f, err := os.Open(string(options.ParseNetwork.Cache))
			if err != nil {
				log.Fatal("Failed to open cache file", err)
			}
			if err = gob.NewDecoder(f).Decode(&decodedMap); err != nil {
				log.Fatal("Failed to decode cache map", err)
			}
		}
	}
	if decodedMap[url] == nil {
		res, err := http.Get(url)
		if err != nil {
			log.Printf("Failed to connect to the target page %v", err)
			return
		}

		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != 200 {
			log.Printf("HTTP Error %d: %s", res.StatusCode, res.Status)
			decodedMap[url] = []byte("<html></html>")
		} else {
			var data []byte
			if data, err = io.ReadAll(res.Body); err != nil {
				log.Printf("Failed to read response body %v", err)
				return
			}
			decodedMap[url] = data
		}
		if !options.ParseNetwork.NoCache {
			f, err := os.Create(string(options.ParseNetwork.Cache))
			defer func() { _ = f.Close() }()

			if err = gob.NewEncoder(f).Encode(decodedMap); err != nil {
				log.Fatal("Failed to encode cache map", err)
			}
		}
	}
	onPage(string(decodedMap[url]))
}

func processPageTable(pageContents string, onRow func(document *goquery.Document, tds *goquery.Selection)) {
	document, err := goquery.NewDocumentFromReader(bytes.NewBufferString(pageContents))
	if err != nil {
		log.Printf("Failed to parse the HTML document %v", err)
	}

	tables := document.Find(".mw-parser-output").First().Find("table")

	tables.Each(func(_ int, it *goquery.Selection) {
		it.Find("tr").Each(func(_ int, tr *goquery.Selection) {
			cols := tr.Find("td")
			if cols.Size() == 0 {
				return
			}
			onRow(document, cols)
		})
	})
}

var rowTypesMap = map[string]ElementType{
	"unsigned":          IOElementUnsigned,
	"signed":            IOElementSigned,
	"hex":               IOElementHEX,
	"ascii":             IOElementASCII,
	"unsigned long int": IOElementUnsigned,
	"unsiged":           IOElementUnsigned,
	"<string>":          IOElementASCII,
}

func parseTableRow(modelName string, tds *goquery.Selection) []*IOElementDefinition {
	c := tds.Map(func(_ int, it *goquery.Selection) string { return asText(it) })

	rawId, rawName, rawNumBytes, rawType, rawMin, rawMax, rawMultiplier, rawUnits, rawDescription :=
		c[0], c[1], c[2], c[3], c[4], c[5], c[6], c[7], c[8]

	var numBytes int
	var min, max, multiplier float64
	var elementType ElementType

	if rawNumBytes == "" && rawType == "<string>" {
		numBytes = -1
	} else if _, err := fmt.Sscan(rawNumBytes, &numBytes); err != nil {
		numBytes = -1
		if !strings.Contains(strings.ToLower(rawNumBytes), "variable") {
			log.Printf("[%s] invalid num bytes - %s", rawId, rawNumBytes)
		}
	}

	var ok bool
	if elementType, ok = rowTypesMap[strings.ToLower(rawType)]; !ok {
		log.Printf("[%s] invalid type '%s', set to unsigned, numBytes set to -1", rawId, rawType)
		elementType = IOElementUnsigned
		numBytes = -1
	}

	if rawMin != "" && rawMax != "" && !strings.HasPrefix(rawMax, "0x") && !strings.HasPrefix(rawMin, "0x") {
		if _, err := fmt.Sscan(rawMin, &min); err != nil {
			log.Printf("[%s] Unable to parse string (min) '%s' as float64, skipping", rawId, rawMin)
			return nil
		}
		if _, err := fmt.Sscan(rawMax, &max); err != nil {
			log.Printf("[%s] Unable to parse string (max) '%s' as float64, skipping", rawId, rawMax)
			return nil
		}
	}
	multiplier = 1
	if rawMultiplier != "" {
		if rawMultiplier == "acc and braking: 0.01" {
			multiplier = 1.0 // we need another io element to make decision, just skip
		} else if _, err := fmt.Sscan(rawMultiplier, &multiplier); err != nil {
			log.Printf("[%s] Unable to parse string (multiplier) '%s' as float64", rawId, rawMultiplier)
			return nil
		}
	}

	supportedModels := make([]string, 1)
	supportedModels[0] = modelName
	tds.Eq(9).Find("a").Each(func(_ int, it *goquery.Selection) {
		if asText(it) != "" {
			supportedModels = append(supportedModels, asText(it))
		}
	})
	supportedModels = unique(supportedModels)

	groups := make([]string, 0)
	if len(c) >= 11 {
		for _, it := range strings.Split(c[10], ",") {
			groups = append(groups, strings.Trim(it, " \n\t"))
		}
	}
	groups = unique(groups)

	if len(groups) == 0 {
		log.Printf("[%s] No groups found, keeping", rawId)
	}

	if elementType != IOElementHEX && elementType != IOElementASCII &&
		(numBytes == -1 || numBytes > 8) {
		elementType = IOElementHEX
	}

	name := rawName
	units := rawUnits
	description := rawDescription

	slices.Sort(supportedModels)
	slices.Sort(groups)

	def := &IOElementDefinition{
		Id:              0,
		Name:            name,
		NumBytes:        numBytes,
		Type:            elementType,
		Min:             min,
		Max:             max,
		Multiplier:      multiplier,
		Units:           units,
		Description:     description,
		SupportedModels: supportedModels,
		Groups:          groups,
	}

	if strings.HasPrefix(rawId, "COM1") {
		ids := strings.Split(strings.ReplaceAll(rawId, "COM1", ""), "COM2")
		id1Str := strings.Trim(strings.ReplaceAll(ids[0], "-", ""), " \n\t")
		id2Str := strings.Trim(strings.ReplaceAll(ids[1], "-", ""), " \n\t")
		id1, err := strconv.Atoi(id1Str)
		if err != nil {
			log.Printf("[%s] unable to parse id '%s'", rawId, id1Str)
		}
		id2, err := strconv.Atoi(id2Str)
		if err != nil {
			log.Printf("[%s] unable to parse id '%s'", rawId, id2Str)
		}
		r1 := *def
		r1.Id = uint16(id1)
		r1.Name = "COM1 " + r1.Name
		r2 := *def
		r2.Id = uint16(id2)
		r2.Name = "COM2 " + r2.Name

		normalize(&r1)
		normalize(&r2)

		return []*IOElementDefinition{&r1, &r2}
	} else {
		id, err := strconv.Atoi(rawId)
		if err != nil {
			log.Printf("[%s] unable to parse id '%s'", rawId, rawId)
		}
		def.Id = uint16(id)

		normalize(def)
		return []*IOElementDefinition{def}
	}
}

func normalize(it *IOElementDefinition) {
	if (it.Id == 9 || it.Id == 6) && it.Units == "mV" {
		it.Units = "V"
	}
	if it.Id == 89 && it.Max == 65535 {
		it.Max = 100
	}
	if it.Id == 30 && it.Type == IOElementASCII {
		it.Type = IOElementUnsigned
		// I am not sure about this patch
		// uint8 https://wiki.teltonika-gps.com/view/FMB920_Teltonika_Data_Sending_Parameters_ID
		// ASCII https://wiki.teltonika-gps.com/view/FMM250_Teltonika_Data_Sending_Parameters_ID
	}
	if it.Id == 168 && it.Max == 6553 {
		it.Max = 65535
	}
	if it.Id == 103 && it.Max == 1677215 {
		it.Max = 16777215
	}
	if (it.Id == 278 || it.Id == 275 || it.Id == 272 || it.Id == 269) && it.NumBytes == 2 {
		it.NumBytes = 1
	}
	if sliceAny([]int{
		478, 477, 476, 475, 474, 473, 472, 471, 470, 469, 468, 467, 466, 465, 464, 463,
	}, func(i int) bool { return i == int(it.Id) }) && it.NumBytes == 8 {
		it.NumBytes = 4
	}
	if sliceAny([]int{
		10812, 10813, 10814, 10815,
	}, func(i int) bool { return i == int(it.Id) }) && it.Max == 1 {
		it.Max = 0
	}
	if sliceAny([]int{
		10824, 10825, 10826, 10827, 10836, 10837, 10838, 10839,
	}, func(i int) bool { return i == int(it.Id) }) && it.Max == 65535 {
		it.Max = 0
	}
	if sliceAny([]int{
		10839, 10838, 10837, 10836, 10839, 10838, 10837, 10836,
	}, func(i int) bool { return i == int(it.Id) }) && it.Max == 0 {
		it.Max = 32767
	}
	if sliceAny([]int{
		463, 467, 471, 475,
	}, func(i int) bool { return i == int(it.Id) }) && it.Max == 4294967295 && it.NumBytes == 256 {
		it.Max = 0
		it.NumBytes = 2
	}
	if it.Units == "-ml" {
		it.Units = "ml"
	}
	if it.Units == "enum (bit field)" || it.Units == "String" {
		it.Units = ""
	}
	if strings.ToLower(it.Units) == "minutes" {
		it.Units = "min."
	}
}

// Utils

func asText(it *goquery.Selection) string {
	str := strings.Trim(it.Text(), " \n\t")
	if str == "-" || str == "–" {
		return ""
	}
	ws := regexp.MustCompile(`[ \t  ]+`)
	res := strings.Trim(ws.ReplaceAllString(str, " "), " \n\t")

	if len(res) > 2 && strings.Count(res, `"`) == 2 && string(res[0]) == `"` && string(res[len(res)-1]) == `"` {
		return res[1 : len(res)-2]
	}
	return res
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

func containsAll[T comparable](a1 []T, a2 []T) bool {
	return sliceAll(a2, func(t T) bool { return slices.Contains(a1, t) })
}

func unique[T comparable](inputSlice []T) []T {
	uniqueSlice := make([]T, 0, len(inputSlice))
	seen := make(map[T]bool, len(inputSlice))
	for _, element := range inputSlice {
		if !seen[element] {
			uniqueSlice = append(uniqueSlice, element)
			seen[element] = true
		}
	}
	return uniqueSlice
}
