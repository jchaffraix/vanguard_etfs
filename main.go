package main

import (
  "encoding/json"
  "encoding/xml"
  "fmt"
  "os"
  "slices"
  "strings"
)

// The first entry is the CIK of the reporting company.
// The second entry is the assession number (without dashes).
const kUrlSingleSubmissionXml = "https://www.sec.gov/Archives/edgar/data/%d/%s/primary_doc.xml"
// From: https://www.sec.gov/search-filings/edgar-application-programming-interfaces
// The first entry is the CIK as an int. The expression will automatically normalize it
// to 10 digits (per EDGAR's format).
const kUrlAllSubmissionsJson = "https://data.sec.gov/submissions/CIK%010d.json"

// Subset of:
// https://www.sec.gov/info/edgar/specifications/form-n-port-xml-tech-specs.htm
type singleSubmission struct {
  XMLName xml.Name `xml:"edgarSubmission"`
  FormData struct {
    GenInfo struct {
      Name string `xml:"seriesName"`
      SeriesId string `xml:"seriesId"`
    } `xml:"genInfo"`
    InvstOrSecs struct {
      InvstOrSec []struct {
        Name string `xml:"name"`
        // The percentages are reported up to E-12 so we shouldn't experience
        // a loss of precision using float32 based on this underflow table:
        // https://docs.oracle.com/cd/E60778_01/html/E60763/z4000ac020351.html
        PctVal float32 `xml:"pctVal"`
        // We don't use `xml:"cusip"` as it is N/A for international stock and `<isin>` contains it.
        Identifiers struct {
          // According to the specification, one of them.
          IsIn struct {
            Value string `xml:"value,attr"`
          } `xml:"isin"`
          Ticker struct {
            Value string `xml:"value,attr"`
          } `xml:"ticker"`
          Other struct {
            OtherDesc string `xml:"otherDesc,attr"`
            Value string `xml:"value,attr"`
          } `xml:"other"`
        } `xml:"identifiers"`
        DerivativeInfo struct {
          FwdDeriv struct {
            DerivCat string `xml:"derivCat,attr"`
          } `xml:"fwdDeriv"`
          FutrDeriv struct {
            DerivCat string `xml:"derivCat,attr"`
          } `xml:"futrDeriv"`
          OptionSwaptionWarrantDeriv struct {
            DerivCat string `xml:"derivCat,attr"`
          } `xml:"optionSwaptionWarrantDeriv"`
          OtherDeriv struct {
            DerivCat string `xml:"derivCat,attr"`
          } `xml:"othDeriv"`
        } `xml:"derivativeInfo"`
      } `xml:"invstOrSec"`
    } `xml:"invstOrSecs"`
  } `xml:"formData"`
}

type IndexComponent struct {
  Name string
  // Only one of the 3 next field may be present.
  // Use IndexComponent.Id() if you want the unique identifier.
  Cusip string
  Ticker string
  OtherId string
  Weight float32
}

const kMissingId = "<no_id>"
func (c IndexComponent) Id() string {
  if c.Cusip != "" {
    return c.Cusip
  }
  if c.Ticker != "" {
    return c.Ticker
  }
  if c.OtherId != "" {
    return c.OtherId
  }
  return kMissingId
}


type Index struct {
  Name string
  SeriesId string
  // Note: The components may add up to more than 100%.
  Components []IndexComponent
}

func populateIndexFromSingleSubmission(submission singleSubmission) Index {
  index := Index{submission.FormData.GenInfo.Name, submission.FormData.GenInfo.SeriesId, []IndexComponent{}}
  for _, component := range submission.FormData.InvstOrSecs.InvstOrSec {
    // Ignore any derivative.
    if component.DerivativeInfo.FutrDeriv.DerivCat != "" {
      continue
    }
    if component.DerivativeInfo.FwdDeriv.DerivCat != "" {
      continue
    }
    if component.DerivativeInfo.OptionSwaptionWarrantDeriv.DerivCat != "" {
      continue
    }
    if component.DerivativeInfo.OtherDeriv.DerivCat != "" {
      continue
    }

    // This should be handled by the derivative checks above, but this is kept to be defensive.
    if component.Identifiers.Other.OtherDesc == "CONTRACT_VANGUARD_ID" {
      continue
    }
    cusip := component.Identifiers.IsIn.Value
    ticker := component.Identifiers.Ticker.Value
    other := component.Identifiers.Other.Value
    index.Components = append(index.Components, IndexComponent{component.Name, cusip, ticker, other, component.PctVal})
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
  return index
}

func fetchSingleSubmission(c EdgarClient, cik int, accessionNumber string) (Index, error) {
  // Note: We convert companyId to int to trim the leading zero that are not needed.
  url := fmt.Sprintf(kUrlSingleSubmissionXml, cik, accessionNumber)
  fmt.Printf("About to query %s\n", url)

  submission := singleSubmission{}
  err := c.GetXml(url, &submission)
  if err != nil {
    return Index{}, nil
  }
  seriesId := submission.FormData.GenInfo.SeriesId
  fmt.Printf("Fetched submission for %s (seriesId=%s, etfName=%s)\n", submission.FormData.GenInfo.Name, seriesId, etfName(cik, seriesId))

  return populateIndexFromSingleSubmission(submission), nil
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

func fetchAllSubmissionsForCik(c EdgarClient, cik int) ([]string, error) {
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
    // TODO: Should we also handle NPORT-EX too?
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

type ValidationResult struct {
  etfName string // empty if unknown (and there will be a warning).
  warnings []string
  errors []string
}

func (r *ValidationResult) addError(err string) {
  r.errors = append(r.errors, err)
}

func (r *ValidationResult) addWarning(err string) {
  r.warnings = append(r.warnings, err)
}

func (r ValidationResult) dump() {
  hasErrors := len(r.errors) != 0
  hasWarnings := len(r.warnings) != 0
  if !hasErrors && !hasWarnings {
    // No issue, nothing to report.
    return
  }
  fmt.Printf("***************** Validation report for %s *****************\n", r.etfName)
  if hasErrors {
    fmt.Printf("Errors:\n")
    for _, err := range r.errors {
      fmt.Printf("  %s\n", err)
    }
    fmt.Printf("\n\n")
  }
  if hasWarnings {
    fmt.Printf("Warnings:\n")
    for _, warning := range r.warnings {
      fmt.Printf("  %s\n", warning)
    }
    fmt.Printf("\n\n")
  }
}

func validateIndex(cik int, index Index) ValidationResult {
  res := ValidationResult{"", []string{}, []string{}}
  if index.Name == "" || index.Name == "N/A" {
    // TODO: Link to index.
    res.addError("Index is missing name")
  }
  if index.SeriesId == "" {
    res.addError(fmt.Sprintf("Index %s is missing seriesName", index.Name))
  }
  res.etfName = etfName(cik, index.SeriesId)
  if res.etfName == "" {
    res.addWarning(fmt.Sprintf("Index %s doesn't have a corresponding ETF in our map", index.Name))
  }

  // If we have errors, it's not worth continuing.
  // Doing so, ensure that we have an ETF name to report.
  if len(res.errors) > 0 || len(res.warnings) > 0 {
    return res
  }

  for _, component := range index.Components {
    componentId := component.Id()
    if component.Name == "N/A" || component.Name == "" {
      res.addError(fmt.Sprintf("ETF %s has a missing component for name=%s, id=%s", res.etfName, component.Name, componentId))
    }

    if componentId == kMissingId {
      res.addError(fmt.Sprintf("ETF %s has a component with no id, name=%s, id=%s", res.etfName, component.Name, componentId))
    }
    if component.Weight < 0 {
      res.addError(fmt.Sprintf("ETF %s has a component with negative weight, name=%s, id=%s", res.etfName, component.Name, componentId))
    }
  }
  return res
}

type IndexId struct {
  Cik int
  SeriesId string
}

func etfName(cik int, seriesId string) string {
  kSeriesToETF := map[IndexId]string{
    // Pulled from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000036405&action=getcompany
    IndexId{36405, "S000002839"}: "VOO",
    IndexId{36405, "S000002840"}: "VTV",
    IndexId{36405, "S000002841"}: "VXF",
    IndexId{36405, "S000002842"}: "VUG",
    IndexId{36405, "S000002843"}: "VV",
    IndexId{36405, "S000002844"}: "VO",
    IndexId{36405, "S000002845"}: "VB",
    IndexId{36405, "S000002846"}: "VBK",
    IndexId{36405, "S000002847"}: "VBR",
    IndexId{36405, "S000002848"}: "VTI",
    IndexId{36405, "S000012756"}: "VOT",
    IndexId{36405, "S000012757"}: "VOE",
    // Pulled from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000736054&action=getcompany
    IndexId{736054, "S000002932"}: "VXUS",
    // Pulled from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000052848&action=getcompany
    IndexId{52848, "S000004441"}: "VAW",
    IndexId{52848, "S000004443"}: "VOX",
    IndexId{52848, "S000004445"}: "VPU",
    IndexId{52848, "S000004446"}: "VCR",
    IndexId{52848, "S000004447"}: "VDC",
    IndexId{52848, "S000004448"}: "VDE",
    IndexId{52848, "S000004449"}: "VFH",
    IndexId{52848, "S000004450"}: "VHT",
    IndexId{52848, "S000004451"}: "VIS",
    IndexId{52848, "S000004452"}: "VGT",
    IndexId{52848, "S000018789"}: "EDV",
    IndexId{52848, "S000019698"}: "MGC",
    IndexId{52848, "S000019699"}: "MGV",
    IndexId{52848, "S000019700"}: "MGK",
    IndexId{52848, "S000063074"}: "VSGX",
    IndexId{52848, "S000063075"}: "ESGV",
    IndexId{52848, "S000069584"}: "VCEB",
    IndexId{52848, "S000094513"}: "VEXC",
  }

  etfName, _ := kSeriesToETF[IndexId{cik, seriesId}]
  return etfName
}

func main() {
  // We store the CIK for the reporting company as int as we need to
  // pad them with 0s in some cases, but not all.
  // If you add a new company's CIK here, make sure to add the new
  // ETFs to etfName or we will ignore them.
  kCompanyIds := []int{52848, 36405, 736054, 36405}
  ua := os.Getenv("USER_AGENT")
  if ua == "" {
    panic("No \"User-Agent\" in the environment")
  }
  c := NewEdgarClient(ua)

  indexMap := map[string]Index{}
  for _, companyId := range kCompanyIds {
    accessionNumbers, err := fetchAllSubmissionsForCik(c, companyId)
    if err != nil {
      fmt.Printf("Error fetching/parsing all submissions JSON, err=%+v\n", err)
      return
    }
    for _, accessionNumber := range accessionNumbers {
      index, err := fetchSingleSubmission(c, companyId, accessionNumber)
      if err != nil {
        fmt.Printf("Error fetching/parsing single XML submission for %s, err=%+v\n", accessionNumber, err)
      }
      res := validateIndex(companyId, index)
      res.dump()
      if res.etfName == "" {
        continue
      }
      indexMap[res.etfName] = index
    }
  }

  // TODO: Do we want to preprocess more of the data (e.g. by standardizing tickers to their name)?
  // This could be done using: https://github.com/JerBouma/FinanceDatabase/tree/main

  for etfName, index := range indexMap {
    f, err := os.OpenFile(fmt.Sprintf("./data/%s.json", etfName), os.O_CREATE | os.O_WRONLY | os.O_TRUNC, 0644)
    if err != nil {
      fmt.Printf("Error: opening file for %s (err=%+v)\n", etfName, err)
      return
    }

    bytes, err := json.Marshal(index)
    if err != nil {
      fmt.Printf("Error: marshaling index for %s (err=%+v, index=%+v)\n", etfName, err, index)
      return
    }

    if _, err := f.Write(bytes); err != nil {
      f.Close() // ignore error; Write error takes precedence
      fmt.Printf("Error: writing to JSON file for %s (err=%+v)\n", etfName, err)
      return
    }
    if err := f.Close(); err != nil {
      fmt.Printf("Error: closing file for %s (err=%+v)\n", etfName, err)
      return
    }
  }
}
