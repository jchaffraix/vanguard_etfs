package main

import (
  "encoding/json"
  "encoding/xml"
  "fmt"
  "os"
  "slices"
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
type SingleSubmission struct {
  XMLName xml.Name `xml:"edgarSubmission"`
  FormData struct {
    GenInfo struct {
      Name string `xml:"seriesName"`
      SeriesId string `xml:"seriesId"`
    } `xml:"genInfo"`
    InvstOrSecs struct {
      InvstOrSec []struct {
        Name string `xml:"name"`
        Cusip string `xml:"cusip"`
        PctVal float32 `xml:"pctVal"`
      } `xml:"invstOrSec"`
    } `xml:"invstOrSecs"`
  } `xml:"formData"`
}

type IndexComponent struct {
  Name string
  Cusip string
  Weight float32
}

type Index struct {
  Name string
  SeriesId string
  // Note: The components may add up to more than 100%.
  Components []IndexComponent
}

func fetchSingleSubmission(c EdgarClient, accessionNumber string) (Index, error) {
  url := fmt.Sprintf(kUrlSingleSubmissionXml, accessionNumber)
  fmt.Printf("About to query %s\n", url)

  submission := SingleSubmission{}
  err := c.GetXml(url, &submission)
  if err != nil {
    return Index{}, nil
  }
  fmt.Printf("Fetched submission for %s (%s)\n", submission.FormData.GenInfo.Name, submission.FormData.GenInfo.SeriesId)
  //fmt.Printf("full submission: %+v\n", submission)

  index := Index{submission.FormData.GenInfo.Name, submission.FormData.GenInfo.SeriesId, []IndexComponent{}}
  for _, component := range submission.FormData.InvstOrSecs.InvstOrSec {
    index.Components = append(index.Components, IndexComponent{component.Name, component.Cusip, component.PctVal})
  }
  // Sort by weight descending, then CUSIP ascending.
  slices.SortFunc(index.Components, func (a, b IndexComponent) int {
    if a.Weight < b.Weight {
      return 1
    }
    if a.Weight > b.Weight {
      return -1
    }
    return strings.Compare(a.Cusip, b.Cusip)
  })
  return index, nil
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
  kSeriesToETF := map[string]string{
    // Pulled from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000036405&action=getcompany.
    "S000002839": "VOO",
    "S000002840": "VTV",
    "S000002841": "VXF",
    "S000002842": "VUG",
    "S000002843": "VV",
    "S000002844": "VO",
    "S000002845": "VB",
    "S000002846": "VBK",
    "S000002847": "VBR",
    "S000002848": "VTI",
    "S000012756": "VOT",
    "S000012757": "VOE",
    // TODO: Pull from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000736054&action=getcompany (for VXUS)
    // TODO: Pull from ??? for ESG funds.
  }

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
  indexMap := map[string]Index{}
  for _, accessionNumber := range accessionNumbers {
    index, err := fetchSingleSubmission(c, accessionNumber)
    if err != nil {
      fmt.Printf("Error fetching/parsing single XML submission for %s, err=%+v", accessionNumber, err)
    }
    etfName, ok := kSeriesToETF[index.SeriesId]
    if !ok {
      fmt.Printf("Error: unknown etf for %s (seriesId=%s)", index.Name, index.SeriesId)
      continue
    }
    indexMap[etfName] = index
  }
  bytes, err := json.Marshal(indexMap)
  fmt.Printf("All indexes JSON: %s", bytes)
  // TODO: Use some DB to qualify the stock, something like:
  // https://github.com/JerBouma/FinanceDatabase/tree/main
}
