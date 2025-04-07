module github.com/withObsrvr/flow-processor-soroswap-router

go 1.23.4

require (
	github.com/stellar/go v0.0.0-20250311234916-385ac5aca1a4
	github.com/withObsrvr/pluginapi v0.0.0-20250303141549-e645e333195c
)

require (
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/stellar/go-xdr v0.0.0-20231122183749-b53fb00bcac2 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
)

// Fix ambiguous import
replace google.golang.org/grpc/stats/opentelemetry => google.golang.org/grpc/stats/opentelemetry v0.0.0-20241028142157-ada6787961b3

// Ensure exact same version as the main application
replace golang.org/x/sys => golang.org/x/sys v0.31.0
