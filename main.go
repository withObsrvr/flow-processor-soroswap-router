package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
	"github.com/withObsrvr/pluginapi"
)

// Event types
const (
	EventTypeSwap   = "swap"
	EventTypeAdd    = "add"
	EventTypeRemove = "remove"
)

// RouterEvent represents the base structure for router events
type RouterEvent struct {
	Type           string    `json:"type"`
	Timestamp      time.Time `json:"timestamp"`
	LedgerSequence uint32    `json:"ledger_sequence"`
	ContractID     string    `json:"contract_id"`
	Account        string    `json:"account"`
	TokenA         string    `json:"token_a"`
	TokenB         string    `json:"token_b"`
	AmountA        string    `json:"amount_a"`
	AmountB        string    `json:"amount_b"`
	TxHash         string    `json:"tx_hash"`
}

// SwapEvent adds path information specific to swaps
type SwapEvent struct {
	RouterEvent
	Path []string `json:"path"`
}

// ContractEvent represents a contract event
type ContractEvent struct {
	Timestamp       time.Time       `json:"timestamp"`
	LedgerSequence  uint32          `json:"ledger_sequence"`
	ContractID      string          `json:"contract_id"`
	Topic           []xdr.ScVal     `json:"topic"`
	Data            json.RawMessage `json:"data"`
	TransactionHash string          `json:"transaction_hash"`
}

// SoroswapRouterProcessor handles router events from Soroswap
type SoroswapRouterProcessor struct {
	consumers []pluginapi.Consumer
	mu        sync.RWMutex
	stats     struct {
		ProcessedEvents uint64
		SwapEvents      uint64
		AddEvents       uint64
		RemoveEvents    uint64
		LastEventTime   time.Time
	}
}

// New creates a new instance of the plugin
func New() pluginapi.Plugin {
	return &SoroswapRouterProcessor{}
}

// Name returns the name of the plugin
func (p *SoroswapRouterProcessor) Name() string {
	return "flow/processor/soroswap-router"
}

// Version returns the version of the plugin
func (p *SoroswapRouterProcessor) Version() string {
	return "1.0.0"
}

// Type returns the type of the plugin
func (p *SoroswapRouterProcessor) Type() pluginapi.PluginType {
	return pluginapi.ProcessorPlugin
}

// Initialize sets up the processor with the provided configuration
func (p *SoroswapRouterProcessor) Initialize(config map[string]interface{}) error {
	// No specific configuration needed for this processor
	return nil
}

// RegisterConsumer registers a consumer with this processor
func (p *SoroswapRouterProcessor) RegisterConsumer(consumer pluginapi.Consumer) {
	p.consumers = append(p.consumers, consumer)
}

// Helper function to encode contract IDs
func encodeContractID(contractId []byte) (string, error) {
	return strkey.Encode(strkey.VersionByteContract, contractId)
}

// Process handles router events
func (p *SoroswapRouterProcessor) Process(ctx context.Context, msg pluginapi.Message) error {
	log.Printf("SoroswapRouterProcessor: Processing message with payload type: %T", msg.Payload)

	var contractEvent ContractEvent
	switch payload := msg.Payload.(type) {
	case []byte:
		if err := json.Unmarshal(payload, &contractEvent); err != nil {
			log.Printf("SoroswapRouterProcessor: Error decoding contract event from bytes: %v", err)
			return fmt.Errorf("error decoding contract event: %w", err)
		}
	case json.RawMessage:
		if err := json.Unmarshal(payload, &contractEvent); err != nil {
			log.Printf("SoroswapRouterProcessor: Error decoding contract event from RawMessage: %v", err)
			return fmt.Errorf("error decoding contract event: %w", err)
		}
	case ContractEvent:
		contractEvent = payload
	default:
		log.Printf("SoroswapRouterProcessor: Unexpected payload type: %T", msg.Payload)
		return fmt.Errorf("unexpected payload type: %T", msg.Payload)
	}

	// Add debug logging
	log.Printf("SoroswapRouterProcessor: Received contract event for contract ID: %s", contractEvent.ContractID)

	// Check if we have enough topics
	if len(contractEvent.Topic) < 2 {
		log.Printf("SoroswapRouterProcessor: Not enough topics in event, skipping")
		return nil
	}

	// Check the event type from topics
	var eventType string
	for _, topic := range contractEvent.Topic {
		if topic.Type == xdr.ScValTypeScvSymbol {
			sym := topic.MustSym()
			log.Printf("SoroswapRouterProcessor: Found topic symbol: %s", sym)
			switch sym {
			case "swap":
				eventType = EventTypeSwap
				break
			case "add":
				eventType = EventTypeAdd
				break
			case "remove":
				eventType = EventTypeRemove
				break
			}
		}
	}

	if eventType == "" {
		log.Printf("SoroswapRouterProcessor: Event type not recognized, skipping")
		return nil
	}

	// Parse the event data
	var eventData struct {
		V0 struct {
			Data struct {
				Map []struct {
					Key struct {
						Sym string `json:"Sym"`
					} `json:"Key"`
					Val struct {
						Address struct {
							ContractId []byte `json:"ContractId"`
							AccountId  *struct {
								Ed25519 []byte `json:"Ed25519"`
							} `json:"AccountId"`
						} `json:"Address"`
						I128 struct {
							Lo uint64 `json:"Lo"`
						} `json:"I128"`
						Bytes []byte `json:"Bytes"`
						Vec   []struct {
							Address struct {
								ContractId []byte `json:"ContractId"`
							} `json:"Address"`
							I128 struct {
								Lo uint64 `json:"Lo"`
							} `json:"I128"`
						} `json:"Vec"`
					} `json:"Val"`
				} `json:"Map"`
			} `json:"Data"`
		} `json:"V0"`
	}

	if err := json.Unmarshal(contractEvent.Data, &eventData); err != nil {
		log.Printf("SoroswapRouterProcessor: Error parsing router event data: %v", err)
		return fmt.Errorf("error parsing router event data: %w", err)
	}

	// Create base router event
	routerEvent := RouterEvent{
		Type:           eventType,
		Timestamp:      contractEvent.Timestamp,
		LedgerSequence: contractEvent.LedgerSequence,
		ContractID:     contractEvent.ContractID,
		TxHash:         contractEvent.TransactionHash,
	}

	// Extract data based on event type
	for _, entry := range eventData.V0.Data.Map {
		switch entry.Key.Sym {
		case "path":
			// Handle swap event path
			if entry.Val.Vec != nil && len(entry.Val.Vec) >= 2 {
				if entry.Val.Vec[0].Address.ContractId != nil {
					if contractID, err := encodeContractID(entry.Val.Vec[0].Address.ContractId); err == nil {
						routerEvent.TokenA = contractID
					}
				}
				if entry.Val.Vec[len(entry.Val.Vec)-1].Address.ContractId != nil {
					if contractID, err := encodeContractID(entry.Val.Vec[len(entry.Val.Vec)-1].Address.ContractId); err == nil {
						routerEvent.TokenB = contractID
					}
				}
			}
		case "amounts":
			// Handle swap event amounts
			if entry.Val.Vec != nil && len(entry.Val.Vec) >= 2 {
				routerEvent.AmountA = fmt.Sprintf("%d", entry.Val.Vec[0].I128.Lo)
				routerEvent.AmountB = fmt.Sprintf("%d", entry.Val.Vec[len(entry.Val.Vec)-1].I128.Lo)
			}
		case "token0", "token_a":
			// Handle add/remove event token A
			if entry.Val.Address.ContractId != nil {
				if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
					routerEvent.TokenA = contractID
				}
			}
		case "token1", "token_b":
			// Handle add/remove event token B
			if entry.Val.Address.ContractId != nil {
				if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
					routerEvent.TokenB = contractID
				}
			}
		case "amount0", "amount_a":
			// Handle add/remove event amount A
			routerEvent.AmountA = fmt.Sprintf("%d", entry.Val.I128.Lo)
		case "amount1", "amount_b":
			// Handle add/remove event amount B
			routerEvent.AmountB = fmt.Sprintf("%d", entry.Val.I128.Lo)
		case "to":
			if entry.Val.Address.AccountId != nil && entry.Val.Address.AccountId.Ed25519 != nil {
				if accountID, err := strkey.Encode(strkey.VersionByteAccountID, entry.Val.Address.AccountId.Ed25519); err == nil {
					routerEvent.Account = accountID
				}
			} else if entry.Val.Address.ContractId != nil {
				if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
					routerEvent.Account = contractID
				}
			}
		}
	}

	// Add validation before forwarding the event
	if routerEvent.TokenA == "" || routerEvent.TokenB == "" ||
		routerEvent.AmountA == "" || routerEvent.AmountB == "" {
		log.Printf("Warning: Incomplete event data detected: %+v", routerEvent)
	}

	// Add debug logging after data extraction
	log.Printf("Extracted event data: %+v", routerEvent)
	log.Printf("Router Event Details:")
	log.Printf("  Type: %s", routerEvent.Type)
	log.Printf("  Account: %s", routerEvent.Account)
	log.Printf("  TokenA: %s", routerEvent.TokenA)
	log.Printf("  TokenB: %s", routerEvent.TokenB)
	log.Printf("  AmountA: %s", routerEvent.AmountA)
	log.Printf("  AmountB: %s", routerEvent.AmountB)
	log.Printf("  TxHash: %s", routerEvent.TxHash)
	log.Printf("  ContractID: %s", routerEvent.ContractID)

	// Update stats
	p.mu.Lock()
	p.stats.ProcessedEvents++
	switch eventType {
	case EventTypeSwap:
		p.stats.SwapEvents++
	case EventTypeAdd:
		p.stats.AddEvents++
	case EventTypeRemove:
		p.stats.RemoveEvents++
	}
	p.stats.LastEventTime = time.Now()
	p.mu.Unlock()

	// Forward the event to downstream consumers
	eventBytes, err := json.Marshal(routerEvent)
	if err != nil {
		return fmt.Errorf("error marshaling router event: %w", err)
	}

	log.Printf("Processing %s event: %s (tokens: %s/%s, amounts: %s/%s)",
		eventType, routerEvent.ContractID, routerEvent.TokenA, routerEvent.TokenB,
		routerEvent.AmountA, routerEvent.AmountB)

	// Forward to all registered consumers
	log.Printf("Forwarding event to %d consumers", len(p.consumers))
	for _, consumer := range p.consumers {
		log.Printf("Forwarding to consumer: %s", consumer.Name())
		if err := consumer.Process(ctx, pluginapi.Message{
			Payload:   eventBytes,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"event_type": eventType,
				"data_type":  "router_events",
			},
		}); err != nil {
			return fmt.Errorf("error in consumer chain: %w", err)
		}
	}

	return nil
}

// GetStats returns processor statistics
func (p *SoroswapRouterProcessor) GetStats() struct {
	ProcessedEvents uint64
	SwapEvents      uint64
	AddEvents       uint64
	RemoveEvents    uint64
	LastEventTime   time.Time
} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// Close implements the Consumer interface
func (p *SoroswapRouterProcessor) Close() error {
	// No resources to clean up
	return nil
}
