# Soroswap Router Flow Processor

A Flow processor plugin for handling Soroswap router events.

## Features

- Processes Soroswap router events (swaps, adds, removes)
- Dual representation of data (raw and decoded)
- Rich error handling with context
- Thread-safe operations
- GraphQL schema integration
- Comprehensive metrics and monitoring

## Building with Nix

This project uses Nix for reproducible builds.

### Prerequisites

- [Nix package manager](https://nixos.org/download.html) with flakes enabled

### Building

1. Clone the repository:
```bash
git clone https://github.com/withObsrvr/flow-processor-soroswap-router.git
cd flow-processor-soroswap-router
```

2. Build with Nix:
```bash
nix build
```

The built plugin will be available at `./result/lib/flow-processor-soroswap-router.so`.

### Development

To enter a development shell with all dependencies:
```bash
nix develop
```

This will automatically vendor dependencies if needed and provide a shell with all necessary tools.

### Manual Build (Inside Nix Shell)

Once in the development shell, you can manually build the plugin:
```bash
go mod tidy
go mod vendor
go build -buildmode=plugin -o flow-processor-soroswap-router.so .
```

## Configuration

The processor accepts the following configuration:

```json
{
  "flow_api_version": "1.0.0"
}
```

## GraphQL Schema

The processor provides the following GraphQL schema:

```graphql
type RouterEvent {
    type: String!
    timestamp: String!
    ledger_sequence: Int!
    contract_id: String!
    account: String
    token_a: String
    token_b: String
    amount_a: String
    amount_b: String
    tx_hash: String!
}

type SwapEvent {
    router_event: RouterEvent!
    path: [String!]!
}

type Query {
    getRouterEvents(contract_id: String): [RouterEvent!]!
    getSwapEvents(contract_id: String): [SwapEvent!]!
}
```

## Metrics & Monitoring

The processor exposes these operational metrics:

| Metric | Type | Description |
|--------|------|-------------|
| processed_events | Counter | Total number of events processed |
| swap_events | Counter | Number of swap events processed |
| add_events | Counter | Number of add events processed |
| remove_events | Counter | Number of remove events processed |
| last_event_time | Timestamp | When the last event was processed |
| errors.processing | Counter | Number of processing errors |
| errors.parsing | Counter | Number of parsing errors |
| errors.validation | Counter | Number of validation errors |
| uptime | Duration | How long the processor has been running |

## Troubleshooting

### Plugin Version Compatibility

Make sure the plugin is built with the exact same Go version that Flow uses. If you see an error like "plugin was built with a different version of package internal/goarch", check that your Go version matches the one used by the Flow application.

### CGO Required

Go plugins require CGO to be enabled. The Nix build and development shell handle this automatically, but if building manually outside of Nix, ensure you've set:
```bash
export CGO_ENABLED=1
```

### Vendoring Dependencies

For reliable builds, we recommend using vendored dependencies:
```bash
go mod vendor
git add vendor
```

## License

This project is licensed under the terms specified in the repository.

## Usage

### Building the Plugin

```bash
go build -buildmode=pie -o flow-processor-soroswap-router.so
```

### Pipeline Configuration

Add this processor to your Flow pipeline configuration:

```yaml
pipelines:
  SoroswapRouterPipeline:
    source:
      type: "BufferedStorageSourceAdapter"
      config:
        bucket_name: "your-bucket-name"
        network: "mainnet" # or "testnet"
        num_workers: 10
        retry_limit: 3
        retry_wait: 5
        start_ledger: 56000000 # Adjust as needed
        ledgers_per_file: 1
        files_per_partition: 64000
    processors:
      - type: "flow/processor/contract-events"
        config:
          network_passphrase: "Public Global Stellar Network ; September 2015" # or testnet
      - type: "flow/processor/soroswap-router"
        config: {}
    consumers:
      - type: "flow/consumer/sqlite"
        config:
          db_path: "flow_data_soroswap_router.db"
```

## Event Types

### Router Event (Base)

All events share these base fields:

```json
{
  "type": "swap|add|remove",
  "timestamp": "2023-06-01T12:00:00Z",
  "ledger_sequence": 48000000,
  "contract_id": "ROUTER_CONTRACT_ID",
  "account": "USER_ACCOUNT_ID",
  "token_a": "TOKEN_A_CONTRACT_ID",
  "token_b": "TOKEN_B_CONTRACT_ID",
  "amount_a": "1000000000",
  "amount_b": "2000000000",
  "tx_hash": "TRANSACTION_HASH"
}
```

### Swap Event

Swap events include the base fields and may include additional path information for multi-hop swaps.

## Integration with Flow

This processor is designed to work with the Flow pipeline system and can be chained with other processors and consumers. It receives contract events from the `flow/processor/contract-events` processor and forwards structured router events to downstream consumers.

## Statistics

The processor tracks the following statistics:

- Total processed events
- Number of swap events
- Number of add events
- Number of remove events
- Timestamp of the last processed event 