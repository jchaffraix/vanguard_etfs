package main

import (
  "bytes"
  "encoding/json"
  "flag"
  "fmt"
  "io"
  "edgar_client"
  "os"

  "golang.org/x/net/html"
)

var debug bool

type State int
const (
  kOutsideTable State = 0
  kInTableRow State = iota
  kFoundSeries State = iota
  kFoundETF State = iota
  kInETFNameCell State = iota
  kWaitingForNextSeries State = iota
)
func (s State) String() string {
  switch s {
    case kOutsideTable:
      return "kOutsideTable"
    case kInTableRow:
      return "kInTableRow"
    case kFoundSeries:
      return "kFoundSeries"
    case kFoundETF:
      return "kFoundETF"
    case kInETFNameCell:
      return "kInETFNameCell"
    case kWaitingForNextSeries:
      return "kWaitingForNextSeries"
  }
  return ""
}

func transition(state *State, newState State) {
  if debug {
    fmt.Printf("Transitioning from %s to %s\n", state.String(), newState.String())
  }
  *state = newState
}

func processHTML(cik int, r io.Reader) (map[string]string, error) {
  tokenizer := html.NewTokenizer(r)
  state := kOutsideTable
  seriesId := ""
  seriesToEtfMap := map[string]string{}
  for {
    tt := tokenizer.Next()
    switch tt {
      case html.ErrorToken:
        if tokenizer.Err() != io.EOF {
          return map[string]string{}, tokenizer.Err()
        }
        return seriesToEtfMap, nil
      case html.TextToken:
        switch state {
          case kOutsideTable:
            continue
          case kWaitingForNextSeries:
            continue
          case kInTableRow:
            text := tokenizer.Text()
            if debug {
              fmt.Printf("%s, text=\"%s\"\n", state.String(), string(text))
            }
            if len(text) == 10 && text[0] == 'S' {
              seriesId = string(text)
              transition(&state, kFoundSeries)
            }
          case kFoundSeries:
            text := tokenizer.Text()
            if debug {
              fmt.Printf("%s, text=%s\n", state.String(), string(text))
            }
            if bytes.Equal(text, []byte("ETF Shares")) {
              transition(&state, kFoundETF)
            }
          case kFoundETF:
            // Skip text nodes outside the next cell.
            continue
          case kInETFNameCell:
            text := tokenizer.Text()
            if debug {
              fmt.Printf("%s, text=%s\n", state.String(), string(text))
            }
            if bytes.HasPrefix(text, []byte("C000")) {
              panic(fmt.Sprintf("Was about to add a class ID into the map: %s (cik=%010d)", string(text), cik))
            }
            if bytes.HasPrefix(text, []byte("S000")) {
              panic(fmt.Sprintf("Was about to add a series ID into the map: %s (cik=%010d)", string(text), cik))
            }
            seriesToEtfMap[seriesId] = string(text)
            transition(&state, kWaitingForNextSeries)
        }
      case html.StartTagToken:
        name, _ := tokenizer.TagName()
        if debug {
          fmt.Printf("start_tag, %s, name=%s\n", state.String(), string(name))
        }
        if bytes.Equal(name, []byte("tr")) {
          if (state == kOutsideTable) {
            transition(&state, kInTableRow)
          }
        }
        if bytes.Equal(name, []byte("td")) {
          if (state == kFoundETF) {
            transition(&state, kInETFNameCell)
          }
        }
      case html.EndTagToken:
        name, _ := tokenizer.TagName()
        if debug {
          fmt.Printf("end_tag, %s, name=%s\n", state.String(), string(name))
        }
        if bytes.Equal(name, []byte("table")) {
          transition(&state, kOutsideTable)
        }
        if bytes.Equal(name, []byte("tr")) {
          if state == kWaitingForNextSeries {
            transition(&state, kInTableRow)
          }
          if state == kFoundETF {
            // Handle missing cell after the ETF name.
            transition(&state, kInTableRow)
          }
        }
        if state == kInETFNameCell && bytes.Equal(name, []byte("td")) {
          // Handle empty cell, which are missing the ETF name.
          transition(&state, kWaitingForNextSeries)
        }
   }
  }
}

type Index struct {
  SeriesId string `json:"series_id"`
  Name string `json:"name"`
}

func writeToJsonFile(path string, v any) error {
  f, err := os.OpenFile(path, os.O_CREATE | os.O_WRONLY | os.O_TRUNC, 0644)
  if err != nil {
    return err
  }

  bytes, err := json.Marshal(v)
  if err != nil {
    return err
  }

  if _, err := f.Write(bytes); err != nil {
    f.Close() // ignore error; Write error takes precedence
    return err
  }
  if err := f.Close(); err != nil {
    return err
  }
  return nil
}

func main() {
  var outputFileFlag = flag.String("out_file", "", "Path to the output file. If it doesn't exist, it will be created")
  var processFileFlag = flag.String("process_file", "", "Path to an HTML file to process [debugging]")
  flag.BoolVar(&debug, "d", false, "Enable debugging mode (more verbose output)")
  flag.Parse()

  if *processFileFlag != "" {
    f, err := os.Open(*processFileFlag)
    if err != nil {
      panic(fmt.Sprintf("Unable to open debug file, err=%+v", err))
    }
    seriesToEtfMap, err := processHTML(1234, f)
    fmt.Printf("seriesToEtfMap=%+v, err=%+v\n", seriesToEtfMap, err)
    return
  }

  if outputFileFlag == nil || *outputFileFlag == "" {
    flag.Usage()
    return
  }

  ua := os.Getenv("USER_AGENT")
  if ua == "" {
    panic("No \"$USER_AGENT\" in the environment")
  }
  c := edgar_client.NewWithRps(ua, 5)

  // TODO: I could also get this info from https://www.sec.gov/files/company_tickers_mf.json if I had the list of Vanguard ETFs.
  output := map[int][]Index{}
  // This is the main list of ciks that we look at.
  ciks := []int{36405, 52848, 105563, 106830, 736054, 857489, 891190, 1021882}
  for _, cik := range ciks {
    resp, err := c.GetResp(fmt.Sprintf("https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=%010d&action=getcompany", cik))
    if err != nil {
      panic(fmt.Sprintf("Couldn't fetch data from edgar, err=%+v", err))
    }
    defer resp.Body.Close()
    seriesToEtfMap, err := processHTML(cik, resp.Body)
    if err != nil {
      panic(fmt.Sprintf("Error processing html %+v", err))
    }

    etfs := []Index{}
    for seriesId, name := range seriesToEtfMap {
      etfs = append(etfs, Index{seriesId, name})
    }
    output[cik] = etfs
  }
  writeToJsonFile(*outputFileFlag, output)
}
