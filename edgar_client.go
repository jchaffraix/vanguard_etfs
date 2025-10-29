package main

import (
  "encoding/xml"
  "encoding/json"
  "net/http"
)

type EdgarClient struct {
  userAgent string
}
// TODO: Implement rate limiting at 10/sec (or less).
// https://go.dev/wiki/RateLimiting

func NewEdgarClient(userAgent string) EdgarClient {
  return EdgarClient{userAgent}
}

func (c EdgarClient) GetResp(url string) (*http.Response, error) {
  req, err := http.NewRequest("GET", url, nil)
  if err != nil {
    return nil, err
  }

  req.Header.Add("User-Agent", c.userAgent)
  req.TransferEncoding = append(req.TransferEncoding, "gzip", "deflate")
  req.Header.Add("Host", "www.sec.gov")

  client := &http.Client{}
  resp, err := client.Do(req)
  if err != nil {
    return nil, err
  }
  //fmt.Print("Resp headers: %+v", resp.Header)
  return resp, nil
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
