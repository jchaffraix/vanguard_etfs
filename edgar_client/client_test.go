package edgar_client

import (
  "bytes"
  "io"
  "testing"
  "net/http"
  "net/http/httptest"
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

  client := New("foobar")
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

  client := New("foobar")
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

  client := New("foobar")
  if client.rpsThrottler != nil || client.globalThrottler != nil {
    t.Errorf("Unexpected rpsThrottler")
  }
  remaining := client.remainingFetches
  resp, err := client.GetResp(server.URL)
  checkSuccessful(t, resp, err)
  if client.rpsThrottler == nil {
    t.Errorf("Missing rpsThrottler after call")
  }
  if client.globalThrottler == nil {
    t.Errorf("Missing globalThrottler after call")
  }
  if client.remainingFetches != remaining - 1 {
    t.Errorf("remainingFetches not correct, expected=%d, but got=%d", remaining - 1, client.remainingFetches)
  }
}

func TestSingleCallServerFailure(t *testing.T) {
  server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      w.WriteHeader(http.StatusInternalServerError)
  }))
  defer server.Close()

  client := New("foobar")
  if client.rpsThrottler != nil || client.globalThrottler != nil {
    t.Errorf("Unexpected rpsThrottler")
  }
  remaining := client.remainingFetches
  _, err := client.GetResp(server.URL)
  if err == nil {
    t.Errorf("Should have gotten an error from the request")
  }
  if client.rpsThrottler == nil {
    t.Errorf("Missing rpsThrottler after call")
  }
  if client.globalThrottler == nil {
    t.Errorf("Missing globalThrottler after call")
  }
  if client.remainingFetches != remaining - 1 {
    t.Errorf("remainingFetches not correct, expected=%d, but got=%d", remaining - 1, client.remainingFetches)
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

  client := New("foobar")
  remaining := client.remainingFetches
  for i := 1; i <= 5; i++ {
    resp, err := client.GetResp(server.URL)
    remaining -= 1
    checkSuccessful(t, resp, err)
    if client.remainingFetches != remaining {
      t.Errorf("remainingFetches was not updated, expected=%d, but got=%d)", remaining, client.remainingFetches)
    }
    var jsonBlob JsonBlob
    err = client.GetJson(server.URL + "/json", &jsonBlob)
    remaining -= 1
    if err != nil {
      t.Errorf("Got unexpected error on GetJson, err=%+v)", err)
    }
    if jsonBlob.Value != "fixed" {
      t.Errorf("Got unexpected value on GetJson, value=%s)", jsonBlob.Value)
    }
    if client.remainingFetches != remaining {
      t.Errorf("remainingFetches was not updated, expected=%d, but got=%d)", remaining, client.remainingFetches)
    }

    var xmlBlob XmlBlob
    err = client.GetXml(server.URL + "/xml", &xmlBlob)
    remaining -= 1
    if err != nil {
      t.Errorf("Got unexpected error on GetXml, err=%+v)", err)
    }
    if xmlBlob.Value != "fixed" {
      t.Errorf("Got unexpected value on GetXml, value=%s)", xmlBlob.Value)
    }
    if client.remainingFetches != remaining {
      t.Errorf("remainingFetches was not updated, expected=%d, but got=%d)", remaining, client.remainingFetches)
    }
  }
  // Check that we do have throttlers in place after the calls.
  if client.rpsThrottler == nil {
    t.Errorf("Missing rpsThrottler after call")
  }
  if client.globalThrottler == nil {
    t.Errorf("Missing globalThrottler after call")
  }
}
