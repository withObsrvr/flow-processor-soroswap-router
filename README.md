# Flow Processor for Soroswap Router Events

This processor plugin for the Flow framework processes Soroswap router contract events from the Stellar blockchain. It extracts and processes three types of events:

1. **Swap Events**: When tokens are swapped through the Soroswap router
2. **Add Events**: When liquidity is added through the router
3. **Remove Events**: When liquidity is removed through the router

## Features

- Extracts and processes Soroswap router contract events
- Tracks token pairs, amounts, and account information
- Forwards structured events to downstream consumers
- Provides statistics on processed events

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