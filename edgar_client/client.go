package edgar_client

import (
  "encoding/xml"
  "encoding/json"
  "errors"
  "fmt"
  "net/http"
  "time"

  "github.com/jmhodges/clock"
)

type EdgarClient struct {
  userAgent string
  client *http.Client // mandatory
  rpsThrottler *Throttler // mandatory
  globalThrottler *Throttler // mandatory
}

// kDefaultThrottleDuration is 105ms, which sets the rate limiting to slightly less than 10/sec.
// This is required by https://www.sec.gov/about/privacy-information#security
//
// Note:  https://go.dev/wiki/RateLimiting recommend a time.Ticker, but that doesn't apply here.
const kDefaultThrottleDuration = 105 * time.Millisecond

// I strongly suspect EDGAR to use a token bucket algorithm over a 10 mins window.
// If we go over the allotted amount, the IP gets automatically banned.
// Thus we implement a global limit over the token bucket's alleged window.
//
// Those limits are best guesses and somewhat conservative to prevent being banned.
const kFetchesBeforeSleep = 80
const kGlobalSleepDuration = 12 * time.Minute

func New(userAgent string) EdgarClient {
  return internalNew(clock.New(), userAgent, 10)
}

func NewWithRps(userAgent string, rps int) EdgarClient {
  return internalNew(clock.New(), userAgent, rps)
}

func internalNew(clock clock.Clock, userAgent string, rps int) EdgarClient {
  if rps <= 0 {
    panic("RPS must be in [1, 10]")
  }
  if rps > 10 {
    panic("We don't allow rps > 10 per the SEC guideline to access EDGAR")
  }

  // We consider the 10 rps limit as a hard limit so use the
  // padded kDefaultThrottleDuration to avoid being impacted by
  // potential clock inaccuracies.
  throttleDuration := kDefaultThrottleDuration
  if rps < 10 {
    // Multiplying first gives an actual RPS closer to the requested one.
    throttleDuration = 1000 * time.Millisecond
    throttleDuration /= time.Duration(rps)
  }

  // TODO: Get the context with the caller so we can cancel all requests?
  // TODO: What should be passed to the client?
  client := &http.Client{}
  rpsThrottler := newThrottler(clock, throttleDuration, 1)
  globalThrottler := newThrottler(clock, kGlobalSleepDuration, kFetchesBeforeSleep)
  return EdgarClient{userAgent, client, rpsThrottler, globalThrottler}
}

func (c EdgarClient) RemainingFetchesBeforeSleeping() int {
  return c.globalThrottler.RemainingFetches()
}

func (c EdgarClient) Sleep() {
  c.globalThrottler.ForcedWait()
  // Also reset the rps throttler as we should have waited longer than a second.
  c.rpsThrottler.Reset()
}

func (c *EdgarClient) GetResp(url string) (*http.Response, error) {
  if c.globalThrottler.MaybeThrottle() {
    // Also reset the rps throttler as we should have waited longer than a second.
    c.rpsThrottler.Reset()
  }
  c.rpsThrottler.MaybeThrottle()

  req, err := http.NewRequest("GET", url, nil)
  if err != nil {
    return nil, err
  }

  req.Header.Add("User-Agent", c.userAgent)
  req.TransferEncoding = append(req.TransferEncoding, "gzip", "deflate")
  req.Header.Add("Host", "www.sec.gov")

  resp, err := c.client.Do(req)
  if resp.StatusCode != 200 {
    return nil, errors.New(fmt.Sprintf("Non-2xx answer: %d", resp.StatusCode))
  }
  return resp, err
}

func (c *EdgarClient) GetXml(url string, v any) error {
  resp, err := c.GetResp(url)
  if err != nil {
    return err
  }

  defer resp.Body.Close()
  // TODO: We can't easily debug this...
  decoder := xml.NewDecoder(resp.Body)
  return decoder.Decode(v)
}

func (c *EdgarClient) GetJson(url string, v any) error {
  resp, err := c.GetResp(url)
  if err != nil {
    return err
  }

  defer resp.Body.Close()
  // TODO: We can't easily debug this...
  decoder := json.NewDecoder(resp.Body)
  return decoder.Decode(v)
}
