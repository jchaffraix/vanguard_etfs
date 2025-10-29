package main

import (
  "encoding/xml"
  "fmt"
  "os"
  "strings"
)

// TODO: Hardcoded Vanguard here.
// Expects the assession number (without dashes).
const kUrlSingleSubmissionXml = "https://www.sec.gov/Archives/edgar/data/36405/%s/primary_doc.xml"
// From: https://www.sec.gov/search-filings/edgar-application-programming-interfaces
// Expected the 10 numbers CIK, including leading zeroes.
const kUrlAllSubmissionsJson = "https://data.sec.gov/submissions/CIK%s.json"

// Subset of:
// https://www.sec.gov/info/edgar/specifications/form-n-port-xml-tech-specs.htm
type InvstOrSec struct {
  Name string `xml:"name"`
  Cusip string `xml:"cusip"`
  PctVal float32 `xml:"pctVal"`
}

type SingleSubmission struct {
  XMLName xml.Name `xml:"edgarSubmission"`
  FormData struct {
    GenInfo struct {
      Name string `xml:"seriesName"`
      Id string `xml:"seriesId"`
    } `xml:"genInfo"`
    InvstOrSecs struct {
      Invst []InvstOrSec `xml:"invstOrSec"`
    } `xml:"invstOrSecs"`
  } `xml:"formData"`
}

func fetchSingleSubmission(c EdgarClient, accessionNumber string) error {
  url := fmt.Sprintf(kUrlSingleSubmissionXml, accessionNumber)
  fmt.Printf("About to query %s\n", url)

  submission := SingleSubmission{}
  c.GetXml(url, &submission)
  fmt.Printf("submission for %s (%s)\n", submission.FormData.GenInfo.Name, submission.FormData.GenInfo.Id)
  //fmt.Printf("submission: %+v\n", submission)

  pct := float32(0.0)
  for _, invst := range submission.FormData.InvstOrSecs.Invst {
    pct += invst.PctVal
  }

  fmt.Printf("Allocated: %f\n", pct)
  // TODO: This is above 100% right now.
  return nil
}

type AllSubmissions struct {
  Cik string `json:"cik"`
  Phone string `json:"phone"`
  Filings struct {
    Recent struct {
      AccessionNumber       []string    `json:"accessionNumber"`
      FilingDate            []string    `json:"filingDate"`
      Form                  []string    `json:"form"`
    } `json:"recent"`
  } `json:"filings"`
}

func joinAccessionNumbers(an string) string {
  return strings.Join(strings.Split(an, "-"), "")
}

func fetchAllSubmissionsForCik(c EdgarClient, cik string) ([]string, error) {
  url := fmt.Sprintf(kUrlAllSubmissionsJson, cik)
  fmt.Printf("About to query %s\n", url)

  v := AllSubmissions{}
  err := c.GetJson(url, &v)
  if err != nil {
    return []string{}, nil
  }
  fmt.Printf("all submissions for %+v\n", v)

  recent := v.Filings.Recent
  // TODO: We should just return all the NPORT-P and build the entire data rather than the newest filings.
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
  ua := os.Getenv("USER_AGENT")
  if ua == "" {
    panic("No \"User-Agent\" in the environment")
  }
  c := NewEdgarClient(ua)

  accessionNumbers, err := fetchAllSubmissionsForCik(c, "0000036405")
  if err != nil {
    fmt.Printf("Error fetching/parsing all submissions JSON, err=%+v", err)
    return
  }

  for _, accessionNumber := range accessionNumbers {
    err = fetchSingleSubmission(c, accessionNumber)
    if err != nil {
      fmt.Printf("Error fetching/parsing single XML submission for %s, err=%+v", accessionNumber, err)
    }
  }

  // TODO: Use some DB to qualify the stock, something like:
  // https://github.com/JerBouma/FinanceDatabase/tree/main
}
