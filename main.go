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

// The throttling in EdgarClient is not enough, we can only fetch a small amount
// of securities or else the IP is banned for 10 mins.
// This limit is somewhat conservative.
const kMaxSubmissionsToFetch = 50

const kFetchedMapFile = "./data/fetched_map.json"

// Subset of:
// https://www.sec.gov/info/edgar/specifications/form-n-port-xml-tech-specs.htm
type invstOrSec struct {
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
    SwapDeriv struct {
      DerivCat string `xml:"derivCat,attr"`
    } `xml:"swapDeriv"`
    OptionSwaptionWarrantDeriv struct {
      DerivCat string `xml:"derivCat,attr"`
    } `xml:"optionSwaptionWarrantDeriv"`
    OtherDeriv struct {
      DerivCat string `xml:"derivCat,attr"`
    } `xml:"othDeriv"`
  } `xml:"derivativeInfo"`
}

type singleSubmission struct {
  XMLName xml.Name `xml:"edgarSubmission"`
  FormData struct {
    GenInfo struct {
      Name string `xml:"seriesName"`
      SeriesId string `xml:"seriesId"`
    } `xml:"genInfo"`
    InvstOrSecs struct {
      InvstOrSec []invstOrSec  `xml:"invstOrSec"`
    } `xml:"invstOrSecs"`
  } `xml:"formData"`
}

type IndexComponent struct {
  Name string `json:"name"`
  Id string `json:"id"`
  IdType string `json:"id_type"`
  Weight float32 `json:"weight"`
}

type Index struct {
  Name string `json:"name"`
  SeriesId string `json:"series_id"`
  FilingDate string `json:"filing_date"`
  // Note: The components may add up to more than 100%.
  Components []IndexComponent
}

func getIdentifier(c invstOrSec) (string, string) {
  isin := c.Identifiers.IsIn.Value
  if isin != "" {
    return isin, "isin"
  }
  ticker := c.Identifiers.Ticker.Value
  if ticker != "" {
    return ticker, "ticker"
  }

  id := c.Identifiers.Other.Value
  if id == "" {
    panic(fmt.Sprintf("No identifier found for %+v", c))
  }

  idType := c.Identifiers.Other.OtherDesc
  return id, strings.ToLower(idType)
}

func populateIndexFromSingleSubmission(submission singleSubmission, info SubmissionInfo) Index {
  index := Index{submission.FormData.GenInfo.Name, submission.FormData.GenInfo.SeriesId, info.FilingDate, []IndexComponent{}}
  for _, component := range submission.FormData.InvstOrSecs.InvstOrSec {
    // Ignore any derivative.
    if component.DerivativeInfo.FutrDeriv.DerivCat != "" {
      continue
    }
    if component.DerivativeInfo.FwdDeriv.DerivCat != "" {
      continue
    }
    if component.DerivativeInfo.SwapDeriv.DerivCat != "" {
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
    id, idType := getIdentifier(component)
    index.Components = append(index.Components, IndexComponent{component.Name, id, idType, component.PctVal})
  }
  // Sort by weight descending, then Id ascending.
  slices.SortFunc(index.Components, func (a, b IndexComponent) int {
    if a.Weight < b.Weight {
      return 1
    }
    if a.Weight > b.Weight {
      return -1
    }
    return strings.Compare(a.Id, b.Id)
  })
  return index
}

func fetchSingleSubmission(c EdgarClient, info SubmissionInfo) (Index, error) {
  // Note: We convert companyId to int to trim the leading zero that are not needed.
  url := fmt.Sprintf(kUrlSingleSubmissionXml, info.Cik, info.AccessionNumber)
  fmt.Printf("About to query %s\n", url)

  submission := singleSubmission{}
  err := c.GetXml(url, &submission)
  if err != nil {
    return Index{}, nil
  }
  seriesId := submission.FormData.GenInfo.SeriesId
  fmt.Printf("Fetched submission for %s (seriesId=%s, etfName=%s)\n", submission.FormData.GenInfo.Name, seriesId, etfName(info.Cik, seriesId))

  return populateIndexFromSingleSubmission(submission, info), nil
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

type SubmissionInfo struct {
  Cik int
  AccessionNumber string
  FilingDate string
}

func fetchAllSubmissions(c EdgarClient, cik int) ([]SubmissionInfo, error) {
  url := fmt.Sprintf(kUrlAllSubmissionsJson, cik)
  fmt.Printf("About to query %s\n", url)

  v := AllSubmissions{}
  err := c.GetJson(url, &v)
  if err != nil {
    return []SubmissionInfo{}, nil
  }
  // TODO: Add some debugging mode as this is verbose: fmt.Printf("all submissions for %+v\n", v)

  recent := v.Filings.Recent
  submissionInfos := []SubmissionInfo{}
  for i, filingDate := range recent.FilingDate {
    // TODO: Should we also handle NPORT-EX too?
    if recent.Form[i] == "NPORT-P" {
      submissionInfos = append(submissionInfos, SubmissionInfo{cik, joinAccessionNumbers(recent.AccessionNumber[i]), filingDate})
    }
  }
  // Sort submissions from newest to oldest.
  slices.SortFunc(submissionInfos, func (a, b SubmissionInfo) int {
    // a and b are flipped to get the newest to oldest behavior.
    return strings.Compare(b.FilingDate, a.FilingDate)
  })
  return submissionInfos, nil
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
    if component.Name == "N/A" || component.Name == "" {
      res.addError(fmt.Sprintf("ETF %s has a component with no name=%s, id=%s", res.etfName, component.Name, component.Id))
    }

    if component.Id == "N/A" || component.Id == "" {
      res.addError(fmt.Sprintf("ETF %s has a component with no id, name=%s, id=%s", res.etfName, component.Name, component.Id))
    }

    if component.IdType == "N/A" || component.IdType == "" {
      res.addError(fmt.Sprintf("ETF %s has a component with no idType name=%s, id=%s, id_type=%s", res.etfName, component.Name, component.Id, component.IdType))
    } else {
      // Validate that we know the type. This is only a warning as it's mostly a signal for the users.
      knownTypes := map[string]bool {
        "isin": true,
        "ticker": true,
        "sedol": true,
        "faid": true,
        "cins": true,
        "cusip": true,
        "vid": true,
      }
      known := knownTypes[component.IdType]
      if !known {
        res.addWarning(fmt.Sprintf("ETF %s has a component with an unknown idType name=%s, id=%s, id_type=%s", res.etfName, component.Name, component.Id, component.IdType))
      }
    }

    if component.Weight < 0 {
      res.addError(fmt.Sprintf("ETF %s has a component with negative weight, name=%s, id=%s", res.etfName, component.Name, component.Id))
    }
  }
  return res
}

type IndexId struct {
  Cik int
  SeriesId string
}

func etfs(cik int) []string {
  kCikToETFs := map[int][]string{
    // Pulled from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000036405&action=getcompany
    36405: []string{"VOO", "VTV", "VXF", "VUG", "VV", "VO", "VB", "VBK", "VBR", "VTI", "VOT", "VOE"},
    // Pulled from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000736054&action=getcompany
    736054: []string{"VXUS"},
    // Pulled from https://www.sec.gov/cgi-bin/browse-edgar?scd=series&CIK=0000052848&action=getcompany
    52848: []string{"VAW", "VOX", "VPU", "VCR", "VDC", "VDE", "VFH", "VHT", "VIS", "VGT", "EDV", "MGC", "MGV", "MGK", "VSGX", "ESGV", "VCEB", "VEXC"},
  }
  return kCikToETFs[cik]
}

func etfName(cik int, seriesId string) string {
  // Note: The full list of mutual fund is listed at: https://www.sec.gov/files/company_tickers_mf.json
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

func readFetchedDate() FetchedDatesMap {
  f, err := os.Open(kFetchedMapFile)
  if err != nil {
    panic(fmt.Sprintf("Couldn't open date file, err=%+v", err))
  }

  v := FetchedDatesMap{}
  decoder := json.NewDecoder(f)
  err = decoder.Decode(&v)
  if err != nil {
    panic(fmt.Sprintf("Couldn't JSON decode the fetched date file, err=%+v", err))
  }
  return v
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

type FetchedDatesMap map[int] FilingDateSpan
type FilingDateSpan struct {
  // Both ends are inclusive so this represents the span: [Start, End]
  // If both are "", it's the empty span.
  Start string `json:"start"`
  End string `json:"end"`
}

func (f FilingDateSpan) isEmpty() bool {
  return f.Start == "" || f.End == ""
}

func (f FilingDateSpan) spans(date string) bool {
  return strings.Compare(date, f.Start) >= 0 && strings.Compare(date, f.End) <= 0
}

func (f *FilingDateSpan) update(start, end string) {
  if strings.Compare(start, end) > 0 {
    tmp := end
    end = start
    start = tmp
  }

  if f.Start == "" || strings.Compare(start, f.Start) < 0 {
    f.Start = start
  }
  if f.End == "" || strings.Compare(end, f.End) > 0 {
    f.End = end
  }
}

func filterFilingDates(infos []SubmissionInfo, fetchedDates FilingDateSpan) []SubmissionInfo {
  if fetchedDates.isEmpty() {
    return infos
  }

  res := []SubmissionInfo{}
  for _, info := range infos {
    if fetchedDates.spans(info.FilingDate) {
      continue
    }
    res = append(res, info)
  }
  return res
}

func buildIndexMap(cik int, fetchedDates FilingDateSpan) map[string][]Index {
  indexMap := map[string][]Index{}
  // Ignore existing files if we don't have any fetched information.
  if fetchedDates.isEmpty() {
    return indexMap
  }
  etfs := etfs(cik)
  for _, etf := range etfs {
    allFilePath := fmt.Sprintf("./data/all/%s.json", etf)
    f, err := os.Open(allFilePath)
    if err != nil {
      if etf == "VEXC" {
        // This is a new index as of 2025-09-01 so ignore a missing file.
        // TODO: Remove this check in 2026.
        continue
      }
      panic(fmt.Sprintf("Error opening file for %s, err=%+v", etf, err))
    }

    v := []Index{}
    decoder := json.NewDecoder(f)
    err = decoder.Decode(&v)
    if err != nil {
      panic(fmt.Sprintf("Couldn't JSON decode the file for %s, err=%+v", etf, err))
    }
  }

  return indexMap
}

func main() {
  // Ensure that the output directories are present before fetching.
  if err := os.MkdirAll("data/latest", 0755); err != nil {
    panic("Couldn't create directory data/latest")
  }
  if err := os.MkdirAll("data/all", 0755); err != nil {
    panic("Couldn't create directory data/latest")
  }
  fetchedDateMap := readFetchedDate()
  fmt.Printf("FetchedMap: %+v\n", fetchedDateMap)

  ua := os.Getenv("USER_AGENT")
  if ua == "" {
    panic("No \"$USER_AGENT\" in the environment")
  }
  c := NewEdgarClientWithRps(ua, 5)

  // We store the CIK for the reporting company as int as we need to
  // pad them with 0s in some cases, but not all.
  // If you add a new company's CIK here, make sure to add the new
  // ETFs to() etfName or we will ignore them.
  kCompanyIds := []int{52848, 36405, 736054, 36405}
  for _, companyId := range kCompanyIds {
    fetchedDates := fetchedDateMap[companyId]
    indexMap := buildIndexMap(companyId, fetchedDates)
    // Vanguard has a lot of submissions, unfortunately we don't know which ones are useful
    // before fetching them as we don't know if the submissions have an associated ETF...
    //
    // Fetching all the potential submissions is prohibitive so we have a hard limit.
    // Ideally we should replace with something better, like a per-seriesId search.
    submissions, err := fetchAllSubmissions(c, companyId)
    if err != nil {
      fmt.Printf("Error fetching/parsing all submissions JSON, err=%+v\n", err)
      return
    }
    submissions = filterFilingDates(submissions, fetchedDates)
    if len(submissions) > kMaxSubmissionsToFetch {
      fmt.Printf("Too many submissions to fetch: %d (limit %d). Finding a suitable boundary.\n", len(submissions), kMaxSubmissionsToFetch)
      maxSubmissionIdx := -1
      for i := 1; i <= kMaxSubmissionsToFetch; i++ {
        if submissions[i - 1].FilingDate != submissions[i].FilingDate {
          maxSubmissionIdx = i
        }
      }
      if maxSubmissionIdx == -1 {
        panic("No filingDate boundary found in data")
      }
      submissions = submissions[0:maxSubmissionIdx]
      fmt.Printf("Will fetch: %d (limit %d), filingDate in [%s,%s].\n", len(submissions), kMaxSubmissionsToFetch, submissions[0].FilingDate, submissions[len(submissions) - 1].FilingDate)
    }
    // TODO: Add a debugging mode as this is verbose: fmt.Printf("submissions to fetch = %+v", submissions)

    for _, submission := range submissions {
      index, err := fetchSingleSubmission(c, submission)
      if err != nil {
        fmt.Printf("Error fetching/parsing single XML submission for %+v, err=%+v\n", submission, err)
      }
      res := validateIndex(companyId, index)
      res.dump()
      if res.etfName == "" {
        continue
      }
      existingIndexes := indexMap[res.etfName]
      existingIndexes = append(existingIndexes, index)
      indexMap[res.etfName] = existingIndexes
    }

    // Sort the indexes from newest to oldest.
    for etfName, indexes := range indexMap {
      slices.SortFunc(indexes, func (a, b Index) int {
        // a and b are flipped to get the newest to oldest behavior.
        return strings.Compare(b.FilingDate, a.FilingDate)
      })
      indexMap[etfName] = indexes
    }

    // TODO: Do we want to preprocess more of the data (e.g. by standardizing tickers to their name)?
    // This could be done using: https://github.com/JerBouma/FinanceDatabase/tree/main

    for etfName, indexes := range indexMap {
      allFilePath := fmt.Sprintf("./data/all/%s.json", etfName)
      if err := writeToJsonFile(allFilePath, indexes); err != nil {
        fmt.Printf("Error: writing to file %s (err=%+v)\n", allFilePath, err)
        return
      }
      latestFilePath := fmt.Sprintf("./data/latest/%s.json", etfName)
      if err := writeToJsonFile(latestFilePath, indexes[0]); err != nil {
        fmt.Printf("Error: writing to file %s (err=%+v)\n", allFilePath, err)
        return
      }
    }
    // Update the fetched dates now that we've succeeded for this company.
    fetchedDates.update(submissions[0].FilingDate, submissions[len(submissions) - 1].FilingDate)
    fetchedDateMap[companyId] = fetchedDates
    writeToJsonFile(kFetchedMapFile, fetchedDateMap)
  }
}
