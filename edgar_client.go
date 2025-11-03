package main

import (
  "encoding/xml"
  "encoding/json"
  "errors"
  "fmt"
  "net/http"
  "time"
)

type EdgarClient struct {
  userAgent string
  throttleDuration time.Duration
  client *http.Client // mandatory
  timer *time.Timer // may be null
}

// kDefaultThrottleDuration is 105ms, which sets the rate limiting to slightly less than 10/sec.
// This is required by https://www.sec.gov/about/privacy-information#security
//
// Note:  https://go.dev/wiki/RateLimiting recommend a time.Ticker, but that doesn't apply here.
const kDefaultThrottleDuration = 105 * time.Millisecond

func NewEdgarClient(userAgent string) EdgarClient {
  // TODO: Get the context with the caller so we can cancel all requests?
  // TODO: What should be passed to the client?
  client := &http.Client{}
  return EdgarClient{userAgent, kDefaultThrottleDuration, client, nil}
}

func NewEdgarClientWithRps(userAgent string, rps int) EdgarClient {
  if rps <= 0 {
    panic("RPS must be in [1, 10]")
  }
  if rps > 10 {
    panic("We don't allow rps > 10 per the SEC guideline to access EDGAR")
  }

  // Multiplying first gives an actual RPS closer to the requested one.
  throttleDuration := 1000 * time.Millisecond
  throttleDuration /= time.Duration(rps)

  // We consider the 10 rps limit as a hard limit so use the
  // padded kDefaultThrottleDuration to avoid being impacted by
  // potential clock inaccuracies.
  if rps == 10 {
    throttleDuration = kDefaultThrottleDuration
  }
  // TODO: Get the context with the caller so we can cancel all requests?
  // TODO: What should be passed to the client?
  client := &http.Client{}
  return EdgarClient{userAgent, throttleDuration, client, nil}
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

  resp, err := c.client.Do(req)
  c.timer = time.NewTimer(c.throttleDuration)
  if resp.StatusCode != 200 {
    return nil, errors.New(fmt.Sprintf("Non-2xx answer: %d", resp.StatusCode))
  }
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
