package main

import (
  "testing"
)

const kCompanyId = 36405
const kValidSeriesId = "S000002841"
const kInvalidSeriesId = "S123452841"
const kDate = "2025-01-01"

func TestValidate(t *testing.T) {
  tt := []struct {
    name string
    index Index
    hasError bool
    hasWarning bool
  } {
    {"Validate the name of the index", Index{"", kValidSeriesId, kDate, []IndexComponent{}}, true, false},
    {"Validate that the seriesId is known", Index{"Index", kInvalidSeriesId, kDate, []IndexComponent{}}, false, true},
    {"Validate that the component have a name ", Index{"Index", kValidSeriesId, kDate, []IndexComponent{IndexComponent{"N/A", "", "JPY", "", 0.0039280644}}}, true, false},
    {"Validate that the component have at least one ID", Index{"Index", kValidSeriesId, kDate, []IndexComponent{IndexComponent{"Company", "", "", "", 0.0039280644}}}, true, false},
    {"Validate that a component has a positive weight", Index{"Index", kValidSeriesId, kDate, []IndexComponent{IndexComponent{"BMC Medical Co Ltd","CNE100005WQ4", "", "", -0.0039280644}}}, true, false},
  }

  for _, tc := range tt {
    t.Run(tc.name, func (t *testing.T) {
      res := validateIndex(kCompanyId, tc.index)
      if tc.hasError && len(res.errors) == 0 {
        t.Errorf("Expected errors but got none (res=%+v)", res)
      }
      if !tc.hasError && len(res.errors) > 0 {
        t.Errorf("Expected no errors but got some errors=%+v", res.errors)
      }
      if tc.hasWarning && len(res.warnings) == 0 {
        t.Errorf("Expected warnings but got none (res=%+v)", res)
      }
      if !tc.hasWarning && len(res.warnings) > 0 {
        t.Errorf("Expected no warnings but got some warnings=%+v", res.warnings)
      }
    })
  }
}
