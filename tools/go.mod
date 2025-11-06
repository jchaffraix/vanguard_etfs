module gen_etf_files

go 1.24.2

replace edgar_client => ../edgar_client

require (
	edgar_client v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.46.0
)
