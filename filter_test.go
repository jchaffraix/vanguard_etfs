package main

import (
  "fmt"
  "testing"
)

const kAccessionNumberTemplate = "000035001-05-%05d"

func generateSubmissionInfos(dates []string) []SubmissionInfo {
  res := []SubmissionInfo{}
  for i, date := range dates {
    res = append(res, SubmissionInfo{kCik, fmt.Sprintf(kAccessionNumberTemplate, i), date})
  }
  return res
}

func TestFilterFilingDate(t *testing.T) {
  tt := []struct {
    name string
    filingDates []string
    fetchedDates FilingDateSpan
    expected []int
  } {
    {"No fetchedDates means no filtering", []string{"2025-10-01", "2025-10-01"}, FilingDateSpan{"", ""}, []int{0, 1}},
    {"Filter a single date at start", []string{"2025-10-01", "2025-10-01", "2025-10-02"}, FilingDateSpan{"2025-10-01", "2025-10-01"}, []int{2}},
    {"Filter a single date at end", []string{"2025-10-01", "2025-10-01", "2025-10-02"}, FilingDateSpan{"2025-10-02", "2025-10-02"}, []int{0, 1}},
    {"Filter a single date, removes all", []string{"2025-10-01", "2025-10-01", "2025-10-01"}, FilingDateSpan{"2025-10-01", "2025-10-01"}, []int{}},
    {"Filter a span at start", []string{"2025-10-01", "2025-10-01", "2025-10-02", "2025-10-03"}, FilingDateSpan{"2025-10-01", "2025-10-02"}, []int{3}},
    {"Filter a span at end", []string{"2025-10-01", "2025-10-01", "2025-10-02", "2025-10-03"}, FilingDateSpan{"2025-10-02", "2025-10-03"}, []int{0, 1}},
  }

  for _, tc := range tt {
    t.Run(tc.name, func (t *testing.T) {
      infos := generateSubmissionInfos(tc.filingDates)
      filteredInfos := filterFilingDates(infos, tc.fetchedDates)
      if len(tc.expected) != len(filteredInfos) {
        t.Errorf("Mismatch in length, expected=%+v (len=%d) but got=%+v (len=%d)", tc.expected, len(tc.expected), filteredInfos, len(filteredInfos))
        return
      }
      for i, j := range tc.expected {
        if filteredInfos[i] != infos[j] {
          t.Errorf("Mismatch at %d (expected=%+v vs got=%+v)", i, infos[j], filteredInfos[i])
          return
        }
      }
    })
  }
}

func TestUpdateDateSpan(t *testing.T) {
  tt := []struct {
    name string
    in FilingDateSpan
    newStart string
    newEnd string
    expected FilingDateSpan
  } {
    {"Same start and end", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-01", "2025-01-02", FilingDateSpan{"2025-01-01", "2025-01-02"}},
    {"Earlier start, same end", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-00", "2025-01-02", FilingDateSpan{"2025-01-00", "2025-01-02"}},
    {"Later start, same end", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-02", "2025-01-02", FilingDateSpan{"2025-01-01", "2025-01-02"}},
    {"Same start, earlier end", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-01", "2025-01-01", FilingDateSpan{"2025-01-01", "2025-01-02"}},
    {"Same start, later end", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-01", "2025-01-10", FilingDateSpan{"2025-01-01", "2025-01-10"}},
    // Empty FilingDateSpan
    {"Empty, setting start/end", FilingDateSpan{"", ""}, "2025-01-01", "2025-01-02", FilingDateSpan{"2025-01-01", "2025-01-02"}},
    // Case where start/end were swapped.
    {"Same start, end earlier than start", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-01", "2025-01-00", FilingDateSpan{"2025-01-00", "2025-01-02"}},
    {"start after end, same end", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-03", "2025-01-02", FilingDateSpan{"2025-01-01", "2025-01-03"}},
    {"start/end swapped", FilingDateSpan{"2025-01-01", "2025-01-02"}, "2025-01-03", "2025-01-00", FilingDateSpan{"2025-01-00", "2025-01-03"}},
    {"empty, start/end swapped", FilingDateSpan{"", ""}, "2025-01-02", "2025-01-01", FilingDateSpan{"2025-01-01", "2025-01-02"}},
  }

  for _, tc := range tt {
    t.Run(tc.name, func (t *testing.T) {
      tc.in.update(tc.newStart, tc.newEnd)
      if tc.in.Start != tc.expected.Start {
        t.Errorf("Mismatch at start: expected=%+v vs got=%+v", tc.expected.Start, tc.in.Start)
        return
      }
      if tc.in.End != tc.expected.End {
        t.Errorf("Mismatch at end: expected=%+v vs got=%+v", tc.expected.End, tc.in.End)
        return
      }
    })
  }
}
