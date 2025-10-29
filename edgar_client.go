package main

import (
  "encoding/xml"
  "encoding/json"
  "net/http"
  "time"
)

type EdgarClient struct {
  userAgent string
  timer *time.Timer
}

// kThrottleDuration is 110ms, which sets the rate limiting to slightly less than 10/sec.
// This is required by https://www.sec.gov/about/privacy-information#security
//
// Note:  https://go.dev/wiki/RateLimiting recommend a time.Ticker, but that doesn't apply here.
// TODO: Do we want to callers to customize this?
const kThrottleDuration = 110 * time.Millisecond

func NewEdgarClient(userAgent string) EdgarClient {
  return EdgarClient{userAgent, nil}
}

func (c EdgarClient) GetResp(url string) (*http.Response, error) {
  if c.timer != nil {
    // If we have a timer, wait until it has triggered to ensure proper throttling.
    // We must clean the timer as we could wait forever if it has already triggered.
    <-c.timer.C
    c.timer = nil
  }

  req, err := http.NewRequest("GET", url, nil)
  if err != nil {
    return nil, err
  }

  req.Header.Add("User-Agent", c.userAgent)
  req.TransferEncoding = append(req.TransferEncoding, "gzip", "deflate")
  req.Header.Add("Host", "www.sec.gov")

  client := &http.Client{}
  resp, err := client.Do(req)
  c.timer = time.NewTimer(kThrottleDuration)
  return resp, err
}

func (c EdgarClient) GetXml(url string, v any) error {
  resp, err := c.GetResp(url)
  if err != nil {
    return err
  }

  defer resp.Body.Close()
  // TODO: We can't easily debug this...
  decoder := xml.NewDecoder(resp.Body)
  return decoder.Decode(v)
}

func (c EdgarClient) GetJson(url string, v any) error {
  resp, err := c.GetResp(url)
  if err != nil {
    return err
  }

  defer resp.Body.Close()
  // TODO: We can't easily debug this...
  decoder := json.NewDecoder(resp.Body)
  return decoder.Decode(v)
}
