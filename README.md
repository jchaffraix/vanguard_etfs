# Vanguard ETFs

This repository contains all listings that makes up specific Vanguard ETFs, with their associated weight.

WARNING: Before using the data, you should read the [Considerations](#Considerations) section below for some important information.

## Data layout

The `data/` directory contains all the parsed data:
- `fetched_map.json` contains the start and end (both inclusive) dates that have been parsed.
- `latest/` contains the latest filing for a specific ETF.
- `all/` contains an array of filings for a specific ETF, ordered from the newest to the oldest.

## Sample filing

Here is a trimmed version of a specific filing:

```
{
  "name": "VANGUARD EXTENDED DURATION TREASURY INDEX FUND",
  "series_id": "S000018789",
  "filing_date": "2025-10-28",
  "components": [
    {
      "name": "United States Treasury Strip Coupon",
      "id": "US912834PZ59",
      "id_type": "isin",
      "weight": 2.0219882
    },
    {
      "name": "United States Treasury Strip Principal",
      "id": "US912803ET65",
      "id_type": "isin",
      "weight": 1.9471362
    },
    ....
    {
      "name": "Vanguard Cmt Funds-Vanguard Market Liquidity Fund",
      "id": "CMT001142",
      "id_type": "faid",
      "weight": 0.009467705
    }
  ]
}
```

Each index components has the same format:
- `name`: the name of the security, equity/bond or other ETF.
- `id` and `id_type`: an identifier for the security and its type. Both comes from the filings and are passed as-is without processing. `id_type` is one of (by rough decreasing order of occurrence): "isin", "ticker", "sedol", "faid", "cins", "cusip", "vid".
- `weight`: the weight of the component in the index. Weights are positive, but can be zero for closed positions. `weight` is guaranteed to fit on a single-precision floating point (`float` or `float32`).

Important: Weights may not add up to 100%.

The components are ordered by decreasing weight.

## Considerations

The importer pipeline fetches Vanguard quarterly filings from the SEC systems (form NPORT-P for the curious). As such, the **data may lag by close to a quarter**.

WARNING: If you want something more real-time, you should explore other options.

When processing the filings, any derivatives are removed. The reason is that those are not part of the core benchmark index, but hedges from the fund's managers. Examples of derivative removed from the data are currency swaps, forward rates, futures, or swaptions.

Outside of this, the pipeline doesn't filter based on the weight size so some components have small or even zero weight.
