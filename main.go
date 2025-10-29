package main

import (
  "encoding/xml"
  "encoding/json"
  "fmt"
  "io"
  "os"
  "strings"
)

// TODO: Hardcoded Vanguard here.
// Expects the assession number (without dashes).
const kUrlXmlSubmission = "https://www.sec.gov/Archives/edgar/data/36405/%s/primary_doc.xml"
// From: https://www.sec.gov/search-filings/edgar-application-programming-interfaces
const kUrlJsonSubmissions = "https://data.sec.gov/submissions/CIK0000036405.json"

// Subset of:
// https://www.sec.gov/info/edgar/specifications/form-n-port-xml-tech-specs.htm
type InvstOrSec struct {
  Name string `xml:"name"`
  Cusip string `xml:"cusip"`
  PctVal float32 `xml:"pctVal"`
}

type EdgarSubmission struct {
  XMLName xml.Name `xml:"edgarSubmission"`
  FormData struct {
    GenInfo struct {
      Name string `xml:"seriesName"`
      Id string `xml:"seriesId"`
    }
    InvstOrSecs struct {
      Invst []InvstOrSec `xml:"invstOrSec"`
    } `xml:"invstOrSecs"`
  } `xml:"formData"`
}

func fetchEdgarSubmission(accessionNumber string) error {
  url := fmt.Sprintf(kUrlXmlSubmission, accessionNumber)
  fmt.Printf("About to query %s\n", url)

  ua := os.Getenv("USER_AGENT")
  if ua == "" {
    panic("No \"User-Agent\" in the environment")
  }
  c := NewEdgarClient(ua)
  submission := EdgarSubmission{}
  c.GetXml(url, &submission)
  fmt.Printf("submission: %+v\n", submission)

  pct := float32(0.0)
  for _, invst := range submission.FormData.InvstOrSecs.Invst {
    pct += invst.PctVal
  }

  fmt.Printf("Allocated: %f\n", pct)
  // TODO: This is above 100% right now.
  return nil
}

type JsonSubmission struct {
  Cik                               string `json:"cik"`
  Phone       string `json:"phone"`
  Filings struct {
    Recent struct {
      AccessionNumber       []string    `json:"accessionNumber"`
      FilingDate            []string    `json:"filingDate"`
      //ReportDate            []string    `json:"reportDate"`
      //AcceptanceDateTime    []time.Time `json:"acceptanceDateTime"`
      Act                   []string    `json:"act"`
      Form                  []string    `json:"form"`
      FileNumber            []string    `json:"fileNumber"`
      FilmNumber            []string    `json:"filmNumber"`
      //Items                 []string    `json:"items"`
      CoreType              []string    `json:"core_type"`
      //Size                  []int       `json:"size"`
      //IsXBRL                []int       `json:"isXBRL"`
      //IsInlineXBRL          []int       `json:"isInlineXBRL"`
      PrimaryDocument       []string    `json:"primaryDocument"`
      PrimaryDocDescription []string    `json:"primaryDocDescription"`
    } `json:"recent"`
  } `json:"filings"`
}

func joinAccessionNumbers(an string) string {
  return strings.Join(strings.Split(an, "-"), "")
}

func parseJSONSubmissions(r io.Reader) ([]string, error) {
  decoder := json.NewDecoder(r)
  v := JsonSubmission{}
  err := decoder.Decode(&v)
  if err != nil {
    return []string{}, err
  }

  recent := v.Filings.Recent
  // First we want to find the newest filing date.
  newestFilingDate := "1999-12-31"
  allAccessionNumbers := map[string] []string{}
  for i, filingDate := range recent.FilingDate {
    if recent.Form[i] == "NPORT-P" {
      if strings.Compare(filingDate, newestFilingDate) > 0 {
        newestFilingDate = filingDate
      }
      accessionNumbers, _ := allAccessionNumbers[filingDate]
      accessionNumbers = append(accessionNumbers, joinAccessionNumbers(recent.AccessionNumber[i]))
      allAccessionNumbers[filingDate] = accessionNumbers
    }
  }
  fmt.Printf("newestFilingDate=%s\n", newestFilingDate)
  fmt.Printf("allAccessionNumbers=%+v (allAccessionNumbers[newestFilingDate]=%+v)\n", allAccessionNumbers, allAccessionNumbers[newestFilingDate])
  return allAccessionNumbers[newestFilingDate], nil
}


func main() {
  // TODO: Pull all indexes from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000036405&action=getcompany to filter request (or map them).
  f, err := os.Open("./CIK0000036405.json")
  if err != nil {
    fmt.Printf("Invalid file, err=%+v", err)
    return
  }
  accessionNumbers, err := parseJSONSubmissions(f)
  if err != nil {
    fmt.Printf("Couldn't parse EDGAR submission JSON, err=%+v", err)
    return
  }
  for _, accessionNumber := range accessionNumbers {
    err = fetchEdgarSubmission(accessionNumber)
    if err != nil {
      fmt.Printf("Error fetching/parsing EDGAR XML submission, err=%+v", err)
    }
    break
  }


  // TODO: Use some DB to qualify the stock, something like:
  // https://github.com/JerBouma/FinanceDatabase/tree/main

  /*f, err := os.Open("./primary_doc.xml")
  if err != nil {
    fmt.Printf("Invalid file, err=%+v", err)
    return
  }
  err = parseEdgarXMLSubmission(f)
  if err != nil {
    fmt.Printf("Couldn't parse EDGAR xml, err=%+v", err)
    return
  }*/
}
