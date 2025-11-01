package main

import (
  "encoding/xml"
  "fmt"
  "testing"
)

func TestPopulate(t *testing.T) {
  tt := []struct {
    name string
    invstOrSecXml string
    expected IndexComponent
  } {
    {"Submission with `isin` and cusip", `<invstOrSec><name>Warby Parker Inc</name><cusip>93403J106</cusip><identifiers><isin value="US93403J1060"/></identifiers><pctVal>0.003502379516</pctVal></invstOrSec>`, IndexComponent{"Warby Parker Inc", "US93403J1060", "", "", 0.003502379516}},
    {"Submission with `other` identifier", `<invstOrSec><name>Daiichi Sankyo Co Ltd</name><cusip>N/A</cusip><identifiers><other otherDesc="FAID" value="023CVR996"/></identifiers><pctVal>0.000000000105</pctVal></invstOrSec>`, IndexComponent{"Daiichi Sankyo Co Ltd", "", "", "023CVR996", 0.000000000105}},
    {"Submission with `ticker` identifier", `<invstOrSec><name>Viridian Therapeutics Inc</name><cusip>901535101</cusip><identifiers><ticker value="1843576D"/></identifiers><pctVal>0.000001836174</pctVal></invstOrSec>`, IndexComponent{"Viridian Therapeutics Inc", "", "1843576D", "", 0.000001836174}},
  }

  for _, tc := range tt {
    t.Run(tc.name, func (t *testing.T) {
      payload := fmt.Sprintf(`<edgarSubmission><formData><genInfo><seriesName>VANGUARD TOTAL STOCK MARKET INDEX FUND</seriesName><seriesId>S000002848</seriesId></genInfo><invstOrSecs>%s</invstOrSecs></formData></edgarSubmission>`, tc.invstOrSecXml)
      submission := singleSubmission{}
      err := xml.Unmarshal([]byte(payload), &submission)
      if err != nil {
        panic(fmt.Sprintf("Failed to parse XML: %s (error=%+v).\n\nDid you make a mistake in the test?", payload, err))
      }
      index := populateIndexFromSingleSubmission(submission)
      if index.Name != "VANGUARD TOTAL STOCK MARKET INDEX FUND" {
        t.Errorf("Invalid index name, got=%s (submission=%+v)", index.Name, submission)
        return
      }
      if index.SeriesId != "S000002848" {
        t.Errorf("Invalid SeriesId, got=%s (submission=%+v)", index.SeriesId, submission)
        return
      }
      if len(index.Components) != 1 {
        t.Errorf("Expect 1 components but got %d (full_payload=%+v)", len(index.Components), index)
        return
      }
      component := index.Components[0]
      if tc.expected.Name != component.Name {
        t.Errorf("Mismatched name, expected=%s but got=%s", tc.expected.Name, component.Name)
        return
      }

      if tc.expected.Cusip != component.Cusip {
        t.Errorf("Mismatched cusip, expected=%s but got=%s", tc.expected.Cusip, component.Cusip)
        return
      }

      if tc.expected.Ticker != component.Ticker {
        t.Errorf("Mismatched ticker, expected=%s but got=%s", tc.expected.Ticker, component.Ticker)
        return
      }

      if tc.expected.OtherId != component.OtherId {
        t.Errorf("Mismatched otherId, expected=%s but got=%s", tc.expected.OtherId, component.OtherId)
        return
      }

      if tc.expected.Weight != component.Weight {
        t.Errorf("Mismatched weight, expected=%f but got=%f", tc.expected.Weight, component.Weight)
        return
      }
    })
  }
}

func TestPopulateIgnore(t *testing.T) {
  tt := []struct {
    name string
    invstOrSecXml string
  } {
    {"Ignore Swap", `<invstOrSec><name>N/A</name><cusip>N/A</cusip><identifiers><other otherDesc="CONTRACT_VANGUARD_ID" value="V1047133201"/></identifiers><pctVal>-0.003502379516</pctVal><derivativeInfo><swapDeriv derivCat="SWP"></swapDeriv></derivativeInfo></invstOrSec>`},
    {"Ignore Future", `<invstOrSec><name>N/A</name><cusip>N/A</cusip><identifiers><ticker value="RTYU5"/></identifiers><pctVal>0.000674480486</pctVal><derivativeInfo><futrDeriv derivCat="FUT"></futrDeriv></derivativeInfo></invstOrSec>`},
    {"Ignore Forward Rate", `<invstOrSec><name>JPY/USD FWD 20250917</name><cusip>N/A</cusip><identifiers><ticker value="JPY"/></identifiers><pctVal>0.000674480486</pctVal><derivativeInfo><futrDeriv derivCat="FWD"></futrDeriv></derivativeInfo></invstOrSec>`},
    // This doesn't seem to happen as contracts are derivative, but this test documents our behavior.
    {"Ignore CONTRACT_VANGUARD_ID (no <derivativeInfo>)", `<invstOrSec><name>N/A</name><cusip>N/A</cusip><identifiers><other otherDesc="CONTRACT_VANGUARD_ID" value="V1047133201"/></identifiers><pctVal>-0.003502379516</pctVal></invstOrSec>`},
  }
  for _, tc := range tt {
    t.Run(tc.name, func (t *testing.T) {
      payload := fmt.Sprintf(`<edgarSubmission><formData><genInfo><seriesName>VANGUARD TOTAL STOCK MARKET INDEX FUND</seriesName><seriesId>S000002848</seriesId></genInfo><invstOrSecs>%s</invstOrSecs></formData></edgarSubmission>`, tc.invstOrSecXml)
      submission := singleSubmission{}
      err := xml.Unmarshal([]byte(payload), &submission)
      if err != nil {
        panic(fmt.Sprintf("Failed to parse XML: %s (error=%+v).\n\nDid you make a mistake in the test?", payload, err))
      }
      index := populateIndexFromSingleSubmission(submission)
      if index.Name != "VANGUARD TOTAL STOCK MARKET INDEX FUND" {
        t.Errorf("Invalid index name, got=%s", index.Name)
        return
      }
      if index.SeriesId != "S000002848" {
        t.Errorf("Invalid SeriesId, got=%s", index.SeriesId)
        return
      }
      if len(index.Components) != 0 {
        t.Errorf("Expect no component but got %d (full_payload=%+v)", len(index.Components), index)
        return
      }
    })
  }
}
