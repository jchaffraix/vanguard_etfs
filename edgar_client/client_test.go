package edgar_client

import (
  "bytes"
  "io"
  "testing"
  "net/http"
  "net/http/httptest"
  "time"

  "github.com/jmhodges/clock"
)

// This discards the body for case where we just want to check that it's fine.
func checkSuccessful(t *testing.T, resp *http.Response, err error) {
  if err != nil {
    t.Errorf("Expected error=%+v)", err)
    return
  }
  defer resp.Body.Close()
  if resp.StatusCode != 200 {
    t.Errorf("Expected statucCode=%d, expected=200)", resp.StatusCode)
  }
}

func TestThrottlingGetRespNoError(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      if r.URL.Path != "/" {
          t.Errorf("Expected to request '/', got: %s", r.URL.Path)
      }
      /*if r.Header.Get("Accept") != "application/json" {
          t.Errorf("Expected Accept: application/json header, got: %s", r.Header.Get("Accept"))
      }*/
      w.WriteHeader(http.StatusOK)
      w.Write([]byte(`{"value":"fixed"}`))
  }))
  defer server.Close()

  client := internalNew(clock.NewFake(), "foobar", 10)
  resp, err := client.GetResp(server.URL)
  if err != nil {
    t.Errorf("Expected error=%+v)", err)
    return
  }
  defer resp.Body.Close()
  body, err := io.ReadAll(resp.Body)
  if err != nil {
    panic("Unexpected error reading body")
  }
  expected := []byte(`{"value":"fixed"}`)
  if !bytes.Equal(body, expected) {
    t.Errorf("Expected %s, but got=%s)", string(expected), string(body))
  }
}

func TestGetErrorOnRateLimited(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      if r.URL.Path != "/" {
          t.Errorf("Expected to request '/', got: %s", r.URL.Path)
      }
      /*if r.Header.Get("Accept") != "application/json" {
          t.Errorf("Expected Accept: application/json header, got: %s", r.Header.Get("Accept"))
      }*/
      w.WriteHeader(http.StatusTooManyRequests)
      w.Write([]byte(`{"value":"fixed"}`))
  }))
  defer server.Close()

  client := internalNew(clock.NewFake(), "foobar", 10)
  _, err := client.GetResp(server.URL)
  if err == nil {
    t.Errorf("Expected an error but got none")
    return
  }
}

func TestSingleCallSuccess(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      w.WriteHeader(http.StatusOK)
  }))
  defer server.Close()

  client := internalNew(clock.NewFake(), "foobar", 10)
  remaining := client.RemainingFetchesBeforeSleeping()
  resp, err := client.GetResp(server.URL)
  checkSuccessful(t, resp, err)
  if client.RemainingFetchesBeforeSleeping() != remaining - 1 {
    t.Errorf("RemainingFetchesBeforeSleeping() not correct, expected=%d, but got=%d", remaining - 1, client.RemainingFetchesBeforeSleeping())
  }
}

func TestSingleCallServerFailure(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      w.WriteHeader(http.StatusInternalServerError)
  }))
  defer server.Close()

  client := internalNew(clock.NewFake(), "foobar", 10)
  remaining := client.RemainingFetchesBeforeSleeping()
  _, err := client.GetResp(server.URL)
  if err == nil {
    t.Errorf("Should have gotten an error from the request")
  }
  if client.RemainingFetchesBeforeSleeping() != remaining - 1 {
    t.Errorf("RemainingFetchesBeforeSleeping() not correct, expected=%d, but got=%d", remaining - 1, client.RemainingFetchesBeforeSleeping())
  }
}
type JsonBlob struct {
  Value string `json:"value"`
}

type XmlBlob struct {
  Value string `xml:"value"`
}

func TestThrottlingMultipleCallsAndFormats(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      w.WriteHeader(http.StatusOK)
      if r.URL.Path == "/json" || r.URL.Path == "/" {
        w.Header().Add("content-type", "application/json")
        w.Write([]byte(`{"value":"fixed"}`))
      } else if r.URL.Path == "/xml" {
        w.Header().Add("content-type", "application/xml")
        w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><top><value>fixed</value></top>`))
      } else {
        panic("Unexpected path")
      }
  }))
  defer server.Close()

  clock := clock.NewFake()
  client := internalNew(clock, "foobar", 10)
  remaining := client.RemainingFetchesBeforeSleeping()
  fetches := 0
  before := clock.Now()
  for i := 1; i <= 5; i++ {
    resp, err := client.GetResp(server.URL)
    remaining -= 1
    fetches += 1
    checkSuccessful(t, resp, err)
    if client.RemainingFetchesBeforeSleeping() != remaining {
      t.Errorf("RemainingFetchesBeforeSleeping() was not updated, expected=%d, but got=%d)", remaining, client.RemainingFetchesBeforeSleeping())
    }
    var jsonBlob JsonBlob
    err = client.GetJson(server.URL + "/json", &jsonBlob)
    remaining -= 1
    fetches += 1
    if err != nil {
      t.Errorf("Got unexpected error on GetJson, err=%+v)", err)
    }
    if jsonBlob.Value != "fixed" {
      t.Errorf("Got unexpected value on GetJson, value=%s)", jsonBlob.Value)
    }
    if client.RemainingFetchesBeforeSleeping() != remaining {
      t.Errorf("RemainingFetchesBeforeSleeping() was not updated, expected=%d, but got=%d)", remaining, client.RemainingFetchesBeforeSleeping())
    }

    var xmlBlob XmlBlob
    err = client.GetXml(server.URL + "/xml", &xmlBlob)
    remaining -= 1
    fetches += 1
    if err != nil {
      t.Errorf("Got unexpected error on GetXml, err=%+v)", err)
    }
    if xmlBlob.Value != "fixed" {
      t.Errorf("Got unexpected value on GetXml, value=%s)", xmlBlob.Value)
    }
    if client.RemainingFetchesBeforeSleeping() != remaining {
      t.Errorf("RemainingFetchesBeforeSleeping() was not updated, expected=%d, but got=%d)", remaining, client.RemainingFetchesBeforeSleeping())
    }
  }
  after := clock.Now()
  actualDuration := after.Sub(before)
  expectedDuration :=  time.Duration(fetches - 1) * client.rpsThrottler.d
  if actualDuration != expectedDuration {
    t.Errorf("Unexpected amount of time sleeping, expected=%d(%s), but got=%d(%s))", expectedDuration, expectedDuration.String(), actualDuration, actualDuration.String())
  }
}
